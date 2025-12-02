//go:build !linux

package tray

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"

	"github.com/SimplyPrint/nfc-agent/internal/api"
	"github.com/SimplyPrint/nfc-agent/internal/core"
	"github.com/SimplyPrint/nfc-agent/internal/welcome"
	"github.com/getlantern/systray"
)

// TrayApp manages the system tray icon and menu
type TrayApp struct {
	serverAddr  string
	onQuit      func()
	readerCount int
	mu          sync.Mutex

	// Menu items for updating
	mStatus  *systray.MenuItem
	mReaders *systray.MenuItem
}

// New creates a new TrayApp instance
func New(serverAddr string, onQuit func()) *TrayApp {
	return &TrayApp{
		serverAddr: serverAddr,
		onQuit:     onQuit,
	}
}

// Run starts the system tray. This function blocks until the tray is closed.
func (t *TrayApp) Run() {
	systray.Run(t.onReady, t.onExit)
}

// RunWithServer runs the tray on the main thread and starts the server in a goroutine.
// This function BLOCKS - it must be called from the main goroutine on macOS.
func (t *TrayApp) RunWithServer(serverStart func()) {
	systray.Run(func() {
		t.onReady()
		if serverStart != nil {
			go serverStart()
		}
	}, t.onExit)
}

func (t *TrayApp) onReady() {
	// Set icon
	systray.SetIcon(iconData)
	systray.SetTitle("") // Empty title for cleaner menu bar (macOS)
	systray.SetTooltip("NFC Agent")

	// Version header (disabled, just for display)
	mVersion := systray.AddMenuItem(fmt.Sprintf("NFC Agent v%s", api.Version), "")
	mVersion.Disable()

	systray.AddSeparator()

	// Status indicator
	t.mStatus = systray.AddMenuItem("Status: Starting...", "Server status")
	t.mStatus.Disable()

	// Reader count
	t.mReaders = systray.AddMenuItem("Readers: Checking...", "Connected NFC readers")
	t.mReaders.Disable()

	systray.AddSeparator()

	// Open status page
	mOpenUI := systray.AddMenuItem("Open Status Page", "Open web UI in browser")

	// About
	mAbout := systray.AddMenuItem("About", "About NFC Agent")

	systray.AddSeparator()

	// Quit
	mQuit := systray.AddMenuItem("Quit", "Exit NFC Agent")

	// Update status after a brief delay
	go t.updateStatus()

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mOpenUI.ClickedCh:
				t.openBrowser(fmt.Sprintf("http://%s/", t.serverAddr))
			case <-mAbout.ClickedCh:
				go welcome.ShowAbout(api.Version)
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func (t *TrayApp) onExit() {
	if t.onQuit != nil {
		t.onQuit()
	}
}

// UpdateStatus refreshes the status display in the tray menu
func (t *TrayApp) updateStatus() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Update status
	if t.mStatus != nil {
		t.mStatus.SetTitle("Status: Running")
	}

	// Count readers
	readers := core.ListReaders()
	t.readerCount = len(readers)

	if t.mReaders != nil {
		if t.readerCount == 0 {
			t.mReaders.SetTitle("Readers: None connected")
		} else if t.readerCount == 1 {
			t.mReaders.SetTitle("Readers: 1 connected")
		} else {
			t.mReaders.SetTitle(fmt.Sprintf("Readers: %d connected", t.readerCount))
		}
	}
}

// SetReaderCount updates the displayed reader count
func (t *TrayApp) SetReaderCount(count int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.readerCount = count
	if t.mReaders != nil {
		if count == 0 {
			t.mReaders.SetTitle("Readers: None connected")
		} else if count == 1 {
			t.mReaders.SetTitle("Readers: 1 connected")
		} else {
			t.mReaders.SetTitle(fmt.Sprintf("Readers: %d connected", count))
		}
	}
}

func (t *TrayApp) openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	cmd.Start()
}

// IsSupported returns true if the system tray is supported on this platform
func IsSupported() bool {
	return true
}
