//go:build !notray && !linux

package tray

// IsSupported returns true if the system tray is supported on this platform.
func IsSupported() bool {
	return true
}
