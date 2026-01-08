package proxmark3

import "errors"

var (
	// ErrNoCard indicates no card is present on the reader
	ErrNoCard = errors.New("no card present")
	// ErrNotConnected indicates the Proxmark3 device is not connected
	ErrNotConnected = errors.New("proxmark3 not connected")
	// ErrAuthFailed indicates MIFARE authentication failed (wrong key)
	ErrAuthFailed = errors.New("authentication failed")
	// ErrTimeout indicates a command timed out
	ErrTimeout = errors.New("command timed out")
	// ErrParseError indicates failed to parse pm3 output
	ErrParseError = errors.New("failed to parse output")
	// ErrPM3NotFound indicates the pm3 binary was not found in PATH
	ErrPM3NotFound = errors.New("pm3 binary not found")
	// ErrWriteFailed indicates a write operation failed
	ErrWriteFailed = errors.New("write operation failed")
)

// IsNoCardError returns true if the error indicates no card is present
func IsNoCardError(err error) bool {
	return errors.Is(err, ErrNoCard)
}

// IsAuthError returns true if the error indicates authentication failed
func IsAuthError(err error) bool {
	return errors.Is(err, ErrAuthFailed)
}
