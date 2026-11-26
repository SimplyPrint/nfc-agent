//go:build linux

package welcome

import (
	"log"
	"os/exec"
)

const welcomeTitle = "NFC Agent"
const welcomeMessage = `NFC Agent is now running!

The app runs quietly in your system tray and provides an API that allows SimplyPrint.io and other applications to communicate with your NFC readers.

You can access the status page at:
http://127.0.0.1:32145

Click the tray icon anytime to check status or quit.`

const aboutMessage = `NFC Agent

A lightweight background service that enables web applications like SimplyPrint.io to communicate with NFC readers connected to your computer.

Features:
• Automatic NFC reader detection
• Secure local API (127.0.0.1 only)
• Cross-platform support

Status page: http://127.0.0.1:32145

© SimplyPrint ApS`

const autostartPromptMessage = `Would you like NFC Agent to start automatically when you log in?

This ensures the agent is always available for SimplyPrint.io and other applications.

You can change this later in the status page settings.`

const crashReportingPromptMessage = `Help improve NFC Agent by sending anonymous crash reports?

If the app crashes, diagnostic information will be sent to help us fix bugs faster. No personal data is collected.

You can change this later in the status page settings.`

// Linux has no single native dialog API, so dialogs are shown via zenity
// (GNOME and most desktops) or kdialog (KDE). If neither is installed the
// dialogs are skipped and prompts answer "no".

// ShowWelcome displays a welcome dialog
func ShowWelcome() {
	showInfo(welcomeTitle, welcomeMessage)
}

// ShowAbout displays an about dialog
func ShowAbout(version string) {
	showInfo("About NFC Agent", aboutMessage+"\nVersion: "+version)
}

// PromptAutostart shows a dialog asking if the user wants to enable auto-start.
// Returns true if the user clicked "Yes".
func PromptAutostart() bool {
	return askYesNo(welcomeTitle, autostartPromptMessage)
}

// PromptCrashReporting shows a dialog asking if the user wants to enable crash reporting.
// Returns true if the user clicked "Yes".
func PromptCrashReporting() bool {
	return askYesNo(welcomeTitle, crashReportingPromptMessage)
}

func showInfo(title, message string) {
	if path, err := exec.LookPath("zenity"); err == nil {
		_ = exec.Command(path, "--info", "--no-markup", "--title", title, "--text", message).Run()
		return
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		_ = exec.Command(path, "--title", title, "--msgbox", message).Run()
		return
	}
	log.Printf("No dialog tool (zenity/kdialog) found; skipping %q dialog", title)
}

func askYesNo(title, message string) bool {
	// Both tools exit 0 for "Yes" and non-zero for "No"/closed.
	if path, err := exec.LookPath("zenity"); err == nil {
		return exec.Command(path, "--question", "--no-markup", "--title", title, "--text", message,
			"--ok-label", "Yes", "--cancel-label", "No").Run() == nil
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return exec.Command(path, "--title", title, "--yesno", message).Run() == nil
	}
	log.Printf("No dialog tool (zenity/kdialog) found; skipping %q prompt", title)
	return false
}
