package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"tg-storage-assistant/internal/logger"
)

func splitVideoByDuration(videoPath, outputPath string, beginDuration, maxSize int64) error {
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-ss", strconv.FormatInt(beginDuration, 10),
		"-fs", strconv.FormatInt(maxSize, 10),
		"-c", "copy", // Copy codec (no re-encoding)
		"-y", // Overwrite output files
		outputPath)
	logger.Debug.Println("Command: ", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to split video: %w", err)
	}
	return nil
}

func getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command(
		"ffprobe",
		"-i", videoPath,
		"-show_entries", "format=duration",
		"-v", "quiet",
		"-of", "default=noprint_wrappers=1:nokey=1")
	logger.Debug.Println("Command: ", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}
	return duration, nil

}

func extractFrames(videoPath, outputPath string, totalDuration float64, count int) ([]string, error) {
	if totalDuration <= 0 {
		return nil, fmt.Errorf("invalid video duration: %f", totalDuration)
	}

	// Calculate timestamps for frame extraction
	interval := totalDuration / float64(count)
	var framePaths []string

	for i := 0; i < count; i++ {
		timestamp := interval * float64(i)
		framePath := filepath.Join(outputPath, fmt.Sprintf("frame_%03d.jpg", i))

		// Extract frame at timestamp
		cmd := exec.Command("ffmpeg",
			"-ss", fmt.Sprintf("%.2f", timestamp),
			"-i", videoPath,
			"-vframes", "1",
			"-q:v", "2", // High quality
			"-y", // Overwrite output files
			framePath,
		)
		logger.Debug.Println("Command: ", cmd.String())

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
