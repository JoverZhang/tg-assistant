package config

import (
	"flag"
	"fmt"
	"os"
	"tg-storage-assistant/internal/logger"
	"tg-storage-assistant/internal/util"

	"github.com/joho/godotenv"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Mtproto MtprotoConfig `yaml:"mtproto"`
	Bot     BotConfig     `yaml:"bot"`
}

type MtprotoConfig struct {
	// MTProto credentials
	SessionFile   string `yaml:"session_file"`
	APIID         int    `yaml:"api_id"`
	APIHash       string `yaml:"api_hash"`
	Phone         string `yaml:"phone"`
	StorageChatID int64  `yaml:"storage_chat_id"`

	// Proxy settings
	Proxy string `yaml:"proxy"`

	// File paths
	LocalDir       string `yaml:"local_dir"`
	TempDir        string `yaml:"temp_dir"`
	DoneDir        string `yaml:"done_dir"`
	MaxSize        string `yaml:"max_size"`         // e.g. "20MB"
	MaxSizeBytes   int64  `yaml:"-"`                // parsed from MaxSize
	CleanupTempDir bool   `yaml:"cleanup_temp_dir"` // default is true
}

type BotConfig struct {
	Token string `yaml:"token"`
	Proxy string `yaml:"proxy"`
}

func ParseConfig() (*Config, error) {
	cfg := &Config{}

	var configFile string
	flag.StringVar(&configFile, "config", "config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}
	return cfg, nil
}

func LoadConfig(path string) (*Config, error) {
	// load environment variables from .env file
	if err := godotenv.Load(); err == nil {
		logger.Info.Println("loaded environment variables from .env file")
	}

	// 1. read file
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config failed: %w", err)
	}

	// 2. expand environment variables
	expanded := os.ExpandEnv(string(raw))

	// 3. parse yaml
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml failed: %w", err)
	}

	// 4. validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if err := c.Mtproto.Validate(); err != nil {
		return fmt.Errorf("mtproto config invalid: %w", err)
	}
	if err := c.Bot.Validate(); err != nil {
		return fmt.Errorf("bot config invalid: %w", err)
	}
	return nil
}

func (c *MtprotoConfig) Validate() error {
	// parse max_size
	if c.MaxSize != "" {
		size, err := util.ParseSize(c.MaxSize)
		if err != nil {
			return fmt.Errorf("invalid mtproto.max_size: %w", err)
		}
		c.MaxSizeBytes = size
	}

	if c.APIID == 0 {
		return fmt.Errorf("api_id is required (get from https://my.telegram.org/apps)")
	}
	if c.APIHash == "" {
		return fmt.Errorf("api_hash is required (get from https://my.telegram.org/apps)")
	}
	if c.StorageChatID == 0 {
		return fmt.Errorf("storage_chat_id is required")
	}
	if c.LocalDir == "" {
		return fmt.Errorf("local_dir is required")
	}
	if c.TempDir == "" {
		return fmt.Errorf("temp_dir is required")
	}
	if c.DoneDir == "" {
		return fmt.Errorf("done_dir is required")
	}

	// phone is optional: if session file does not exist, it must be provided
	if c.Phone == "" {
		if _, err := os.Stat(c.SessionFile); os.IsNotExist(err) {
			return fmt.Errorf("phone is required for first-time authentication (session file not found: %s)", c.SessionFile)
		}
	}

	// check & create directories
	if _, err := os.Stat(c.LocalDir); os.IsNotExist(err) {
		return fmt.Errorf("local_dir does not exist: %s", c.LocalDir)
	}

	if _, err := os.Stat(c.TempDir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.TempDir, 0o755); err != nil {
			return fmt.Errorf("failed to create temp_dir: %w", err)
		}
	}

	if _, err := os.Stat(c.DoneDir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.DoneDir, 0o755); err != nil {
			return fmt.Errorf("failed to create done_dir: %w", err)
		}
	}

	return nil
}

func (c *BotConfig) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("bot.token is required (get from @BotFather)")
	}

	return nil
}
