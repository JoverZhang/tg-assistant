package config

import (
	"flag"
	"fmt"
	"os"
)

// Config holds the application configuration
type Config struct {
	Token    string
	LocalDir string
	DoneDir  string
	ChatID   int64
}

// Parse parses command-line flags and returns a Config
func Parse() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.Token, "token", "", "Telegram bot token for authentication")
	flag.StringVar(&cfg.LocalDir, "local-dir", "", "Source directory path containing files to upload")
	flag.StringVar(&cfg.DoneDir, "done-dir", "", "Destination directory path for successfully uploaded files")
	flag.Int64Var(&cfg.ChatID, "chat-id", 0, "Target Telegram chat ID where files will be sent")

	flag.Parse()

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
