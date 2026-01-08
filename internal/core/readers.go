package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/ebfe/scard"
)

// Reader represents a single NFC reader device.
type Reader struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "picc" for contactless readers, "sam" for SAM slots
}

// ListReaders returns a list of available NFC readers using PC/SC and Proxmark3.
// Only returns PICC (contactless) readers, filtering out SAM slots.
// Always returns a non-nil slice (empty slice if no readers found).
func ListReaders() []Reader {
	readers := []Reader{}

	// Get PC/SC readers
	readers = append(readers, listPCSCReaders()...)

	// Get Proxmark3 readers (if enabled)
	readers = append(readers, listProxmark3Readers()...)

	return readers
}

// listPCSCReaders returns PC/SC readers.
func listPCSCReaders() []Reader {
	ctx, err := scard.EstablishContext()
	if err != nil {
		// Log the error for diagnostics - this usually means pcscd is not running
		logging.Error(logging.CatReader, "Failed to establish PC/SC context - is pcscd running?", map[string]any{
			"error": err.Error(),
			"hint":  "On Linux, ensure pcscd is installed and running: sudo systemctl status pcscd",
		})
		return []Reader{}
	}
	defer ctx.Release()

	readerNames, err := ctx.ListReaders()
	if err != nil {
		// This is normal when no readers are connected, log at debug level
		logging.Debug(logging.CatReader, "No readers found", map[string]any{
			"error": err.Error(),
		})
		return []Reader{}
	}

	readers := make([]Reader, 0, len(readerNames))
	readerIndex := 0
	for _, name := range readerNames {
		readerType := detectReaderType(name)

		// Filter out SAM readers - we only want PICC (contactless NFC) readers
		if readerType == "sam" {
			continue
		}

		readers = append(readers, Reader{
			ID:   fmt.Sprintf("reader-%d", readerIndex),
			Name: name,
			Type: readerType,
		})
		readerIndex++
	}

	return readers
}

// detectReaderType determines if a reader is a PICC or SAM interface based on its name.
func detectReaderType(name string) string {
	nameLower := strings.ToLower(name)

	// Check for SAM keywords
	if strings.Contains(nameLower, " sam") || strings.Contains(nameLower, "sam ") {
		return "sam"
	}

	// Check for PICC keywords
	if strings.Contains(nameLower, "picc") {
		return "picc"
	}

	// Default to PICC for readers without explicit type indicators
	// (like some ACR122U models that don't include "PICC" in the name)
	return "picc"
}

// listProxmark3Readers detects connected Proxmark3 devices and returns them as readers.
// Proxmark3 detection is disabled by default and can be enabled via NFC_AGENT_PROXMARK3=1.
func listProxmark3Readers() []Reader {
	// Check if Proxmark3 support is enabled via environment variable
	if os.Getenv("NFC_AGENT_PROXMARK3") != "1" {
		return nil
	}

	// Use the singleton client (shared with card operations)
	client := getProxmark3Client()
	if client == nil {
		return nil
	}

	if !client.IsConnected() {
		logging.Debug(logging.CatReader, "Proxmark3 binary found but no device connected", map[string]any{
			"pm3Path": client.GetPath(),
			"pm3Port": client.GetPort(),
		})
		return nil
	}

	// Proxmark3 is connected and responding
	logging.Info(logging.CatReader, "Proxmark3 device detected", map[string]any{
		"pm3Path": client.GetPath(),
		"pm3Port": client.GetPort(),
	})

	return []Reader{
		{
			ID:   "proxmark3-0",
			Name: "Proxmark3",
			Type: "proxmark3",
		},
	}
}

// IsProxmark3Reader returns true if the reader name indicates a Proxmark3 device.
func IsProxmark3Reader(readerName string) bool {
	return strings.HasPrefix(readerName, "Proxmark3")
}
