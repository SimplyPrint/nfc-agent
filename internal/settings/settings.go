package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings holds user preferences that persist across restarts.
type Settings struct {
	CrashReporting  bool `json:"crashReporting"`  // Whether to send crash reports to Sentry
	ExtendedLogging bool `json:"extendedLogging"` // Whether to log detailed APDU traffic for debugging
}

var (
	current  *Settings
	mu       sync.RWMutex
	filePath string
)

// DefaultSettings returns the default settings.
func DefaultSettings() *Settings {
	return &Settings{
		CrashReporting: false, // Opt-in, disabled by default
	}
}

// getSettingsPath returns the path to the settings file.
func getSettingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "nfc-agent", "settings.json"), nil
}

// Load reads settings from disk, or returns defaults if file doesn't exist.
func Load() (*Settings, error) {
	mu.Lock()
	defer mu.Unlock()

	path, err := getSettingsPath()
	if err != nil {
		current = DefaultSettings()
		return current, err
	}
	filePath = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			current = DefaultSettings()
			return current, nil
		}
		current = DefaultSettings()
		return current, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		current = DefaultSettings()
		return current, err
	}

	current = &s
	return current, nil
}

// Save writes the current settings to disk.
func Save() error {
	mu.Lock()
	defer mu.Unlock()

	if current == nil {
		current = DefaultSettings()
	}

	path, err := getSettingsPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Get returns the current settings (loads from disk if not yet loaded).
func Get() *Settings {
	mu.RLock()
	if current != nil {
		defer mu.RUnlock()
		return current
	}
	mu.RUnlock()

	// Not loaded yet, load now
	s, _ := Load()
	return s
}

// SetCrashReporting updates the crash reporting preference and saves.
func SetCrashReporting(enabled bool) error {
	mu.Lock()
	if current == nil {
		current = DefaultSettings()
	}
	current.CrashReporting = enabled
	mu.Unlock()

	return Save()
}

// IsCrashReportingEnabled returns whether crash reporting is enabled.
func IsCrashReportingEnabled() bool {
	return Get().CrashReporting
}

// SetExtendedLogging updates the extended logging preference and saves.
func SetExtendedLogging(enabled bool) error {
	mu.Lock()
	if current == nil {
		current = DefaultSettings()
	}
	current.ExtendedLogging = enabled
	mu.Unlock()

	return Save()
}

// IsExtendedLoggingEnabled returns whether extended APDU logging is enabled.
func IsExtendedLoggingEnabled() bool {
	return Get().ExtendedLogging
}
