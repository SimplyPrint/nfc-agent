package proxmark3

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/logging"
)

const (
	// DefaultIdleTimeout is how long to keep the subprocess alive after last command
	DefaultIdleTimeout = 60 * time.Second
	// StartupTimeout is how long to wait for pm3 to start and show prompt
	StartupTimeout = 15 * time.Second
	// ReadTimeout is the timeout for reading individual lines
	ReadTimeout = 100 * time.Millisecond
)

// promptRegex matches the pm3 interactive prompt like "[usb] pm3 -->" or "[usb|script] pm3 -->"
var promptRegex = regexp.MustCompile(`^\[.+\]\s+pm3\s+-->`)

// readyRegex matches the pm3 ready message indicating successful connection
var readyRegex = regexp.MustCompile(`Communicating with PM3`)

// detectPort attempts to find a Proxmark3 device port.
// Returns empty string if no device found.
func detectPort() string {
	var patterns []string
	if runtime.GOOS == "darwin" {
		// macOS: Proxmark3 shows up as /dev/cu.usbmodem*
		patterns = []string{"/dev/cu.usbmodem*"}
	} else {
		// Linux: Proxmark3 shows up as /dev/ttyACM* or /dev/ttyUSB*
		patterns = []string{"/dev/ttyACM*", "/dev/ttyUSB*"}
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			// Return first match - could be improved to verify it's actually a PM3
			return match
		}
	}
	return ""
}

// PersistentClient wraps a long-running pm3 subprocess for fast command execution.
// Instead of spawning a new process for each command (~1s overhead), it keeps pm3
// running in interactive mode and sends commands via stdin.
type PersistentClient struct {
	pm3Path     string
	port        string
	timeout     time.Duration
	idleTimeout time.Duration

	// Subprocess management
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	// Line reader channel - single goroutine reads lines, sends to channel
	lines chan string

	// Concurrency control
	mu   sync.Mutex
	busy bool

	// State tracking
	running  bool
	lastUsed time.Time

	// Idle timer
	idleTimer *time.Timer

	// Shutdown coordination
	shutdownCh chan struct{}
}

// PersistentConfig holds configuration for the persistent client.
type PersistentConfig struct {
	PM3Path     string
	Port        string
	Timeout     time.Duration // Command timeout (default: 10s)
	IdleTimeout time.Duration // Idle timeout (default: 60s, -1 = never)
}

// NewPersistentClient creates a new persistent Proxmark3 client.
// The subprocess is not started until Start() is called or the first Execute().
func NewPersistentClient(cfg PersistentConfig) *PersistentClient {
	if cfg.PM3Path == "" {
		cfg.PM3Path = DefaultPM3Path
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}

	return &PersistentClient{
		pm3Path:     cfg.PM3Path,
		port:        cfg.Port,
		timeout:     cfg.Timeout,
		idleTimeout: cfg.IdleTimeout,
	}
}

// IsAvailable checks if the pm3 binary is installed and accessible.
func (p *PersistentClient) IsAvailable() bool {
	_, err := exec.LookPath(p.pm3Path)
	return err == nil
}

// IsConnected checks if the Proxmark3 device is connected and responding.
func (p *PersistentClient) IsConnected() bool {
	p.mu.Lock()
	if p.busy {
		p.mu.Unlock()
		return true // Device is busy, so it must be connected
	}
	running := p.running
	p.mu.Unlock()

	if running {
		return true // Subprocess is running, device is connected
	}

	// Try to start and verify
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := p.Execute(ctx, "hw version")
	return err == nil
}

// IsBusy returns true if a command is currently executing.
func (p *PersistentClient) IsBusy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.busy
}

// GetPath returns the pm3 binary path.
func (p *PersistentClient) GetPath() string {
	return p.pm3Path
}

// GetPort returns the configured serial port.
func (p *PersistentClient) GetPort() string {
	return p.port
}

// SetPort sets the serial port to use.
func (p *PersistentClient) SetPort(port string) {
	p.port = port
}

// Start spawns the pm3 subprocess in interactive mode.
// This is called automatically by Execute() if needed.
func (p *PersistentClient) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.startLocked(ctx)
}

