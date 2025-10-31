package fileprocessor

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tg-storage-assistant/internal/telegram"
	"tg-storage-assistant/internal/video"
)

// Stats tracks processing statistics
type Stats struct {
	Processed int
	Succeeded int
	Failed    int
}

// Processor handles file scanning, parsing, and moving
type Processor struct {
	localDir string
	doneDir  string
}

// NewProcessor creates a new file processor
func NewProcessor(localDir, doneDir string) *Processor {
	return &Processor{
		localDir: localDir,
		doneDir:  doneDir,
	}
}

// ScanFiles returns a sorted list of files in the local directory
func (p *Processor) ScanFiles() ([]string, error) {
	entries, err := os.ReadDir(p.localDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	// Sort alphabetically for predictable processing order
	sort.Strings(files)

	return files, nil
}

// ParseFilename extracts tag and description from filename
// Format: TAG_DESCRIPTION.extension
// Returns: tag, description, error
func ParseFilename(filename string) (string, string, error) {
	// Remove extension
	nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Split on first underscore
	parts := strings.SplitN(nameWithoutExt, "_", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid filename format: expected TAG_DESCRIPTION.ext, got %s", filename)
	}

	tag := parts[0]
	description := parts[1]

	if tag == "" || description == "" {
		return "", "", fmt.Errorf("invalid filename format: tag or description is empty")
	}

	return tag, description, nil
}

// BuildCaption creates a caption from tag and description
// Format: #TAG DESCRIPTION (with underscores replaced by spaces in description)
func BuildCaption(tag, description string) string {
	// Replace underscores with spaces in description
	readableDescription := strings.ReplaceAll(description, "_", " ")
	return fmt.Sprintf("#%s %s", tag, readableDescription)
}

// MoveFile moves a file from source to destination
// Renames the file to include message ID: originalname_msgid_<ID>.ext
func (p *Processor) MoveFile(filename string, messageID int) error {
	sourcePath := filepath.Join(p.localDir, filename)

	// Build new filename with message ID
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)
	newFilename := fmt.Sprintf("%s_msgid_%d%s", nameWithoutExt, messageID, ext)
	destPath := filepath.Join(p.doneDir, newFilename)

	// Try rename first (fast, works if same filesystem)
	err := os.Rename(sourcePath, destPath)
	if err == nil {
		return nil
	}

	// Fallback to copy+delete (works across filesystems)
	if err := copyFile(sourcePath, destPath); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	if err := os.Remove(sourcePath); err != nil {
		return fmt.Errorf("failed to remove source file: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

// GetFilePath returns the full path to a file in the local directory
func (p *Processor) GetFilePath(filename string) string {
	return filepath.Join(p.localDir, filename)
}

// LogFileInfo logs information about a file
func LogFileInfo(filename string, size int64, success bool, err error) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	sizeKB := float64(size) / 1024.0
	if err != nil {
		log.Printf("[%s] %s (%.2f KB) - Error: %v", status, filename, sizeKB, err)
	} else {
		log.Printf("[%s] %s (%.2f KB)", status, filename, sizeKB)
	}
}

// IsVideoFile checks if a file is a video based on extension
func IsVideoFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	videoExts := []string{".mp4", ".avi", ".mov", ".mkv", ".webm", ".flv"}
	for _, videoExt := range videoExts {
		if ext == videoExt {
			return true
		}
	}
	return false
}

// ProcessVideo handles video processing workflow: preview generation, splitting, and upload
// Returns message ID and error
func ProcessVideo(filePath, tag, description string, maxSize int64, uploader interface {
	SendMediaGroup(int64, []telegram.MediaItem) (int, error)
}, chatID int64) (int, []string, error) {
	// Create temporary directory for video processing
	tempDir := filepath.Join(os.TempDir(), "uploader_video_processing")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return 0, nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	var tempFiles []string
	cleanup := func() {
		for _, f := range tempFiles {
			os.Remove(f)
		}
		os.RemoveAll(tempDir)
	}

	// Step 1: Generate preview thumbnail (5×6 grid, 30 frames)
	log.Printf("Extracting 30 frames for preview...")
	frames, err := video.ExtractFrames(filePath, 30, tempDir)
	if err != nil {
		cleanup()
		return 0, nil, fmt.Errorf("failed to extract frames: %w", err)
	}
	tempFiles = append(tempFiles, frames...)

	previewPath := filepath.Join(tempDir, fmt.Sprintf("%s_%s_preview.jpg", tag, description))
	log.Printf("Composing preview grid...")
	if err := video.ComposeGrid(frames, 6, 5, previewPath); err != nil {
		cleanup()
		return 0, nil, fmt.Errorf("failed to compose grid: %w", err)
	}
	tempFiles = append(tempFiles, previewPath)

	// Step 2: Split video if needed
	log.Printf("Checking video size for splitting...")
	videoParts, err := video.SplitVideo(filePath, maxSize, tempDir)
	if err != nil {
		cleanup()
		return 0, nil, fmt.Errorf("failed to split video: %w", err)
	}

	// Add split parts to temp files (but not original video)
	for _, part := range videoParts {
		if part != filePath {
			tempFiles = append(tempFiles, part)
		}
	}

	// Step 3: Validate media group size
	if err := video.ValidateChunkCount(len(videoParts)); err != nil {
		cleanup()
		return 0, nil, err
	}

	// Step 4: Build media group
	baseCaption := fmt.Sprintf("#%s %s", tag, strings.ReplaceAll(description, "_", " "))
	var mediaItems []telegram.MediaItem

	// First item: preview photo with caption (this is the only caption for the entire album)
	mediaItems = append(mediaItems, telegram.MediaItem{
		FilePath:  previewPath,
		MediaType: "photo",
		Caption:   baseCaption,
	})

	// Remaining items: video parts with empty captions
	// Telegram only shows the first item's caption for the entire album
	for _, partPath := range videoParts {
		mediaItems = append(mediaItems, telegram.MediaItem{
			FilePath:  partPath,
			MediaType: "video",
			Caption:   "",
		})
	}

	log.Printf("Uploading media group (%d items: 1 preview + %d video parts)...", len(mediaItems), len(videoParts))

	// Step 5: Upload media group
	messageID, err := uploader.SendMediaGroup(chatID, mediaItems)
	if err != nil {
		cleanup()
		return 0, nil, fmt.Errorf("failed to upload media group: %w", err)
	}

	// Step 6: Return message ID and files to save
	// Files to save: preview.jpg and optionally split parts
	var filesToSave []string
	filesToSave = append(filesToSave, previewPath)
	for _, part := range videoParts {
		if part != filePath {
			filesToSave = append(filesToSave, part)
		}
	}

	// Clean up temporary frame files (but keep preview and video parts)
	for _, frame := range frames {
		os.Remove(frame)
	}

	return messageID, filesToSave, nil
}

// MoveVideoFiles moves the original video and related files to done directory
func (p *Processor) MoveVideoFiles(originalFilename string, messageID int, additionalFiles []string) error {
	// Move original video
	sourcePath := filepath.Join(p.localDir, originalFilename)
	ext := filepath.Ext(originalFilename)
	nameWithoutExt := strings.TrimSuffix(originalFilename, ext)

	// Original video: originalname_msgid_<ID>.ext
	newFilename := fmt.Sprintf("%s_msgid_%d%s", nameWithoutExt, messageID, ext)
	destPath := filepath.Join(p.doneDir, newFilename)

	if err := moveOrCopy(sourcePath, destPath); err != nil {
		return fmt.Errorf("failed to move original video: %w", err)
	}

	// Move additional files (preview, split parts)
	for _, addFile := range additionalFiles {
		addBasename := filepath.Base(addFile)
		addExt := filepath.Ext(addFile)
		addNameWithoutExt := strings.TrimSuffix(addBasename, addExt)

		// Add message ID to filename
		newAddFilename := fmt.Sprintf("%s_msgid_%d%s", addNameWithoutExt, messageID, addExt)
		addDestPath := filepath.Join(p.doneDir, newAddFilename)

		if err := moveOrCopy(addFile, addDestPath); err != nil {
			log.Printf("WARNING: Failed to move %s: %v", addBasename, err)
		}
	}

	return nil
}

// moveOrCopy moves a file, with fallback to copy+delete
func moveOrCopy(src, dst string) error {
	// Try rename first
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Fallback to copy+delete
	if err := copyFile(src, dst); err != nil {
		return err
	}

	return os.Remove(src)
}
