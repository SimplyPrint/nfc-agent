//go:build darwin

package tray

import (
	"log"
	"os/exec"
	"time"
)

// WaitForGUI waits for the GUI/WindowServer to be ready.
// Uses pgrep to check for WindowServer process.
// Returns true if GUI is ready, false if timed out.
func WaitForGUI(timeout time.Duration, retryInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if WindowServer process exists
		cmd := exec.Command("pgrep", "-x", "WindowServer")
		if err := cmd.Run(); err == nil {
			// WindowServer is running, give it a moment to be fully ready
			time.Sleep(500 * time.Millisecond)
			log.Println("WindowServer is ready")
			return true
		}
		log.Printf("Waiting for WindowServer... (%.0fs remaining)", time.Until(deadline).Seconds())
		time.Sleep(retryInterval)
	}

	log.Println("Timed out waiting for WindowServer")
	return false
}
