package core

import (
	"fmt"
	"strings"

	"github.com/ebfe/scard"
)

// Reader represents a single NFC reader device.
type Reader struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "picc" for contactless readers, "sam" for SAM slots
}

// ListReaders returns a list of available NFC readers using PC/SC.
// Only returns PICC (contactless) readers, filtering out SAM slots.
func ListReaders() []Reader {
	ctx, err := scard.EstablishContext()
	if err != nil {
		// If PC/SC is not available, return empty list
		return []Reader{}
	}
	defer ctx.Release()

	readerNames, err := ctx.ListReaders()
	if err != nil {
		// No readers found
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
