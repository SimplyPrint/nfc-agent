//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const (
	launchAgentLabel = "com.simplyprint.nfc-agent"
	plistTemplate    = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.ExecutablePath}}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>LimitLoadToSessionType</key>
    <string>Aqua</string>
    <key>ProcessType</key>
    <string>Interactive</string>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}/nfc-agent.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}/nfc-agent.err</string>
    <key>WorkingDirectory</key>
    <string>{{.WorkingDir}}</string>
</dict>
</plist>
`
)

type darwinService struct{}

// New creates a new platform-specific service manager
func New() Service {
	return &darwinService{}
}

func (s *darwinService) plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func (s *darwinService) logPath() string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, "Library", "Logs", "NFC-Agent")
	os.MkdirAll(logDir, 0755)
	return logDir
}

func (s *darwinService) Install() error {
	if s.IsInstalled() {
		return ErrAlreadyInstalled
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the actual path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Ensure LaunchAgents directory exists
	launchAgentsDir := filepath.Dir(s.plistPath())
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	// Parse and execute template
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %w", err)
	}

	data := struct {
		Label          string
		ExecutablePath string
		LogPath        string
		WorkingDir     string
	}{
		Label:          launchAgentLabel,
		ExecutablePath: execPath,
		LogPath:        s.logPath(),
		WorkingDir:     filepath.Dir(execPath),
	}

	// Write plist file
	f, err := os.Create(s.plistPath())
	if err != nil {
		return fmt.Errorf("failed to create plist file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	// Load the launch agent
	cmd := exec.Command("launchctl", "load", "-w", s.plistPath())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to load launch agent: %s: %w", string(output), err)
	}

	return nil
}

func (s *darwinService) Uninstall() error {
	if !s.IsInstalled() {
		return ErrNotInstalled
	}

	// Unload the launch agent
	cmd := exec.Command("launchctl", "unload", "-w", s.plistPath())
	cmd.CombinedOutput() // Ignore errors if not loaded

	// Remove plist file
	if err := os.Remove(s.plistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	return nil
}

func (s *darwinService) IsInstalled() bool {
	_, err := os.Stat(s.plistPath())
	return err == nil
}

func (s *darwinService) Status() (string, error) {
	if !s.IsInstalled() {
		return "not installed", nil
	}

	// Check if running using launchctl
	cmd := exec.Command("launchctl", "list", launchAgentLabel)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "installed but not running", nil
	}

	if len(output) > 0 {
		return "running", nil
	}

	return "installed", nil
}
