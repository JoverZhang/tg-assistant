package fileprocessor

import (
	"fmt"
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

// GetFilePath returns the full path to a file in the local directory
func (p *Processor) GetFilePath(filename string) string {
	return filepath.Join(p.localDir, filename)
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
