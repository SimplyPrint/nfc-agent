package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any existing env vars
	os.Unsetenv("NFC_AGENT_PORT")
	os.Unsetenv("NFC_AGENT_HOST")

	cfg := Load()

	if cfg.Host != DefaultHost {
		t.Errorf("expected host %q, got %q", DefaultHost, cfg.Host)
	}

	if cfg.Port != DefaultPort {
		t.Errorf("expected port %d, got %d", DefaultPort, cfg.Port)
	}
}

func TestLoad_CustomPort(t *testing.T) {
	os.Setenv("NFC_AGENT_PORT", "8080")
	defer os.Unsetenv("NFC_AGENT_PORT")

	cfg := Load()

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	tests := []struct {
		name     string
		portStr  string
		expected int
	}{
		{"non-numeric", "abc", DefaultPort},
		{"negative", "-1", DefaultPort},
		{"zero", "0", DefaultPort},
		{"too high", "70000", DefaultPort},
		{"empty", "", DefaultPort},
		{"float", "3.14", DefaultPort},
		{"special chars", "!@#$", DefaultPort},
		{"leading spaces", " 8080", DefaultPort},
		{"trailing spaces", "8080 ", DefaultPort},
		{"hex", "0x1F90", DefaultPort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("NFC_AGENT_PORT", tt.portStr)
			defer os.Unsetenv("NFC_AGENT_PORT")

			cfg := Load()

			if cfg.Port != tt.expected {
				t.Errorf("expected port %d, got %d", tt.expected, cfg.Port)
			}
		})
	}
}

func TestLoad_ValidPorts(t *testing.T) {
	tests := []struct {
		name     string
		portStr  string
		expected int
	}{
		{"standard port", "8080", 8080},
		{"low port", "1024", 1024},
		{"high port", "65535", 65535},
		{"default port value", "32145", 32145},
		{"common http", "80", 80},
		{"common https", "443", 443},
		{"node default", "3000", 3000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("NFC_AGENT_PORT", tt.portStr)
			defer os.Unsetenv("NFC_AGENT_PORT")

			cfg := Load()

			if cfg.Port != tt.expected {
				t.Errorf("expected port %d, got %d", tt.expected, cfg.Port)
			}
		})
	}
}

func TestLoad_CustomHost(t *testing.T) {
	os.Setenv("NFC_AGENT_HOST", "0.0.0.0")
	defer os.Unsetenv("NFC_AGENT_HOST")

	cfg := Load()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got %q", cfg.Host)
	}
}

func TestLoad_VariousHosts(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{"localhost", "localhost", "localhost"},
		{"all interfaces", "0.0.0.0", "0.0.0.0"},
		{"loopback", "127.0.0.1", "127.0.0.1"},
		{"ipv6 loopback", "::1", "::1"},
		{"custom ip", "192.168.1.100", "192.168.1.100"},
		{"hostname", "my-server", "my-server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("NFC_AGENT_HOST", tt.host)
			defer os.Unsetenv("NFC_AGENT_HOST")

			cfg := Load()

			if cfg.Host != tt.expected {
				t.Errorf("expected host %q, got %q", tt.expected, cfg.Host)
			}
		})
	}
}

func TestAddress(t *testing.T) {
	tests := []struct {
		host     string
		port     int
		expected string
	}{
		{"127.0.0.1", 32145, "127.0.0.1:32145"},
		{"0.0.0.0", 8080, "0.0.0.0:8080"},
		{"localhost", 3000, "localhost:3000"},
		{"192.168.1.1", 443, "192.168.1.1:443"},
		{"::1", 8000, "::1:8000"},
		{"my-server.local", 9000, "my-server.local:9000"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			cfg := &Config{Host: tt.host, Port: tt.port}
			result := cfg.Address()

			if result != tt.expected {
				t.Errorf("Address() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Host: "192.168.1.1",
		Port: 9999,
	}

	if cfg.Host != "192.168.1.1" {
		t.Errorf("expected Host '192.168.1.1', got %q", cfg.Host)
	}
	if cfg.Port != 9999 {
		t.Errorf("expected Port 9999, got %d", cfg.Port)
	}
}

func TestDefaultConstants(t *testing.T) {
	// Verify default constants are set correctly
	if DefaultHost != "127.0.0.1" {
		t.Errorf("expected DefaultHost '127.0.0.1', got %q", DefaultHost)
	}
	if DefaultPort != 32145 {
		t.Errorf("expected DefaultPort 32145, got %d", DefaultPort)
	}
}

func TestLoad_BothEnvVars(t *testing.T) {
	os.Setenv("NFC_AGENT_HOST", "0.0.0.0")
	os.Setenv("NFC_AGENT_PORT", "9000")
	defer func() {
		os.Unsetenv("NFC_AGENT_HOST")
		os.Unsetenv("NFC_AGENT_PORT")
	}()

	cfg := Load()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host '0.0.0.0', got %q", cfg.Host)
	}
	if cfg.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Port)
	}

	expected := "0.0.0.0:9000"
	if cfg.Address() != expected {
		t.Errorf("expected address %q, got %q", expected, cfg.Address())
	}
}

func TestLoad_EmptyHost(t *testing.T) {
	os.Setenv("NFC_AGENT_HOST", "")
	defer os.Unsetenv("NFC_AGENT_HOST")

	cfg := Load()

	// Empty string should result in empty host (or default depending on implementation)
	// This tests the current behavior
	if cfg.Host != "" && cfg.Host != DefaultHost {
		t.Errorf("unexpected host value for empty env var: %q", cfg.Host)
	}
}

// Benchmark tests
func BenchmarkLoad(b *testing.B) {
	os.Unsetenv("NFC_AGENT_PORT")
	os.Unsetenv("NFC_AGENT_HOST")

	for i := 0; i < b.N; i++ {
		Load()
	}
}

func BenchmarkLoad_WithEnvVars(b *testing.B) {
	os.Setenv("NFC_AGENT_HOST", "0.0.0.0")
	os.Setenv("NFC_AGENT_PORT", "9000")
	defer func() {
		os.Unsetenv("NFC_AGENT_HOST")
		os.Unsetenv("NFC_AGENT_PORT")
	}()

	for i := 0; i < b.N; i++ {
		Load()
	}
}

func BenchmarkAddress(b *testing.B) {
	cfg := &Config{Host: "127.0.0.1", Port: 32145}

	for i := 0; i < b.N; i++ {
		cfg.Address()
	}
}
