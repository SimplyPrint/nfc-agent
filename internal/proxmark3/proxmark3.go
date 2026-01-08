package proxmark3

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTimeout is the default command execution timeout
	DefaultTimeout = 10 * time.Second
	// DefaultPM3Path is the default path to the pm3 binary
	DefaultPM3Path = "pm3"
)

// PM3Executor defines the interface for executing pm3 commands.
// Both Client (single-shot) and PersistentClient implement this interface.
type PM3Executor interface {
	Execute(ctx context.Context, command string) (string, error)
	IsAvailable() bool
	IsConnected() bool
	IsBusy() bool
	GetPath() string
	GetPort() string
	SetPort(port string)

	// Card operations
	GetCardInfo(ctx context.Context) (*CardInfo, error)
	ReadMifareBlock(ctx context.Context, block int, key []byte, keyType byte) ([]byte, error)
	WriteMifareBlock(ctx context.Context, block int, data []byte, key []byte, keyType byte) error
	ReadUltralightPage(ctx context.Context, page int, password []byte) ([]byte, error)
	WriteUltralightPage(ctx context.Context, page int, data []byte, password []byte) error
	ReadNDEF(ctx context.Context) ([]byte, error)
	WriteISO15693Block(ctx context.Context, block int, data []byte) error
}

// Ensure Client implements PM3Executor
var _ PM3Executor = (*Client)(nil)

// Client wraps the pm3 CLI for Proxmark3 communication
type Client struct {
	pm3Path string
	port    string        // Optional: specific serial port (e.g., /dev/ttyACM0)
	timeout time.Duration // Command timeout
	mu      sync.Mutex    // Mutex to prevent concurrent pm3 access
	busy    bool          // True when a command is executing
}

// Config holds Proxmark3 client configuration
type Config struct {
	PM3Path string        // Path to pm3 binary (default: "pm3")
	Port    string        // Specific serial port, empty for auto-detect
	Timeout time.Duration // Command timeout (default: 10s)
}

// NewClient creates a new Proxmark3 client
func NewClient(cfg Config) *Client {
	if cfg.PM3Path == "" {
		cfg.PM3Path = DefaultPM3Path
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	return &Client{
		pm3Path: cfg.PM3Path,
		port:    cfg.Port,
		timeout: cfg.Timeout,
	}
}

// IsAvailable checks if the pm3 binary is installed and accessible
func (c *Client) IsAvailable() bool {
	_, err := exec.LookPath(c.pm3Path)
	return err == nil
}

// IsConnected checks if a Proxmark3 device is connected and responding
// Returns true if device is busy (already executing a command)
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return true // Device is busy, so it must be connected
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Execute(ctx, "hw version")
	return err == nil
}

// IsBusy returns true if a command is currently executing
func (c *Client) IsBusy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy
}

// Execute runs a pm3 command and returns the output
func (c *Client) Execute(ctx context.Context, command string) (string, error) {
	c.mu.Lock()
	c.busy = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
	}()

	args := []string{"-c", command}
	if c.port != "" {
		args = append([]string{"-p", c.port}, args...)
	}

	cmd := exec.CommandContext(ctx, c.pm3Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%w: %s", ErrTimeout, command)
		}
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "executable file not found") {
			return "", ErrPM3NotFound
		}
		return "", fmt.Errorf("pm3 error: %w, stderr: %s", err, stderr.String())
	}

	output := stdout.String()

	// Check for common error patterns in output
	if err := detectOutputError(output); err != nil {
		return output, err
	}

	return output, nil
}

// ExecuteWithTimeout runs a pm3 command with a custom timeout
func (c *Client) ExecuteWithTimeout(command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.Execute(ctx, command)
}

// detectOutputError checks the pm3 output for error patterns
func detectOutputError(output string) error {
	lower := strings.ToLower(output)

	// No card errors
	noCardPatterns := []string{
		"no tag found",
		"can't select card",
		"no card",
		"tag lost",
		"iso14443a card select failed",
	}
	for _, pattern := range noCardPatterns {
		if strings.Contains(lower, pattern) {
			return ErrNoCard
		}
	}

	// Authentication errors
	authFailPatterns := []string{
		"authentication failed",
		"auth error",
		"wrong key",
		"nested authentication failed",
	}
	for _, pattern := range authFailPatterns {
		if strings.Contains(lower, pattern) {
			return ErrAuthFailed
		}
	}

	return nil
}

// GetPort returns the configured serial port
func (c *Client) GetPort() string {
	return c.port
}

// GetPath returns the pm3 binary path
func (c *Client) GetPath() string {
	return c.pm3Path
}

// SetPort sets the serial port to use
func (c *Client) SetPort(port string) {
	c.port = port
}

// DetectPM3 checks common paths for the pm3 binary and returns info about it
func DetectPM3() (string, error) {
	paths := []string{
		"pm3",                        // In PATH
		"/usr/local/bin/pm3",         // Homebrew macOS (Intel)
		"/opt/homebrew/bin/pm3",      // Homebrew macOS (Apple Silicon)
		"/usr/bin/pm3",               // Linux system
		"/opt/proxmark3/client/pm3",  // Custom install
		"/opt/proxmark3/pm3",         // Alternative custom install
	}

	for _, path := range paths {
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
	}

	return "", ErrPM3NotFound
}
