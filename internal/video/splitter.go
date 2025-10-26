package video

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

// SplitVideo splits a video file into chunks if it exceeds maxSize
// Returns paths to video files (split chunks or original if no split needed)
func SplitVideo(videoPath string, maxSize int64, outputDir string) ([]string, error) {
	// Get file size
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()

	// If no maxSize specified or file is smaller, return original
	if maxSize <= 0 || fileSize <= maxSize {
		return []string{videoPath}, nil
	}

	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Calculate number of chunks needed
	numChunks := int(math.Ceil(float64(fileSize) / float64(maxSize)))

	// Prepare output pattern
	ext := filepath.Ext(videoPath)
	baseName := filepath.Base(videoPath)
	baseName = baseName[:len(baseName)-len(ext)]
	outputPattern := filepath.Join(outputDir, fmt.Sprintf("%s_part%%03d%s", baseName, ext))

	// Split video using ffmpeg
	// Use segment muxer with segment_size to split by size
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-c", "copy", // Copy codec (no re-encoding)
		"-map", "0", // Map all streams
		"-f", "segment",
		"-segment_size", fmt.Sprintf("%d", maxSize),
		"-reset_timestamps", "1",
		"-y", // Overwrite output files
		outputPattern,
	)

	// Suppress ffmpeg output
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg split command failed: %w", err)
	}

	// Collect generated chunk paths
	var chunks []string
	for i := 0; i < numChunks+2; i++ { // +2 as buffer, ffmpeg may create more/fewer chunks
		chunkPath := filepath.Join(outputDir, fmt.Sprintf("%s_part%03d%s", baseName, i, ext))
		if _, err := os.Stat(chunkPath); err == nil {
			chunks = append(chunks, chunkPath)
		} else {
			// No more chunks
			break
		}
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks were created by ffmpeg")
	}

	return chunks, nil
}

// ValidateChunkCount validates that the total number of media items doesn't exceed Telegram's limit
func ValidateChunkCount(numVideoParts int) error {
	// Total items = 1 preview + N video parts
	totalItems := 1 + numVideoParts

	if totalItems > 10 {
		return fmt.Errorf("media group would have %d items (1 preview + %d video parts), exceeds Telegram limit of 10",
			totalItems, numVideoParts)
	}

	return nil
}

// CleanupTempFiles removes temporary files
func CleanupTempFiles(paths []string) error {
	var errors []error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to clean up %d files", len(errors))
	}

	return nil
}
