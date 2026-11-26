//go:build !notray

package tray

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
*/
import "C"

import (
	"log"
	"os"
)

// IsSupported reports whether the tray can be started, i.e. a graphical
// display is actually usable.
//
// systray calls gtk_init(), which terminates the whole process when the display
// cannot be opened (stale SSH-forwarded DISPLAY, missing Xauthority after
// sudo/su, DISPLAY inherited into a container, ...). gtk_init_check() performs
// the same initialisation but returns FALSE instead, so we can fall back to
// headless mode. A successful check leaves GTK initialised, and systray's later
// gtk_init() call is then a no-op.
//
// This must run on the main OS thread, like the rest of GTK. That holds because
// the systray package locks the main goroutine to the main thread in its init()
// and IsSupported is called from main().
func IsSupported() bool {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	if C.gtk_init_check(nil, nil) == C.FALSE {
		log.Println("DISPLAY/WAYLAND_DISPLAY is set but the display could not be opened")
		return false
	}
	return true
}
