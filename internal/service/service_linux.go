//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	appName = "nfc-agent"

	// XDG Autostart desktop entry - runs as part of graphical session
	// This ensures proper polkit authorization (recognized as "active" session)
	desktopTemplate = `[Desktop Entry]
Type=Application
Name=NFC Agent
Comment=Local NFC card reader service for web applications
Exec={{.ExecutablePath}}
Icon=nfc-agent
Terminal=false
Categories=Utility;
StartupNotify=false
X-GNOME-Autostart-enabled=true
`

	// Legacy systemd user service (kept for headless/server environments)
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

func (s *linuxService) autostartPath() string {
	// XDG autostart directory
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "autostart", appName+".desktop")
}

func (s *linuxService) systemdServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", appName+".service")
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

	// Use XDG autostart for graphical sessions (works with polkit)
	if err := s.installAutostart(execPath); err != nil {
		return err
	}

	return nil
}

func (s *linuxService) installAutostart(execPath string) error {
	// Ensure autostart directory exists
	autostartDir := filepath.Dir(s.autostartPath())
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("failed to create autostart directory: %w", err)
	}

	// Parse and execute template
	tmpl, err := template.New("desktop").Parse(desktopTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse desktop template: %w", err)
	}

	data := struct {
		ExecutablePath string
	}{
		ExecutablePath: execPath,
	}

	// Write desktop file
	f, err := os.Create(s.autostartPath())
	if err != nil {
		return fmt.Errorf("failed to create autostart file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write autostart file: %w", err)
	}

	return nil
}

func (s *linuxService) Uninstall() error {
	if !s.IsInstalled() {
		return ErrNotInstalled
	}

	// Remove XDG autostart file
	if err := os.Remove(s.autostartPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove autostart file: %w", err)
	}

	// Also clean up legacy systemd service if present
	s.cleanupLegacySystemd()

	return nil
}

func (s *linuxService) cleanupLegacySystemd() {
	servicePath := s.systemdServicePath()
	if _, err := os.Stat(servicePath); err == nil {
		// Stop and disable the systemd service
		exec.Command("systemctl", "--user", "stop", appName+".service").Run()
		exec.Command("systemctl", "--user", "disable", appName+".service").Run()
		os.Remove(servicePath)
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
}

func (s *linuxService) IsInstalled() bool {
	// Check XDG autostart
	if _, err := os.Stat(s.autostartPath()); err == nil {
		return true
	}
	// Also check legacy systemd service
	if _, err := os.Stat(s.systemdServicePath()); err == nil {
		return true
	}
	return false
}

func (s *linuxService) Status() (string, error) {
	autostartExists := false
	if _, err := os.Stat(s.autostartPath()); err == nil {
		autostartExists = true
	}

	systemdExists := false
	if _, err := os.Stat(s.systemdServicePath()); err == nil {
		systemdExists = true
	}

	if !autostartExists && !systemdExists {
		return "not installed", nil
	}

	// Check if process is running
	cmd := exec.Command("pgrep", "-x", appName)
	if err := cmd.Run(); err == nil {
		if autostartExists {
			return "running (autostart)", nil
		}
		return "running (systemd)", nil
	}

	// Installed but not running
	var methods []string
	if autostartExists {
		methods = append(methods, "autostart")
	}
	if systemdExists {
		methods = append(methods, "systemd (legacy)")
	}
	return fmt.Sprintf("installed (%s) but not running", strings.Join(methods, ", ")), nil
}

// UpgradeIfNeeded is a no-op on Linux (desktop entry doesn't need migration)
func (s *linuxService) UpgradeIfNeeded() (bool, error) {
	return false, nil
}
