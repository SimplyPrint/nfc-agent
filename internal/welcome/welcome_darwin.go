//go:build darwin

package welcome

import (
	"os/exec"
	"strings"
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

const autostartPromptMessage = `Would you like NFC Agent to start automatically when you log in?

This ensures the agent is always available for SimplyPrint.io and other applications.

You can change this later in the status page settings.`

// PromptAutostart shows a dialog asking if the user wants to enable auto-start.
// Returns true if the user clicked "Yes".
func PromptAutostart() bool {
	script := `display dialog "` + escapeAppleScript(autostartPromptMessage) + `" with title "NFC Agent" buttons {"No", "Yes"} default button 2 with icon note`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Yes")
}

const crashReportingPromptMessage = `Help improve NFC Agent by sending anonymous crash reports?

If the app crashes, diagnostic information will be sent to help us fix bugs faster. No personal data is collected.

You can change this later in the status page settings.`

// PromptCrashReporting shows a dialog asking if the user wants to enable crash reporting.
// Returns true if the user clicked "Yes".
func PromptCrashReporting() bool {
	script := `display dialog "` + escapeAppleScript(crashReportingPromptMessage) + `" with title "NFC Agent" buttons {"No", "Yes"} default button 2 with icon note`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Yes")
}

// CanShowDialogs reports whether native dialogs can be shown. Always true here.
func CanShowDialogs() bool {
	return true
}
