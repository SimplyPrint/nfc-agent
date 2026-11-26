//go:build notray

package tray

// TrayApp is a no-op stub used when built with the "notray" tag (headless only).
type TrayApp struct {
	serverAddr string
	onQuit     func()
}

// New creates a new no-op TrayApp instance
func New(serverAddr string, isFirstRun bool, onQuit func()) *TrayApp {
	// isFirstRun is ignored - no tray, no welcome dialogs
	return &TrayApp{
		serverAddr: serverAddr,
		onQuit:     onQuit,
	}
}

// Run is a no-op - immediately returns
func (t *TrayApp) Run() {
}

// RunWithServer starts the server immediately (no tray)
func (t *TrayApp) RunWithServer(serverStart func()) {
	if serverStart != nil {
		serverStart()
	}
}

// SetReaderCount is a no-op
func (t *TrayApp) SetReaderCount(count int) {}

// IsSupported returns false: tray support was compiled out
func IsSupported() bool {
	return false
}