func (p *PersistentClient) startLocked(ctx context.Context) error {
	if p.running {
		return nil // Already running
	}

	// Auto-detect port if not specified
	// PM3 interactive mode requires explicit port (unlike single-shot -c mode)
	port := p.port
	if port == "" {
		port = detectPort()
		if port != "" {
			p.port = port // Remember for future use
		}
	}

	args := []string{}
	if port != "" {
		args = append(args, "-p", port)
	}
	// No "-c" flag - run in interactive mode

	// Don't use CommandContext - we manage the subprocess lifecycle ourselves
	// The context is only used for the startup timeout, not for killing the process
	p.cmd = exec.Command(p.pm3Path, args...)

	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// We don't need stderr for now, but capture it to prevent blocking
	p.cmd.Stderr = io.Discard

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pm3: %w", err)
	}

	p.running = true
	p.lastUsed = time.Now()
	p.shutdownCh = make(chan struct{})
	p.lines = make(chan string, 100)

	// Start a single reader goroutine - this avoids race conditions
	// from multiple concurrent reads on the same bufio.Reader
	go p.readLines()

	// Wait for pm3 to connect (look for "Communicating with PM3" message)
	// Need to temporarily release lock for reading
	p.mu.Unlock()
	err = p.waitForReady(StartupTimeout)
	p.mu.Lock()

	if err != nil {
		p.killLocked()
		return fmt.Errorf("pm3 failed to start: %w", err)
	}

	logging.Info(logging.CatReader, "Proxmark3 persistent subprocess started", map[string]any{
		"pm3Path": p.pm3Path,
		"pm3Port": p.port,
	})

	// Start idle timeout
	p.resetIdleTimerLocked()

	// Start process monitor goroutine
	go p.monitorProcess()

	return nil
}

// Stop gracefully shuts down the pm3 subprocess.
func (p *PersistentClient) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.stopLocked()
}

func (p *PersistentClient) stopLocked() error {
	if !p.running {
		return nil
	}

	logging.Info(logging.CatReader, "Stopping Proxmark3 persistent subprocess", nil)

	// Stop idle timer
	if p.idleTimer != nil {
		p.idleTimer.Stop()
		p.idleTimer = nil
	}

	// Close shutdown channel to signal monitor goroutine
	if p.shutdownCh != nil {
		close(p.shutdownCh)
		p.shutdownCh = nil
	}

	// Send quit command
	if p.stdin != nil {
		io.WriteString(p.stdin, "quit\n")
		p.stdin.Close()
		p.stdin = nil
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		if p.cmd != nil && p.cmd.Process != nil {
			done <- p.cmd.Wait()
		} else {
			done <- nil
		}
	}()

	select {
	case <-done:
		// Process exited cleanly
	case <-time.After(3 * time.Second):
		// Force kill
		p.killLocked()
	}

	p.running = false
	p.cmd = nil
	p.stdout = nil
	p.lines = nil

	return nil
}

func (p *PersistentClient) killLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	p.running = false
}

// Execute runs a command on the persistent pm3 subprocess.
// Falls back to single-shot mode if persistent mode fails.
func (p *PersistentClient) Execute(ctx context.Context, command string) (string, error) {
	p.mu.Lock()

	// Try to start/ensure subprocess is running
	if !p.running {
		if err := p.startLocked(ctx); err != nil {
			p.mu.Unlock()
			logging.Debug(logging.CatReader, "Persistent mode failed to start, falling back to single-shot", map[string]any{
				"error": err.Error(),
			})
			return p.executeSingleShot(ctx, command)
		}
	}

	p.busy = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.busy = false
		p.lastUsed = time.Now()
		p.resetIdleTimerLocked()
		p.mu.Unlock()
	}()

	// Send command
	_, err := fmt.Fprintf(p.stdin, "%s\n", command)
	if err != nil {
		// Subprocess died, try to restart
		p.mu.Lock()
		p.killLocked()
		p.mu.Unlock()

		logging.Debug(logging.CatReader, "Failed to write to pm3 stdin, falling back to single-shot", map[string]any{
			"error": err.Error(),
		})
		return p.executeSingleShot(ctx, command)
	}

	// Calculate timeout from context or default
	timeout := p.timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	// Read response until next prompt
	output, err := p.readUntilDone(timeout)
	if err != nil {
		// Process hung or died, kill and fall back
		p.mu.Lock()
		p.killLocked()
		p.mu.Unlock()

		logging.Debug(logging.CatReader, "Failed to read pm3 output, falling back to single-shot", map[string]any{
			"error": err.Error(),
		})
		return p.executeSingleShot(ctx, command)
	}

	// Check for error patterns in output
	if err := detectOutputError(output); err != nil {
		return output, err
	}

	return output, nil
}

