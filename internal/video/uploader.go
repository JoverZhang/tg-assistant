package video

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tg-storage-assistant/internal/client"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/util"

	"github.com/gotd/td/tg"
)

type MediaItem = client.MediaItem

func ProcessVideo(
	client *client.Client,
	peer tg.InputPeerClass,
	filePath, tag, description string,
	maxSize int64,
	tempDir string, cleanupTempDir bool,
) error {
	defer func() {
		if cleanupTempDir {
			logger.Info.Printf("Cleaning up temporary directory: %s", tempDir)
			os.RemoveAll(tempDir)
		}
	}()

	logger.Info.Println("┏━━━━━━━━━━━━━━━ Processing video... ━━━━━━━━━━━━━━━┓")

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	logger.Info.Printf("  FILE_NAME: %s", filePath)
	logger.Info.Printf("  TAG: %s", tag)
	logger.Info.Printf("  DESCRIPTION: %s", description)
	logger.Info.Printf("  SIZE: %s", util.FormatBytesToHumanReadable(fileInfo.Size()))

	// Step 1: Generate preview thumbnail (5×6 grid, 30 frames)
	logger.Info.Printf("Extracting 30 frames for preview...")
	frames, err := ExtractFrames(filePath, 30, tempDir)
	if err != nil {
		return fmt.Errorf("failed to extract frames: %w", err)
	}

	previewPath := filepath.Join(tempDir, fmt.Sprintf("%s_%s_preview.jpg", tag, description))
	logger.Info.Printf("Composing preview grid...")
	if err := ComposeGrid(frames, 5, 6, previewPath); err != nil {
		return fmt.Errorf("failed to compose grid: %w", err)
	}

	// Step 2: Split video if needed
	logger.Info.Printf("Splitting video into parts if needed...")
	videoParts, err := splitVideoV2(filePath, maxSize, tempDir)
	if err != nil {
		return fmt.Errorf("failed to split video: %w", err)
	}

	// Step 3: Validate media group size
	if 1+len(videoParts) > 10 {
		return fmt.Errorf("media group would have %d items (1 preview + %d video parts), exceeds Telegram limit of 10",
			1+len(videoParts), len(videoParts))
	}

	// Step 4: Build media group
	baseCaption := fmt.Sprintf("#%s %s", tag, strings.ReplaceAll(description, "_", " "))
	var mediaItems []MediaItem

	// First item: preview photo with caption (this is the only caption for the entire album)
	mediaItems = append(mediaItems, MediaItem{
		FilePath:  previewPath,
		MediaType: "photo",
		Caption:   baseCaption,
	})

	// Remaining items: video parts with empty captions
	// Telegram only shows the first item's caption for the entire album
	for _, partPath := range videoParts {
		mediaItems = append(mediaItems, MediaItem{
			FilePath:  partPath,
			MediaType: "video",
			Caption:   "",
		})
	}

	logger.Info.Printf("Preparing album with %d items: 1 preview + %d video parts...", len(mediaItems), len(videoParts))

	err = client.SendMultiMedia(peer, mediaItems)
	if err != nil {
		return fmt.Errorf("failed to send multi media: %w", err)
	}

	logger.Info.Println("┗━━━━━━━━━━━ Video successfully uploaded ━━━━━━━━━━━┛")
	return nil
}

func LogFileInfo(filename string, size int64, success bool, err error) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	sizeKB := float64(size) / 1024.0
	if err != nil {
		logger.Warn.Printf("[%s] %s (%.2f KB) - Error: %v", status, filename, sizeKB, err)
	} else {
		logger.Info.Printf("[%s] %s (%.2f KB)", status, filename, sizeKB)
	}
}

func MoveVideoFiles(cfg *config.Config, originalFilename string) error {
	sourcePath := filepath.Join(cfg.LocalDir, originalFilename)
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)

	newFilename := fmt.Sprintf("%s%s", nameWithoutExt, ext)
	destPath := filepath.Join(cfg.DoneDir, newFilename)

	if err := move(sourcePath, destPath); err != nil {
		return fmt.Errorf("failed to move original video: %w", err)
	}

	return nil
}

func move(src, dst string) error {
	return os.Rename(src, dst)
}

func splitVideo(videoPath string, maxSize int64, outputDir string) ([]string, error) {
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()

	// If no maxSize specified or file is smaller, return original
	if maxSize <= 0 || fileSize <= maxSize {
		return []string{videoPath}, nil
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Prepare output pattern
	ext := filepath.Ext(videoPath)
	baseName := filepath.Base(videoPath)
	baseName = baseName[:len(baseName)-len(ext)]
	outputPattern := filepath.Join(outputDir, fmt.Sprintf("%s_part%%03d%s", baseName, ext))

	totalDuration, err := getVideoDuration(videoPath)
	if err != nil {
		return nil, err
	}

	// Split videos by specified maxSize
	result := []string{}
	curDuration := 0.0
	i := 0
	for curDuration < totalDuration {
		// Split video by maxSize
		outputPath := fmt.Sprintf(outputPattern, i)
		err := splitVideoByDuration(videoPath, outputPath, int64(curDuration), maxSize)
		if err != nil {
			return nil, err
		}
		result = append(result, outputPath)

		newDuration, err := getVideoDuration(outputPath)
		if err != nil {
			return nil, err
		}

		curDuration += newDuration
		i++
	}

	return result, nil
}

func splitVideoV2(videoPath string, maxSize int64, outputDir string) ([]string, error) {
	fileInfo, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	fileSize := fileInfo.Size()

	fname := filepath.Base(videoPath)
	ext := filepath.Ext(fname)
	basename := strings.TrimSuffix(fname, ext)

	// If no maxSize specified or file is smaller, return original
	if maxSize <= 0 || fileSize <= maxSize {
		return []string{videoPath}, nil
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	durSec, err := getVideoDurationSeconds(videoPath)
	if err != nil {
		return nil, err
	}

	bitrate, err := getVideoBitrate(videoPath)
	if err != nil {
		return nil, err
	}
	if bitrate <= 0 {
		bitrate = (fileSize * 8) / durSec
		logger.Warn.Printf("No metadata bitrate, estimate bitrate=%d bps", bitrate)
	}

	segmentTime := (maxSize * 8) / bitrate
	if segmentTime < 1 {
		segmentTime = 1
	}

	logger.Debug.Printf("Video: %s, duration=%ds, bitrate=%d bps, segment_time≈%ds (target %dMB/segment)",
		videoPath, durSec, bitrate, segmentTime, maxSize)

	tmpPattern := filepath.Join(outputDir, basename+"_%03d.ts")
	logger.Debug.Printf("Splitting video (generate .ts): %s", tmpPattern)

	err = generateTSFiles(videoPath, tmpPattern, segmentTime)
	if err != nil {
		return nil, err
	}

	// remux each .ts -> mp4
	tsGlob := filepath.Join(outputDir, basename+"_*"+".ts")
	tsFiles, _ := filepath.Glob(tsGlob)

	result := []string{}

	idx := 0
	for _, tsFile := range tsFiles {
		outMp4 := filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", basename, idx, ext))

		logger.Debug.Printf("remux: %s -> %s", filepath.Base(tsFile), filepath.Base(outMp4))
		err = remuxTSFile(tsFile, outMp4)
		if err != nil {
			return nil, err
		}
		result = append(result, outMp4)
		idx++
	}

	return result, nil
}
