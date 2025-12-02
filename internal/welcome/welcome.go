package welcome

import (
	"os"
	"path/filepath"
)

const markerFileName = ".nfc-agent-welcomed"

// getMarkerPath returns the path to the first-run marker file
func getMarkerPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "nfc-agent", markerFileName), nil
}

// IsFirstRun checks if this is the first time the app is running
func IsFirstRun() bool {
	markerPath, err := getMarkerPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(markerPath)
	return os.IsNotExist(err)
}

// MarkAsShown creates the marker file to indicate welcome has been shown
func MarkAsShown() error {
	markerPath, err := getMarkerPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(markerPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Create marker file
	f, err := os.Create(markerPath)
	if err != nil {
		return err
	}
	return f.Close()
}
