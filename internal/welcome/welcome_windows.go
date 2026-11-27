//go:build windows

package welcome

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
)

const (
	MB_OK          = 0x00000000
	MB_YESNO       = 0x00000004
	MB_ICONINFO    = 0x00000040
	MB_ICONQUESTION = 0x00000020
	IDYES          = 6
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

// ShowWelcome displays a native welcome dialog on Windows
func ShowWelcome() {
	messageBox(welcomeTitle, welcomeMessage)
}

// ShowAbout displays a native about dialog on Windows
func ShowAbout(version string) {
	msg := aboutMessage + "\nVersion: " + version
	messageBox("About NFC Agent", msg)
}

func messageBox(title, message string) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messagePtr, _ := syscall.UTF16PtrFromString(message)
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(MB_OK|MB_ICONINFO),
	)
}

const autostartPromptMessage = `Would you like NFC Agent to start automatically when you log in?

This ensures the agent is always available for SimplyPrint.io and other applications.

You can change this later in the status page settings.`

// PromptAutostart shows a dialog asking if the user wants to enable auto-start.
// Returns true if the user clicked "Yes".
func PromptAutostart() bool {
	titlePtr, _ := syscall.UTF16PtrFromString("NFC Agent")
	messagePtr, _ := syscall.UTF16PtrFromString(autostartPromptMessage)
	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(MB_YESNO|MB_ICONQUESTION),
	)
	return ret == IDYES
}

const crashReportingPromptMessage = `Help improve NFC Agent by sending anonymous crash reports?

If the app crashes, diagnostic information will be sent to help us fix bugs faster. No personal data is collected.

You can change this later in the status page settings.`

// PromptCrashReporting shows a dialog asking if the user wants to enable crash reporting.
// Returns true if the user clicked "Yes".
func PromptCrashReporting() bool {
	titlePtr, _ := syscall.UTF16PtrFromString("NFC Agent")
	messagePtr, _ := syscall.UTF16PtrFromString(crashReportingPromptMessage)
	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(MB_YESNO|MB_ICONQUESTION),
	)
	return ret == IDYES
}

// CanShowDialogs reports whether native dialogs can be shown. Always true here.
func CanShowDialogs() bool {
	return true
}
