//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
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

	// Disable the packaged systemd user unit if enabled. No --now: the agent
	// may be running under it and would otherwise stop itself mid-request.
	if s.systemdUnitEnabled() {
		_ = exec.Command("systemctl", "--user", "disable", appName+".service").Run()
	}

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
	// The .deb/.rpm ship a systemd user unit (/usr/lib/systemd/user) that users
	// may have enabled. Treat that as auto-start too, so we don't add an XDG
	// autostart entry on top of it and start two agents at login.
	return s.systemdUnitEnabled()
}

// systemdUnitEnabled reports whether the nfc-agent systemd user unit is enabled
// for this user (packaged or legacy).
func (s *linuxService) systemdUnitEnabled() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "systemctl", "--user", "is-enabled", "--quiet", appName+".service").Run() == nil
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

	unitEnabled := s.systemdUnitEnabled()

	if !autostartExists && !systemdExists && !unitEnabled {
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
	} else if unitEnabled {
		methods = append(methods, "systemd")
	}
	return fmt.Sprintf("installed (%s) but not running", strings.Join(methods, ", ")), nil
}

// UpgradeIfNeeded is a no-op on Linux (desktop entry doesn't need migration)
func (s *linuxService) UpgradeIfNeeded() (bool, error) {
	return false, nil
}
