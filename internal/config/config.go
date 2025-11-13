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
	// MTProto credentials
	SessionFile   string
	APIID         int
	APIHash       string
	Phone         string
	StorageChatID int64

	// Proxy settings
	ProxyURL string

	// File paths
	LocalDir string
	TempDir  string
	DoneDir  string
	MaxSize  int64 // Maximum file size for video splitting in bytes (0 = no splitting)
}

// Parse parses command-line flags and returns a Config
func Parse() (*Config, error) {
	cfg := &Config{}

	var maxSizeStr string

	// MTProto flags
	flag.StringVar(&cfg.SessionFile, "session-file", "./session.json", "Path to MTProto session file")
	flag.IntVar(&cfg.APIID, "api-id", 0, "Telegram API ID (from https://my.telegram.org/apps)")
	flag.StringVar(&cfg.APIHash, "api-hash", "", "Telegram API hash")
	flag.StringVar(&cfg.Phone, "phone", "", "Phone number for authentication (e.g., +1234567890)")
	flag.Int64Var(&cfg.StorageChatID, "storage-chat-id", 0, "Storage chat ID where files will be uploaded")

	// Proxy flags
	flag.StringVar(&cfg.ProxyURL, "proxy", "", "Proxy URL (e.g., socks5://127.0.0.1:1080 or http://127.0.0.1:8080)")

	// File path flags
	flag.StringVar(&cfg.LocalDir, "local-dir", "", "Source directory path containing files to upload")
	flag.StringVar(&cfg.TempDir, "temp-dir", "", "Temporary directory path for video processing")
	flag.StringVar(&cfg.DoneDir, "done-dir", "", "Destination directory path for successfully uploaded files")
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
	// Check for MTProto credentials
	if c.APIID == 0 {
		return fmt.Errorf("api-id is required (get from https://my.telegram.org/apps)")
	}

	if c.APIHash == "" {
		return fmt.Errorf("api-hash is required (get from https://my.telegram.org/apps)")
	}

	if c.StorageChatID == 0 {
		return fmt.Errorf("storage-chat-id is required")
	}

	if c.LocalDir == "" {
		return fmt.Errorf("local-dir is required")
	}

	if c.TempDir == "" {
		return fmt.Errorf("temp-dir is required")
	}

	if c.DoneDir == "" {
		return fmt.Errorf("done-dir is required")
	}

	// Phone is optional if session file already exists
	if c.Phone == "" {
		if _, err := os.Stat(c.SessionFile); os.IsNotExist(err) {
			return fmt.Errorf("phone is required for first-time authentication (session file not found)")
		}
	}

	// Check if local directory exists
	if _, err := os.Stat(c.LocalDir); os.IsNotExist(err) {
		return fmt.Errorf("local-dir does not exist: %s", c.LocalDir)
	}

	// Create temp directory if it doesn't exist
	if _, err := os.Stat(c.TempDir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.TempDir, 0755); err != nil {
			return fmt.Errorf("failed to create temp-dir: %w", err)
		}
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