// waitForReady waits for pm3 to output the "Communicating with PM3" message.
func (p *PersistentClient) waitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for pm3 to start")
		}

		line, err := p.readLineWithTimeout(ReadTimeout)
		if err != nil {
			if isTimeoutError(err) {
				continue // Keep trying until deadline
			}
			return err
		}

		// Check if pm3 is ready
		if readyRegex.MatchString(line) {
			return nil
		}

		// Check for error messages
		if strings.Contains(strings.ToLower(line), "error") {
			return fmt.Errorf("pm3 error during startup: %s", line)
		}
	}
}

// readUntilDone reads from stdout until there's been no output for silenceTimeout,
// or until the overall deadline is reached.
// PM3 doesn't output a standalone prompt after commands - it just stops outputting.
func (p *PersistentClient) readUntilDone(timeout time.Duration) (string, error) {
	return p.readUntilDoneWithSilence(timeout, 500*time.Millisecond)
}

// readUntilDoneWithSilence reads until silence, with configurable silence timeout.
func (p *PersistentClient) readUntilDoneWithSilence(timeout time.Duration, silenceTimeout time.Duration) (string, error) {
	var output bytes.Buffer
	deadline := time.Now().Add(timeout)
	gotOutput := false

	for {
		if time.Now().After(deadline) {
			if gotOutput {
				// We got some output but hit deadline - return what we have
				return strings.TrimSpace(output.String()), nil
			}
			return output.String(), fmt.Errorf("%w: no response", ErrTimeout)
		}

		// Use shorter timeout as silence detection
		line, err := p.readLineWithTimeout(silenceTimeout)
		if err != nil {
			if isTimeoutError(err) {
				if gotOutput {
					// We got output and then silence - command is done
					return strings.TrimSpace(output.String()), nil
				}
				// No output yet, keep waiting
				continue
			}
			return output.String(), err
		}

		// Skip empty lines and command echo lines
		if line == "" {
			continue
		}
		if promptRegex.MatchString(line) {
			// This is the command echo line (e.g., "[usb|script] pm3 --> hf 15 reader")
			continue
		}

		gotOutput = true
		output.WriteString(line)
		output.WriteString("\n")
	}
}

// readLines is a goroutine that continuously reads lines from stdout
// and sends them to the lines channel. This avoids race conditions
// from multiple concurrent reads on the same reader.
func (p *PersistentClient) readLines() {
	reader := bufio.NewReader(p.stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF or error - close channel and exit
			close(p.lines)
			return
		}
		p.lines <- strings.TrimRight(line, "\r\n")
	}
}

// readLineWithTimeout reads a line with a timeout from the lines channel.
func (p *PersistentClient) readLineWithTimeout(timeout time.Duration) (string, error) {
	select {
	case line, ok := <-p.lines:
		if !ok {
			return "", io.EOF
		}
		return line, nil
	case <-time.After(timeout):
		return "", &timeoutError{}
	}
}

// timeoutError represents a read timeout.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "read timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func isTimeoutError(err error) bool {
	if te, ok := err.(interface{ Timeout() bool }); ok {
		return te.Timeout()
	}
	return false
}

// executeSingleShot falls back to the original -c mode.
func (p *PersistentClient) executeSingleShot(ctx context.Context, command string) (string, error) {
	// Wait a bit for the port to be released after killing the subprocess
	time.Sleep(500 * time.Millisecond)

	args := []string{"-c", command}
	if p.port != "" {
		args = append([]string{"-p", p.port}, args...)
	}

	cmd := exec.CommandContext(ctx, p.pm3Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%w: %s", ErrTimeout, command)
		}
		if strings.Contains(err.Error(), "executable file not found") {
			return "", ErrPM3NotFound
		}
		// Include more context in error
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = "(no stderr output - port may be busy)"
		}
		return "", fmt.Errorf("pm3 error: %w, stderr: %s", err, errMsg)
	}

	output := stdout.String()
	if err := detectOutputError(output); err != nil {
		return output, err
	}

	return output, nil
}

// resetIdleTimerLocked resets the idle timeout timer.
// Must be called with p.mu held.
func (p *PersistentClient) resetIdleTimerLocked() {
	if p.idleTimer != nil {
		p.idleTimer.Stop()
	}

	if p.idleTimeout < 0 {
		return // No timeout (-1 means never)
	}

	p.idleTimer = time.AfterFunc(p.idleTimeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		if p.busy {
			// Don't kill while busy, reschedule
			p.resetIdleTimerLocked()
			return
		}

		if !p.running {
			return
		}

		logging.Info(logging.CatReader, "Shutting down idle Proxmark3 subprocess", map[string]any{
			"idleFor": time.Since(p.lastUsed).String(),
		})

		p.stopLocked()
	})
}

