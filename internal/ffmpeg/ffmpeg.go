package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"tg-storage-assistant/internal/logger"
)

func SplitVideoByDuration(videoPath, outputPath string, beginDuration, maxSize int64) error {
	cmd := exec.Command(
		"ffmpeg",
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

func GetVideoDurationSeconds(videoPath string) (int64, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	logger.Debug.Println("Command: ", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}

	durStr := strings.TrimSpace(string(output))
	durf, err := strconv.ParseFloat(durStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}
	return int64(durf), nil
}

func GetVideoBitrate(videoPath string) (int64, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=bit_rate",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	logger.Debug.Println("Command: ", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get video bitrate: %w", err)
	}

	bitrateStr := strings.TrimSpace(string(output))
	bitrate, err := strconv.ParseInt(bitrateStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bitrate: %w", err)
	}
	return bitrate, nil
}

func GenerateTSFiles(outputPath, tmpPattern string, segmentTime int64) error {
	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner", "-loglevel", "info", "-i", outputPath,
		"-c", "copy", "-map", "0",
		"-f", "segment",
		"-segment_time", strconv.FormatInt(segmentTime, 10),
		"-reset_timestamps", "1",
		tmpPattern,
	)
	logger.Debug.Println("Command: ", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate TS files: %w", err)
	}
	return nil
}

func RemuxTSFile(tsFile, outMp4 string) error {
	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner", "-loglevel", "info", "-i", tsFile,
		"-c", "copy", "-bsf:a", "aac_adtstoasc",
		outMp4,
	)
	logger.Debug.Println("Command: ", cmd.String())

	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remux TS file %s -> %s: %w", tsFile, outMp4, err)
	}
	return nil
}

func GetVideoDuration(videoPath string) (float64, error) {
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

func GetVideoResolution(videoPath string) (int, int, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	logger.Debug.Println("Command: ", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get video resolution: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("invalid ffprobe output: %s", output)
	}

	width, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse width: %w", err)
	}

	height, err := strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse height: %w", err)
	}

	return int(width), int(height), nil
}

func ExtractFrames(videoPath, outputPath string, totalDuration float64, count int) ([]string, error) {
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
		cmd := exec.Command(
			"ffmpeg",
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
