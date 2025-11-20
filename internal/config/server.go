package config

import (
	"flag"
	"fmt"
	"net/url"
)

// ServerConfig holds the application configuration
type ServerConfig struct {
	Token    string
	ProxyURL *url.URL
}

func ParseServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{}

	var proxyURLStr string

	flag.StringVar(&cfg.Token, "token", "", "Telegram bot token")
	flag.StringVar(&proxyURLStr, "proxy", "", "Proxy URL (e.g., socks5://127.0.0.1:1080 or http://127.0.0.1:8080)")
	flag.Parse()

	if proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy url: %s", proxyURLStr)
		}
		cfg.ProxyURL = proxyURL
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *ServerConfig) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("token is required (get from @BotFather)")
	}

	return nil
}
