//go:build windows

package service

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	registryKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryName = "NFCAgent"
)

type windowsService struct{}

// New creates a new platform-specific service manager
func New() Service {
	return &windowsService{}
}

func (s *windowsService) Install() error {
	if s.IsInstalled() {
		return ErrAlreadyInstalled
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve to absolute path
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Open registry key
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		registryKey,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// Set the value with --no-tray flag for background running
	value := fmt.Sprintf(`"%s" --no-tray`, execPath)
	if err := key.SetStringValue(registryName, value); err != nil {
		return fmt.Errorf("failed to set registry value: %w", err)
	}

	return nil
}

func (s *windowsService) Uninstall() error {
	if !s.IsInstalled() {
		return ErrNotInstalled
	}

	// Open registry key
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		registryKey,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// Delete the value
	if err := key.DeleteValue(registryName); err != nil {
		return fmt.Errorf("failed to delete registry value: %w", err)
	}

	return nil
}

func (s *windowsService) IsInstalled() bool {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		registryKey,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(registryName)
	return err == nil
}

func (s *windowsService) Status() (string, error) {
	if !s.IsInstalled() {
		return "not installed", nil
	}
	return "installed (will start on login)", nil
}

// UpgradeIfNeeded is a no-op on Windows (registry entry doesn't need migration)
func (s *windowsService) UpgradeIfNeeded() (bool, error) {
	return false, nil
}
