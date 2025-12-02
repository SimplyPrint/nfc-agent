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
	MB_ICONINFO    = 0x00000040
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
