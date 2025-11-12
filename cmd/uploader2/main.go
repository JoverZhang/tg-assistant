package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"tg-storage-assistant/internal/client"
	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/fileprocessor"
	"tg-storage-assistant/internal/video"
)

func main() {
	ctx := context.Background()

	// Parse configuration from command-line arguments
	cfg, err := config.Parse()
	if err != nil {
		log.Fatal(err)
	}

	// Check if ffmpeg and ffprobe are available (required for video processing)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("WARNING: ffmpeg not found in PATH. Video processing will fail")
		log.Fatal("please install ffmpeg to enable video processing features")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		log.Println("WARNING: ffprobe not found in PATH. Video processing will fail")
		log.Fatal("please install ffprobe (usually bundled with ffmpeg)")
	}

	// Create client
	client, err := client.NewClient(ctx, cfg)
	if err != nil {
		log.Fatal(err)
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

		log.Printf("Found %d files to process", len(files))

		// Process each file
		stats := fileprocessor.Stats{}
		for _, filename := range files {
			stats.Processed++

			// Parse filename
			tag, description, err := fileprocessor.ParseFilename(filename)
			if err != nil {
				log.Printf("WARNING: Skipping file %s - %v", filename, err)
				stats.Failed++
				continue
			}

			// Get full file path
			filePath := processor.GetFilePath(filename)

			// Get file info for logging
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				log.Printf("WARNING: Failed to get file info for %s - %v", filename, err)
				stats.Failed++
				continue
			}

			if !fileprocessor.IsVideoFile(filename) {
				log.Printf("WARNING: Skipping non-video file: %s", filename)
				stats.Failed++
				continue
			}

			// Process video
			log.Printf("Processing video: %s", filename)
			msgID, additionalFiles, err := video.ProcessVideo(client.Client, ctx, peer, filePath, tag, description, cfg.MaxSize)
			if err != nil {
				video.LogFileInfo(filename, fileInfo.Size(), false, err)
				stats.Failed++
				continue
			}

			// Move video and additional files to done directory
			if err := video.MoveVideoFiles(cfg, filename, msgID, additionalFiles); err != nil {
				log.Printf("WARNING: Uploaded %s (msg ID: %d) but failed to move files - %v", filename, msgID, err)
				stats.Failed++
				continue
			}
		}

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
