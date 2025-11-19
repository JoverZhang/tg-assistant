package ffmpeg

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"tg-storage-assistant/internal/logger"
)

func EnsureMP4Compatible(videoPath, outputDir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(videoPath))

	// Is already mp4, check if it's compatible
	if ext == ".mp4" {
		vCodec, aCodec, err := probeCodecs(videoPath)
		if err != nil {
			return "", fmt.Errorf("probe codecs failed for %s: %w", videoPath, err)
		}

		// Return original path if it's compatible
		if isCopyCompatible(vCodec, aCodec) {
			return videoPath, nil
		}

		// Transcode if it's not compatible
		outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.fixed.mp4", filepath.Base(videoPath)))
		if err := transcodeToMP4(videoPath, outputPath); err != nil {
			return "", err
		}
		return outputPath, nil
	}

	// If not mp4, convert to mp4
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	outputPath := filepath.Join(outputDir, base+".mp4")

	vCodec, aCodec, err := probeCodecs(videoPath)
	if err != nil {
		return "", fmt.Errorf("probe codecs failed for %s: %w", videoPath, err)
	}

	// Try to remux if it's compatible
	if isCopyCompatible(vCodec, aCodec) {
		if err := remuxToMP4(videoPath, outputPath); err == nil {
			return outputPath, nil
		}

		// Fallback to transcode
	}

	// Transcode if it's not compatible
	if err := transcodeToMP4(videoPath, outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

func probeCodecs(path string) (videoCodec, audioCodec string, err error) {
	vCmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	logger.Debug.Println("Command: ", vCmd.String())

	var vOut bytes.Buffer
	vCmd.Stdout = &vOut
	if err := vCmd.Run(); err == nil {
		videoCodec = strings.TrimSpace(vOut.String())
	}

	aCmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	logger.Debug.Println("Command: ", aCmd.String())

	var aOut bytes.Buffer
	aCmd.Stdout = &aOut
	if err := aCmd.Run(); err == nil {
		audioCodec = strings.TrimSpace(aOut.String())
	}

	if videoCodec == "" && audioCodec == "" {
		return "", "", fmt.Errorf("no streams detected by ffprobe")
	}

	return videoCodec, audioCodec, nil
}

func isCopyCompatible(vCodec, aCodec string) bool {
	vCodec = strings.ToLower(vCodec)
	aCodec = strings.ToLower(aCodec)

	videoOk := vCodec == "h264" || vCodec == "hevc"
	audioOk := aCodec == "" || aCodec == "aac" || aCodec == "mp3"

	return videoOk && audioOk
}

func remuxToMP4(inputPath, outputPath string) error {
	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-i", inputPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	)
	logger.Debug.Println("Command: ", cmd.String())

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg remux failed: %w, output: %s", err, string(out))
	}
	return nil
}

func transcodeToMP4(inputPath, outputPath string) error {
	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "22",
		"-c:a", "aac",
		"-movflags", "+faststart",
		outputPath,
	)
	logger.Debug.Println("Command: ", cmd.String())

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg transcode failed: %w, output: %s", err, string(out))
	}
	return nil
}
