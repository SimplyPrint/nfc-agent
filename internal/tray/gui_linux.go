//go:build linux

package tray

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const statusNotifierWatcher = "org.kde.StatusNotifierWatcher"

// WaitForGUI checks whether a StatusNotifierItem host (the thing that actually
// draws AppIndicator icons) is present on the session bus.
//
// It never blocks: the HTTP server is started from the tray's onReady callback,
// so waiting here would delay the API. libayatana-appindicator registers with
// the watcher on its own if one appears later (e.g. gnome-shell starting after
// us during XDG autostart), so a missing watcher is only logged as a hint.
func WaitForGUI(timeout time.Duration, retryInterval time.Duration) bool {
	logGUIEnvironment()

	present, checked := hasStatusNotifierWatcher()
	switch {
	case !checked:
		log.Println("Could not query D-Bus for a tray host (busctl/dbus-send not found); assuming one is available")
	case present:
		log.Println("System tray host (StatusNotifierWatcher) found")
	default:
		log.Println("No system tray host (StatusNotifierWatcher) on the session bus yet - the tray icon may not be visible. " +
			"On GNOME, install and enable the AppIndicator extension (gnome-shell-extension-appindicator). " +
			"The API keeps running either way.")
	}
	return true
}

// hasStatusNotifierWatcher reports whether the StatusNotifierWatcher name is owned
// on the session bus. checked is false when no D-Bus CLI tool was available.
func hasStatusNotifierWatcher() (present bool, checked bool) {
	if path, err := exec.LookPath("busctl"); err == nil {
		// busctl status exits non-zero when the name has no owner
		err := exec.Command(path, "--user", "status", statusNotifierWatcher).Run()
		return err == nil, true
	}
	if path, err := exec.LookPath("dbus-send"); err == nil {
		out, err := exec.Command(path, "--session", "--print-reply",
			"--dest=org.freedesktop.DBus", "/org/freedesktop/DBus",
			"org.freedesktop.DBus.NameHasOwner", "string:"+statusNotifierWatcher).Output()
		if err != nil {
			return false, true
		}
		return strings.Contains(string(out), "boolean true"), true
	}
	return false, false
}

// logGUIEnvironment logs environment variables relevant to tray support on Linux
func logGUIEnvironment() {
	relevantVars := []string{
		"DISPLAY",
		"WAYLAND_DISPLAY",
		"XDG_CURRENT_DESKTOP",
		"XDG_SESSION_TYPE",
		"DBUS_SESSION_BUS_ADDRESS",
	}

	var found []string
	for _, v := range relevantVars {
		if val := os.Getenv(v); val != "" {
			found = append(found, v+"="+val)
		}
	}

	if len(found) > 0 {
		log.Printf("GUI environment: %s", strings.Join(found, ", "))
	} else {
		log.Println("GUI environment: no relevant variables set")
	}
}
