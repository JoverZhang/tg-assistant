package main

import (
	"fmt"
	"log"
	"os"

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

	// Initialize Telegram uploader
	uploader, err := telegram.NewUploader(cfg.Token)
	if err != nil {
		log.Fatalf("Failed to initialize Telegram uploader: %v", err)
	}

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

		// Build caption
		caption := fileprocessor.BuildCaption(tag, description)

		// Get full file path
		filePath := processor.GetFilePath(filename)

		// Get file info for logging
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			log.Printf("WARNING: Failed to get file info for %s - %v", filename, err)
			stats.Failed++
			continue
		}

		// Upload file
		messageID, err := uploader.SendMedia(cfg.ChatID, filePath, caption)
		if err != nil {
			fileprocessor.LogFileInfo(filename, fileInfo.Size(), false, err)
			stats.Failed++
			continue
		}

		// Move file to done directory with message ID in filename
		if err := processor.MoveFile(filename, messageID); err != nil {
			log.Printf("WARNING: Uploaded %s (msg ID: %d) but failed to move file - %v", filename, messageID, err)
			stats.Failed++
			continue
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
