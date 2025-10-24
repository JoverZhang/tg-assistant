package fileprocessor

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
