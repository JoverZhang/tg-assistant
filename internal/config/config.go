package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration
type Config struct {
	Token    string
	LocalDir string
	DoneDir  string
	ChatID   int64
	MaxSize  int64 // Maximum file size for video splitting in bytes (0 = no splitting)
}

// Parse parses command-line flags and returns a Config
func Parse() (*Config, error) {
	cfg := &Config{}

	var maxSizeStr string
	flag.StringVar(&cfg.Token, "token", "", "Telegram bot token for authentication")
	flag.StringVar(&cfg.LocalDir, "local-dir", "", "Source directory path containing files to upload")
	flag.StringVar(&cfg.DoneDir, "done-dir", "", "Destination directory path for successfully uploaded files")
	flag.Int64Var(&cfg.ChatID, "chat-id", 0, "Target Telegram chat ID where files will be sent")
	flag.StringVar(&maxSizeStr, "max-size", "", "Maximum file size for video splitting (e.g., \"2G\", \"500M\", \"1.5G\")")

	flag.Parse()

	// Parse max-size if provided
	if maxSizeStr != "" {
		size, err := parseSize(maxSizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid max-size: %w", err)
		}
		cfg.MaxSize = size
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if all required configuration fields are provided and valid
func (c *Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}

	if c.LocalDir == "" {
		return fmt.Errorf("local-dir is required")
	}

	if c.DoneDir == "" {
		return fmt.Errorf("done-dir is required")
	}

	if c.ChatID == 0 {
		return fmt.Errorf("chat-id is required")
	}

	// Check if local directory exists
	if _, err := os.Stat(c.LocalDir); os.IsNotExist(err) {
		return fmt.Errorf("local-dir does not exist: %s", c.LocalDir)
	}

	// Create done directory if it doesn't exist
	if _, err := os.Stat(c.DoneDir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.DoneDir, 0755); err != nil {
			return fmt.Errorf("failed to create done-dir: %w", err)
		}
	}

	return nil
}

// parseSize parses a size string like "2G", "500M", "1.5G" to bytes
func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))
	if sizeStr == "" {
		return 0, fmt.Errorf("empty size string")
	}

	// Extract numeric part and unit
	var numStr string
	var unit string
	for i, ch := range sizeStr {
		if ch >= '0' && ch <= '9' || ch == '.' {
			numStr += string(ch)
		} else {
			unit = sizeStr[i:]
			break
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("no numeric value found")
	}

	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value: %w", err)
	}

	var multiplier int64
	switch unit {
	case "B", "":
		multiplier = 1
	case "K", "KB":
		multiplier = 1024
	case "M", "MB":
		multiplier = 1024 * 1024
	case "G", "GB":
		multiplier = 1024 * 1024 * 1024
	case "T", "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown unit: %s (use B, K/KB, M/MB, G/GB, T/TB)", unit)
	}

	return int64(value * float64(multiplier)), nil
}
