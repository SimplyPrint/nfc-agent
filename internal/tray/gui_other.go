//go:build !darwin

package tray

import "time"

// WaitForGUI is a no-op on non-macOS platforms.
// Returns true immediately as no GUI wait is needed.
func WaitForGUI(timeout time.Duration, retryInterval time.Duration) bool {
	return true
}
