//go:build linux

package tray

// TrayApp is a no-op on Linux (runs headless)
type TrayApp struct {
	serverAddr string
	onQuit     func()
}

// New creates a new TrayApp instance (no-op on Linux)
func New(serverAddr string, onQuit func()) *TrayApp {
	return &TrayApp{
		serverAddr: serverAddr,
		onQuit:     onQuit,
	}
}

// Run is a no-op on Linux - immediately returns
func (t *TrayApp) Run() {
	// Linux runs headless, no system tray
}

// RunWithServer starts the server immediately on Linux (no tray)
func (t *TrayApp) RunWithServer(serverStart func()) {
	if serverStart != nil {
		serverStart()
	}
}

// SetReaderCount is a no-op on Linux
func (t *TrayApp) SetReaderCount(count int) {
	// No-op on Linux
}

// IsSupported returns false on Linux
func IsSupported() bool {
	return false
}
