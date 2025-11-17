package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"tg-storage-assistant/internal/client"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/fileprocessor"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/video"
)

func main() {
	ctx := context.Background()

	// Parse configuration from command-line arguments
	cfg, err := config.Parse()
	if err != nil {
		logger.Error.Fatal(err)
	}

	// Check if ffmpeg and ffprobe are available (required for video processing)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		logger.Error.Fatal("ffmpeg not found in PATH. Video processing will fail")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		logger.Error.Fatal("ffprobe not found in PATH. Video processing will fail")
	}

	// Create client
	client, err := client.NewClient(ctx, cfg)
	if err != nil {
		logger.Error.Fatal(err)
	}

	// Run client
	if err := client.Run(func(ctx context.Context) error {
		// Scan for files
		processor := fileprocessor.NewProcessor(cfg.LocalDir, cfg.DoneDir)
		files, err := processor.ScanFiles()
		if err != nil {
			return fmt.Errorf("failed to scan files: %w", err)
		}

		if len(files) == 0 {
			return fmt.Errorf("no files to process")
		}

		peer, err := client.ResolvePeer(cfg.StorageChatID)
		if err != nil {
			return fmt.Errorf("resolve peer: %w", err)
		}

		logger.Info.Printf("Found %d files to process", len(files))

		// Process each file
		stats := fileprocessor.Stats{}
		for _, filename := range files {
			stats.Processed++

			// Parse filename
			tag, description, err := fileprocessor.ParseFilename(filename)
			if err != nil {
				logger.Warn.Printf("Skipping file %s - %v", filename, err)
				stats.Failed++
				continue
			}

			// Get full file path
			filePath := processor.GetFilePath(filename)

			// Get file info for logging
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				logger.Warn.Printf("Failed to get file info for %s - %v", filename, err)
				stats.Failed++
				continue
			}

			if !fileprocessor.IsVideoFile(filename) {
				logger.Warn.Printf("Skipping non-video file: %s", filename)
				stats.Failed++
				continue
			}

			// Process video
			logger.Info.Printf("Processing video: %s", filename)
			err = video.ProcessVideo(client, peer, filePath, tag, description, cfg.MaxSize, cfg.TempDir, cfg.CleanupTempDir)
			if err != nil {
				video.LogFileInfo(filename, fileInfo.Size(), false, err)
				stats.Failed++
				continue
			}

			// Move video file to done directory
			if err := video.MoveVideoFiles(cfg, filename); err != nil {
				logger.Warn.Printf("Uploaded %s but failed to move file - %v", filename, err)
				stats.Failed++
				continue
			}
		}

		return nil
	}); err != nil {
		logger.Error.Fatal(err)
	}
}
