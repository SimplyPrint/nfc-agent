package config

import (
	"os"
	"strconv"
	"time"
)

const (
	DefaultPort = 32145
	DefaultHost = "127.0.0.1"
)

// Config holds the application configuration.
type Config struct {
	Host      string
	Port      int
	Proxmark3 Proxmark3Config
}

// Proxmark3Config holds Proxmark3-specific configuration.
type Proxmark3Config struct {
	Enabled        bool          // Enable Proxmark3 support (NFC_AGENT_PROXMARK3=1)
	Path           string        // Custom path to pm3 binary (NFC_AGENT_PM3_PATH)
	Port           string        // Specific serial port (NFC_AGENT_PM3_PORT)
	PersistentMode bool          // Use persistent subprocess (NFC_AGENT_PM3_PERSISTENT, default: true)
	IdleTimeout    time.Duration // Idle timeout before killing subprocess (NFC_AGENT_PM3_IDLE_TIMEOUT, default: 60s)
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

	// Proxmark3 configuration
	cfg.Proxmark3 = Proxmark3Config{
		Enabled:        os.Getenv("NFC_AGENT_PROXMARK3") == "1",
		Path:           os.Getenv("NFC_AGENT_PM3_PATH"),
		Port:           os.Getenv("NFC_AGENT_PM3_PORT"),
		PersistentMode: os.Getenv("NFC_AGENT_PM3_PERSISTENT") != "0", // Default ON
		IdleTimeout:    parseIdleTimeout(os.Getenv("NFC_AGENT_PM3_IDLE_TIMEOUT")),
	}

	return cfg
}

// parseIdleTimeout parses the idle timeout from a string.
// Returns 60s by default, -1 for "never" or "-1".
func parseIdleTimeout(s string) time.Duration {
	if s == "" {
		return 60 * time.Second
	}
	if s == "-1" || s == "never" {
		return -1 // Never timeout
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 60 * time.Second
	}
	return d
}

// Address returns the formatted host:port address string.
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}
