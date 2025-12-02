package config

import (
	"os"
	"strconv"
)

const (
	DefaultPort = 32145
	DefaultHost = "127.0.0.1"
)

// Config holds the application configuration.
type Config struct {
	Host string
	Port int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	cfg := &Config{
		Host: DefaultHost,
		Port: DefaultPort,
	}

	// NFC_AGENT_PORT - override the default port
	if portStr := os.Getenv("NFC_AGENT_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port < 65536 {
			cfg.Port = port
		}
	}

	// NFC_AGENT_HOST - override the default host (rarely needed, localhost is safest)
	if host := os.Getenv("NFC_AGENT_HOST"); host != "" {
		cfg.Host = host
	}

	return cfg
}

// Address returns the formatted host:port address string.
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}
