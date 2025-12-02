//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const (
	serviceName     = "nfc-agent"
	serviceTemplate = `[Unit]
Description=NFC Agent - Local NFC card reader service
After=graphical-session.target

[Service]
Type=simple
ExecStart={{.ExecutablePath}} --no-tray
Restart=on-failure
RestartSec=5
Environment=DISPLAY=:0

[Install]
WantedBy=default.target
`
)

type linuxService struct{}

// New creates a new platform-specific service manager
func New() Service {
	return &linuxService{}
}

func (s *linuxService) servicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", serviceName+".service")
}

func (s *linuxService) Install() error {
	if s.IsInstalled() {
		return ErrAlreadyInstalled
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Ensure systemd user directory exists
	serviceDir := filepath.Dir(s.servicePath())
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd user directory: %w", err)
	}

	// Parse and execute template
	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse service template: %w", err)
	}

	data := struct {
		ExecutablePath string
	}{
		ExecutablePath: execPath,
	}

	// Write service file
	f, err := os.Create(s.servicePath())
	if err != nil {
		return fmt.Errorf("failed to create service file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd daemon
	if err := s.runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable the service
	if err := s.runSystemctl("enable", serviceName+".service"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	// Start the service
	if err := s.runSystemctl("start", serviceName+".service"); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

func (s *linuxService) Uninstall() error {
	if !s.IsInstalled() {
		return ErrNotInstalled
	}

	// Stop the service
	s.runSystemctl("stop", serviceName+".service")

	// Disable the service
	s.runSystemctl("disable", serviceName+".service")

	// Remove service file
	if err := os.Remove(s.servicePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd daemon
	s.runSystemctl("daemon-reload")

	return nil
}

func (s *linuxService) IsInstalled() bool {
	_, err := os.Stat(s.servicePath())
	return err == nil
}

func (s *linuxService) Status() (string, error) {
	if !s.IsInstalled() {
		return "not installed", nil
	}

	// Check if running
	cmd := exec.Command("systemctl", "--user", "is-active", serviceName+".service")
	output, _ := cmd.Output()

	status := string(output)
	if status == "active\n" || status == "active" {
		return "running", nil
	}

	return "installed but not running", nil
}

func (s *linuxService) runSystemctl(args ...string) error {
	allArgs := append([]string{"--user"}, args...)
	cmd := exec.Command("systemctl", allArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}
