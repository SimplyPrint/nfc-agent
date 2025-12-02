//go:build darwin

package welcome

import (
	"os/exec"
)

const welcomeTitle = "NFC Agent"
const welcomeMessage = `NFC Agent is now running!

The app runs quietly in your menu bar and provides an API that allows SimplyPrint.io and other applications to communicate with your NFC readers.

You can access the status page at:
http://127.0.0.1:32145

Click the menu bar icon anytime to check status or quit.`

const aboutMessage = `NFC Agent

A lightweight background service that enables web applications like SimplyPrint.io to communicate with NFC readers connected to your computer.

Features:
- Automatic NFC reader detection
- Secure local API (127.0.0.1 only)
- Cross-platform support

Status page: http://127.0.0.1:32145

© SimplyPrint ApS`

// ShowWelcome displays a native welcome dialog on macOS
func ShowWelcome() {
	script := `display dialog "` + escapeAppleScript(welcomeMessage) + `" with title "` + welcomeTitle + `" buttons {"Got it!"} default button 1 with icon note`
	exec.Command("osascript", "-e", script).Run()
}

// ShowAbout displays a native about dialog on macOS
func ShowAbout(version string) {
	msg := aboutMessage + "\nVersion: " + version
	script := `display dialog "` + escapeAppleScript(msg) + `" with title "About NFC Agent" buttons {"OK"} default button 1 with icon note`
	exec.Command("osascript", "-e", script).Run()
}

func escapeAppleScript(s string) string {
	result := ""
	for _, c := range s {
		if c == '"' {
			result += `\"`
		} else if c == '\\' {
			result += `\\`
		} else {
			result += string(c)
		}
	}
	return result
}
