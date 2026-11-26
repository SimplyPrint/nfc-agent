//go:build !notray

package tray

import "os"

// IsSupported reports whether a graphical session is available.
// GTK aborts the whole process if it is initialised without a display, so the
// tray must only be started when DISPLAY (X11) or WAYLAND_DISPLAY is set.
func IsSupported() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}