// monitorProcess watches for unexpected subprocess exit.
func (p *PersistentClient) monitorProcess() {
	if p.cmd == nil {
		return
	}

	// Wait for process to exit
	err := p.cmd.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if this was an expected shutdown
	select {
	case <-p.shutdownCh:
		// Expected shutdown, ignore
		return
	default:
		// Unexpected exit
		if p.running {
			logging.Warn(logging.CatReader, "Proxmark3 subprocess exited unexpectedly", map[string]any{
				"error": fmt.Sprintf("%v", err),
			})
			p.running = false
		}
	}
}

// ExecuteWithTimeout runs a pm3 command with a custom timeout.
func (p *PersistentClient) ExecuteWithTimeout(command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return p.Execute(ctx, command)
}

// ExecuteFast runs a command with a shorter silence timeout for fast-returning commands.
// Use this for write commands that return quickly and don't need 500ms silence detection.
func (p *PersistentClient) ExecuteFast(ctx context.Context, command string) (string, error) {
	p.mu.Lock()

	// Try to start/ensure subprocess is running
	if !p.running {
		if err := p.startLocked(ctx); err != nil {
			p.mu.Unlock()
			logging.Debug(logging.CatReader, "Persistent mode failed to start, falling back to single-shot", map[string]any{
				"error": err.Error(),
			})
			return p.executeSingleShot(ctx, command)
		}
	}

	p.busy = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.busy = false
		p.lastUsed = time.Now()
		p.resetIdleTimerLocked()
		p.mu.Unlock()
	}()

	// Send command
	_, err := fmt.Fprintf(p.stdin, "%s\n", command)
	if err != nil {
		// Subprocess died, try to restart
		p.mu.Lock()
		p.killLocked()
		p.mu.Unlock()

		logging.Debug(logging.CatReader, "Failed to write to pm3 stdin, falling back to single-shot", map[string]any{
			"error": err.Error(),
		})
		return p.executeSingleShot(ctx, command)
	}

	// Calculate timeout from context or default
	timeout := p.timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	// Read response with shorter silence timeout (100ms instead of 500ms)
	output, err := p.readUntilDoneWithSilence(timeout, 100*time.Millisecond)
	if err != nil {
		// Process hung or died, kill and fall back
		p.mu.Lock()
		p.killLocked()
		p.mu.Unlock()

		logging.Debug(logging.CatReader, "Failed to read pm3 output, falling back to single-shot", map[string]any{
			"error": err.Error(),
		})
		return p.executeSingleShot(ctx, command)
	}

	// Check for error patterns in output
	if err := detectOutputError(output); err != nil {
		return output, err
	}

	return output, nil
}

// =============================================================================
// Card Operation Methods (mirror Client methods from commands.go)
// =============================================================================

// GetCardInfo reads card UID and type information.
func (p *PersistentClient) GetCardInfo(ctx context.Context) (*CardInfo, error) {
	// Try ISO 15693 first - it's faster (~1.3s) and common for industrial NFC tags
	output15, err15 := p.Execute(ctx, "hf 15 reader")
	if err15 == nil {
		if info, parseErr := ParseHF15Info(output15); parseErr == nil {
			return info, nil
		}
	}

	// Fall back to ISO 14443A (MIFARE, NTAG, etc.)
	output14a, err14a := p.Execute(ctx, "hf 14a reader")
	if err14a == nil {
		if info, parseErr := ParseHF14AInfo(output14a); parseErr == nil {
			return info, nil
		}
	}

	// Both protocols failed
	if err15 != nil && err14a != nil {
		return nil, fmt.Errorf("failed to get card info: %w", err15)
	}

	return nil, fmt.Errorf("failed to get card info: %w", ErrNoCard)
}

// ReadMifareBlock reads a 16-byte block from a MIFARE Classic card.
func (p *PersistentClient) ReadMifareBlock(ctx context.Context, block int, key []byte, keyType byte) ([]byte, error) {
	if len(key) != 6 {
		return nil, fmt.Errorf("key must be 6 bytes, got %d", len(key))
	}

	keyHex := hex.EncodeToString(key)
	keyFlag := "-a"
	if keyType == 'B' || keyType == 'b' {
		keyFlag = "-b"
	}

	cmd := fmt.Sprintf("hf mf rdbl --blk %d -k %s %s", block, keyHex, keyFlag)
	output, err := p.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return ParseBlockData(output)
}

