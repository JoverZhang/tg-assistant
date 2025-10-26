package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"tg-storage-assistant/internal/config"
	"tg-storage-assistant/internal/fileprocessor"
	"tg-storage-assistant/internal/telegram"
)

func main() {
	// Parse configuration from command-line arguments
	cfg, err := config.Parse()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Check if ffmpeg and ffprobe are available (required for video processing)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("WARNING: ffmpeg not found in PATH. Video processing will fail.")
		log.Println("Please install ffmpeg to enable video processing features.")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		log.Println("WARNING: ffprobe not found in PATH. Video processing will fail.")
		log.Println("Please install ffprobe (usually bundled with ffmpeg).")
	}

	// Initialize MTProto uploader
	uploader, err := telegram.NewMTProtoClient(telegram.MTProtoConfig{
		SessionFile: cfg.SessionFile,
		APIID:       cfg.APIID,
		APIHash:     cfg.APIHash,
		Phone:       cfg.Phone,
		ProxyURL:    cfg.ProxyURL,
	})
	if err != nil {
		log.Fatalf("Failed to initialize MTProto uploader: %v", err)
	}
	defer uploader.Close()

	// Initialize file processor
	processor := fileprocessor.NewProcessor(cfg.LocalDir, cfg.DoneDir)

	// Scan for files
	files, err := processor.ScanFiles()
	if err != nil {
		log.Fatalf("Failed to scan files: %v", err)
	}

	if len(files) == 0 {
		log.Println("No files to process")
		return
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

		// Check if file is a video
		isVideo := fileprocessor.IsVideoFile(filename)

		var messageID int

		if isVideo {
			// Video processing workflow
			log.Printf("Processing video: %s", filename)
			msgID, additionalFiles, err := fileprocessor.ProcessVideo(filePath, tag, description, cfg.MaxSize, uploader, cfg.StorageChatID)
			if err != nil {
				fileprocessor.LogFileInfo(filename, fileInfo.Size(), false, err)
				stats.Failed++
				continue
			}
			messageID = msgID

			// Move video and additional files to done directory
			if err := processor.MoveVideoFiles(filename, messageID, additionalFiles); err != nil {
				log.Printf("WARNING: Uploaded %s (msg ID: %d) but failed to move files - %v", filename, messageID, err)
				stats.Failed++
				continue
			}
		} else {
			// Non-video file processing
			caption := fileprocessor.BuildCaption(tag, description)
			msgID, err := uploader.SendMedia(cfg.StorageChatID, filePath, caption)
			if err != nil {
				fileprocessor.LogFileInfo(filename, fileInfo.Size(), false, err)
				stats.Failed++
				continue
			}
			messageID = msgID

			// Move file to done directory with message ID in filename
			if err := processor.MoveFile(filename, messageID); err != nil {
				log.Printf("WARNING: Uploaded %s (msg ID: %d) but failed to move file - %v", filename, messageID, err)
				stats.Failed++
				continue
			}
		}

		fileprocessor.LogFileInfo(filename, fileInfo.Size(), true, nil)
		stats.Succeeded++
	}

	// Print final statistics
	fmt.Println("\n========== Upload Statistics ==========")
	fmt.Printf("Total files processed: %d\n", stats.Processed)
	fmt.Printf("Successfully uploaded:  %d\n", stats.Succeeded)
	fmt.Printf("Failed:                 %d\n", stats.Failed)
	fmt.Println("=======================================")

	if stats.Failed > 0 {
		os.Exit(1)
	}
}
