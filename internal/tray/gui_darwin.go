//go:build darwin

package tray

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WaitForGUI waits for the GUI/WindowServer to be ready.
// Uses pgrep to check for WindowServer process.
// Returns true if GUI is ready, false if timed out.
func WaitForGUI(timeout time.Duration, retryInterval time.Duration) bool {
	log.Printf("WaitForGUI called (timeout=%v, retryInterval=%v)", timeout, retryInterval)

	// Log environment for debugging
	logGUIEnvironment()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if WindowServer process exists
		cmd := exec.Command("pgrep", "-x", "WindowServer")
		if err := cmd.Run(); err == nil {
			log.Println("WindowServer process found, waiting for GUI to stabilize...")
			// Wait longer for the GUI to fully initialize
			time.Sleep(2 * time.Second)
			log.Println("WindowServer is ready")
			return true
		}
		log.Printf("Waiting for WindowServer... (%.0fs remaining)", time.Until(deadline).Seconds())
		time.Sleep(retryInterval)
	}

	log.Println("Timed out waiting for WindowServer")
	return false
}

// logGUIEnvironment logs environment variables relevant to GUI session detection
func logGUIEnvironment() {
	relevantVars := []string{
		"DISPLAY",
		"XPC_SERVICE_NAME",
		"__CFBundleIdentifier",
		"TERM_PROGRAM",
		"SSH_TTY",
		"Apple_PubSub_Socket_Render",
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