// WriteMifareBlock writes 16 bytes to a MIFARE Classic block.
func (p *PersistentClient) WriteMifareBlock(ctx context.Context, block int, data []byte, key []byte, keyType byte) error {
	if len(data) != 16 {
		return fmt.Errorf("data must be 16 bytes, got %d", len(data))
	}
	if len(key) != 6 {
		return fmt.Errorf("key must be 6 bytes, got %d", len(key))
	}

	dataHex := hex.EncodeToString(data)
	keyHex := hex.EncodeToString(key)
	keyFlag := "-a"
	if keyType == 'B' || keyType == 'b' {
		keyFlag = "-b"
	}

	cmd := fmt.Sprintf("hf mf wrbl --blk %d -k %s %s -d %s", block, keyHex, keyFlag, dataHex)
	// Use ExecuteFast for writes - they return quickly
	output, err := p.ExecuteFast(ctx, cmd)
	if err != nil {
		return err
	}

	if !IsWriteSuccess(output) {
		return fmt.Errorf("%w: %s", ErrWriteFailed, strings.TrimSpace(output))
	}

	return nil
}

// ReadUltralightPage reads a 4-byte page from a MIFARE Ultralight/NTAG card.
func (p *PersistentClient) ReadUltralightPage(ctx context.Context, page int, password []byte) ([]byte, error) {
	var cmd string
	if len(password) == 4 {
		cmd = fmt.Sprintf("hf mfu rdbl -b %d -k %s", page, hex.EncodeToString(password))
	} else {
		cmd = fmt.Sprintf("hf mfu rdbl -b %d", page)
	}

	output, err := p.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return ParseMFUPage(output)
}

// WriteUltralightPage writes 4 bytes to a MIFARE Ultralight/NTAG page.
func (p *PersistentClient) WriteUltralightPage(ctx context.Context, page int, data []byte, password []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("data must be 4 bytes, got %d", len(data))
	}

	var cmd string
	dataHex := hex.EncodeToString(data)
	if len(password) == 4 {
		cmd = fmt.Sprintf("hf mfu wrbl -b %d -d %s -k %s", page, dataHex, hex.EncodeToString(password))
	} else {
		cmd = fmt.Sprintf("hf mfu wrbl -b %d -d %s", page, dataHex)
	}

	// Use ExecuteFast for writes - they return quickly
	output, err := p.ExecuteFast(ctx, cmd)
	if err != nil {
		return err
	}

	if !IsWriteSuccess(output) {
		return fmt.Errorf("%w: %s", ErrWriteFailed, strings.TrimSpace(output))
	}

	return nil
}

// ReadNDEF reads NDEF data from a MIFARE Ultralight/NTAG card.
func (p *PersistentClient) ReadNDEF(ctx context.Context) ([]byte, error) {
	output, err := p.Execute(ctx, "hf mfu ndefread")
	if err != nil {
		return nil, err
	}

	// The ndefread command outputs hex data - parse it
	lines := strings.Split(output, "\n")
	var ndefData []byte

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") && len(line) > 0 {
			cleaned := strings.ReplaceAll(line, " ", "")
			if data, err := hex.DecodeString(cleaned); err == nil {
				ndefData = append(ndefData, data...)
			}
		}
	}

	if len(ndefData) == 0 {
		return nil, fmt.Errorf("%w: no NDEF data found", ErrParseError)
	}

	return ndefData, nil
}

// WriteISO15693Block writes a 4-byte block to an ISO 15693 tag
func (p *PersistentClient) WriteISO15693Block(ctx context.Context, block int, data []byte) error {
	if len(data) != 4 {
		return fmt.Errorf("ISO 15693 block must be exactly 4 bytes, got %d", len(data))
	}

	// Use hf 15 wrbl command with -* to scan for tag
	// Format: hf 15 wrbl -* -b <block> -d <data>
	cmd := fmt.Sprintf("hf 15 wrbl -* -b %d -d %s", block, strings.ToUpper(hex.EncodeToString(data)))
	// Use ExecuteFast for writes - they return quickly and don't need 500ms silence timeout
	output, err := p.ExecuteFast(ctx, cmd)
	if err != nil {
		return err
	}

	// Check for error patterns in output
	lower := strings.ToLower(output)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "no tag found") {
		return fmt.Errorf("write failed: %s", output)
	}

	return nil
}

// Ensure PersistentClient implements PM3Executor
var _ PM3Executor = (*PersistentClient)(nil)
