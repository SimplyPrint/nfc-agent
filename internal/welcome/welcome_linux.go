//go:build linux

package welcome

import (
	"log"
	"os"
	"os/exec"
	"strings"
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

// Linux has no single native dialog API, so dialogs are shown via kdialog
// (preferred on KDE) or zenity (GNOME and most other desktops). If neither is
// installed, CanShowDialogs reports false and the first-run flow is postponed.

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

// CanShowDialogs reports whether a dialog tool (zenity or kdialog) is installed.
func CanShowDialogs() bool {
	return dialogTool() != ""
}

// dialogTool picks the dialog program to use: kdialog on KDE when available,
// otherwise zenity, otherwise kdialog. Returns "" when neither is installed.
func dialogTool() string {
	_, zenityErr := exec.LookPath("zenity")
	_, kdialogErr := exec.LookPath("kdialog")
	hasZenity, hasKdialog := zenityErr == nil, kdialogErr == nil

	if hasKdialog && strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE") {
		return "kdialog"
	}
	if hasZenity {
		return "zenity"
	}
	if hasKdialog {
		return "kdialog"
	}
	return ""
}

// The dialog commands below use fixed binary names. title/message are the
// constant strings in this file (plus our own version string), never user input.

func showInfo(title, message string) {
	switch dialogTool() {
	case "kdialog":
		_ = exec.Command("kdialog", "--title", title, "--msgbox", message).Run() // #nosec G204 -- fixed binary, constant text
	case "zenity":
		_ = exec.Command("zenity", "--info", "--no-markup", "--title", title, "--text", message).Run() // #nosec G204 -- fixed binary, constant text
	default:
		log.Printf("No dialog tool (zenity/kdialog) found; skipping %q dialog", title)
	}
}

func askYesNo(title, message string) bool {
	// Both tools exit 0 for "Yes" and non-zero for "No"/closed.
	switch dialogTool() {
	case "kdialog":
		return exec.Command("kdialog", "--title", title, "--yesno", message).Run() == nil // #nosec G204 -- fixed binary, constant text
	case "zenity":
		return exec.Command("zenity", "--question", "--no-markup", "--title", title, "--text", message, // #nosec G204 -- fixed binary, constant text
			"--ok-label", "Yes", "--cancel-label", "No").Run() == nil
	default:
		log.Printf("No dialog tool (zenity/kdialog) found; skipping %q prompt", title)
		return false
	}
}
