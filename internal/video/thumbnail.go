package video

import (
	"fmt"
	"image"
	stddraw "image/draw"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/image/draw"
)

// ExtractFrames extracts N frames evenly distributed from a video
// Returns paths to the extracted frame images
func ExtractFrames(videoPath string, count int, outputDir string) ([]string, error) {
	if count <= 0 {
		return nil, fmt.Errorf("frame count must be positive")
	}

	// Check if ffmpeg and ffprobe are available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return nil, fmt.Errorf("ffprobe not found in PATH: %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get video duration using ffprobe
	duration, err := getVideoDuration(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get video duration: %w", err)
	}

	if duration <= 0 {
		return nil, fmt.Errorf("invalid video duration: %f", duration)
	}

	// Calculate timestamps for frame extraction
	interval := duration / float64(count)
	var framePaths []string

	for i := 0; i < count; i++ {
		timestamp := interval * float64(i)
		framePath := filepath.Join(outputDir, fmt.Sprintf("frame_%03d.jpg", i))

		// Extract frame at timestamp
		cmd := exec.Command("ffmpeg",
			"-ss", fmt.Sprintf("%.2f", timestamp),
			"-i", videoPath,
			"-vframes", "1",
			"-q:v", "2", // High quality
			"-y", // Overwrite output files
			framePath,
		)

		// Run ffmpeg with suppressed output
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Run(); err != nil {
			// Clean up already extracted frames
			for _, path := range framePaths {
				os.Remove(path)
			}
			return nil, fmt.Errorf("failed to extract frame %d: %w", i, err)
		}

		framePaths = append(framePaths, framePath)
	}

	return framePaths, nil
}

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

	return nil
}

// // getVideoDuration returns the duration of a video in seconds using ffprobe
// func getVideoDuration(videoPath string) (float64, error) {
// 	cmd := exec.Command("ffprobe",
// 		"-v", "error",
// 		"-show_entries", "format=duration",
// 		"-of", "default=noprint_wrappers=1:nokey=1",
// 		videoPath,
// 	)

// 	output, err := cmd.Output()
// 	if err != nil {
// 		return 0, fmt.Errorf("ffprobe command failed: %w", err)
// 	}

// 	durationStr := strings.TrimSpace(string(output))
// 	duration, err := strconv.ParseFloat(durationStr, 64)
// 	if err != nil {
// 		return 0, fmt.Errorf("failed to parse duration: %w", err)
// 	}

// 	return duration, nil
// }

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
