package video

import (
	"fmt"
	"image"
	stddraw "image/draw"
	"image/jpeg"
	"os"
	"tg-storage-assistant/internal/logger"

	"golang.org/x/image/draw"
)

// ComposeGrid arranges frames into a grid and saves as a single JPEG
func ComposeGrid(framePaths []string, cols, rows int, outputPath string) error {
	if len(framePaths) == 0 {
		return fmt.Errorf("no frames to compose")
	}

	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("invalid grid dimensions: %dx%d", cols, rows)
	}

	expectedFrames := cols * rows
	if len(framePaths) != expectedFrames {
		return fmt.Errorf("frame count mismatch: got %d frames, expected %d (%dx%d grid)",
			len(framePaths), expectedFrames, cols, rows)
	}

	// Load first frame to get original dimensions
	firstFrame, err := loadImage(framePaths[0])
	if err != nil {
		return fmt.Errorf("failed to load first frame: %w", err)
	}
	originalBounds := firstFrame.Bounds()
	originalWidth := originalBounds.Dx()
	originalHeight := originalBounds.Dy()

	// Calculate thumbnail size for each frame
	// Target: final grid should be around 1920-2560 pixels wide (suitable for Telegram)
	// With 6 columns, each thumbnail should be ~320 pixels wide
	thumbnailWidth := 320
	thumbnailHeight := thumbnailWidth * originalHeight / originalWidth

	// Ensure minimum size
	if thumbnailHeight < 180 {
		thumbnailHeight = 180
		thumbnailWidth = thumbnailHeight * originalWidth / originalHeight
	}

	// Create output image
	gridWidth := thumbnailWidth * cols
	gridHeight := thumbnailHeight * rows
	grid := image.NewRGBA(image.Rect(0, 0, gridWidth, gridHeight))

	// Draw frames onto grid
	for i, framePath := range framePaths {
		frame, err := loadImage(framePath)
		if err != nil {
			return fmt.Errorf("failed to load frame %d: %w", i, err)
		}

		// Calculate position in grid
		col := i % cols
		row := i / cols
		x := col * thumbnailWidth
		y := row * thumbnailHeight

		// Create thumbnail rectangle
		thumbRect := image.Rect(x, y, x+thumbnailWidth, y+thumbnailHeight)

		// Resize and draw frame at position using bilinear interpolation
		draw.BiLinear.Scale(grid, thumbRect, frame, frame.Bounds(), stddraw.Over, nil)
	}

	// Save grid as JPEG
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Encode with quality 85
	if err := jpeg.Encode(outFile, grid, &jpeg.Options{Quality: 85}); err != nil {
		return fmt.Errorf("failed to encode JPEG: %w", err)
	}

	logger.Debug.Printf("Grid composed into [%s](%dx%d)",
		outputPath, grid.Bounds().Dx(), grid.Bounds().Dy())
	return nil
}

// loadImage loads an image from a file
func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return img, nil
}
