//go:build !linux

package tray

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/api"
	"github.com/SimplyPrint/nfc-agent/internal/core"
	"github.com/SimplyPrint/nfc-agent/internal/service"
	"github.com/SimplyPrint/nfc-agent/internal/settings"
	"github.com/SimplyPrint/nfc-agent/internal/welcome"
	"github.com/getlantern/systray"
)

// TrayApp manages the system tray icon and menu
type TrayApp struct {
	serverAddr  string
	onQuit      func()
	isFirstRun  bool
	readerCount int
	mu          sync.Mutex

	// Menu items for updating
	mStatus  *systray.MenuItem
	mReaders *systray.MenuItem
}

// New creates a new TrayApp instance
func New(serverAddr string, isFirstRun bool, onQuit func()) *TrayApp {
	return &TrayApp{
		serverAddr: serverAddr,
		isFirstRun: isFirstRun,
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
	// Wait for GUI/WindowServer to be ready (handles macOS startup race condition)
	if !WaitForGUI(30*time.Second, 1*time.Second) {
		log.Println("Warning: GUI may not be ready, systray initialization may fail")
	}

	systray.Run(func() {
		t.onReady()
		if serverStart != nil {
			go serverStart()
		}
	}, t.onExit)
}

func (t *TrayApp) onReady() {
	// Set icon - use template icon for proper dark/light mode support on macOS
	systray.SetTemplateIcon(templateIconData, iconData)
	systray.SetTitle("") // Empty title for cleaner menu bar (macOS)
	systray.SetTooltip("NFC Agent")

	// Version header (disabled, just for display)
	// Only add "v" prefix for proper version numbers (e.g., "1.2.3"), not for dev builds
	versionStr := api.Version
	if len(versionStr) > 0 && versionStr[0] >= '0' && versionStr[0] <= '9' {
		versionStr = "v" + versionStr
	}
	mVersion := systray.AddMenuItem(fmt.Sprintf("NFC Agent %s", versionStr), "")
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

	// Show first-run prompts after tray is fully initialized
	// This prevents race condition with Cocoa event loop on macOS
	if t.isFirstRun {
		go t.showFirstRunPrompts()
	}
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

// showFirstRunPrompts displays welcome dialogs and prompts on first run.
// This runs after the tray is initialized to avoid race conditions with Cocoa on macOS.
func (t *TrayApp) showFirstRunPrompts() {
	welcome.ShowWelcome()

	// Check if auto-start is not already configured (e.g., by Homebrew)
	svc := service.New()
	if !svc.IsInstalled() {
		// Prompt user to enable auto-start
		if welcome.PromptAutostart() {
			if err := svc.Install(); err != nil {
				log.Printf("Failed to enable auto-start: %v", err)
			} else {
				log.Println("Auto-start enabled")
			}
		}
	}

	// Prompt for crash reporting
	if welcome.PromptCrashReporting() {
		if err := settings.SetCrashReporting(true); err != nil {
			log.Printf("Failed to save crash reporting setting: %v", err)
		} else {
			log.Println("Crash reporting enabled")
		}
	}

	_ = welcome.MarkAsShown() // Ignore error - non-critical
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
