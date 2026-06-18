package data

import (
	_ "embed"
	"encoding/json"
)

// SupportedReader represents a known-to-work NFC reader with its capabilities
type SupportedReader struct {
	Name          string            `json:"name"`
	Manufacturer  string            `json:"manufacturer"`
	Description   string            `json:"description"`
	SupportedTags []string          `json:"supportedTags"`
	Capabilities  ReaderCapability  `json:"capabilities"`
	Limitations   []string          `json:"limitations"`
}

// ReaderCapability describes what operations a reader can perform
type ReaderCapability struct {
	Read      bool `json:"read"`
	Write     bool `json:"write"`
	NDEF      bool `json:"ndef"`
	// Desfire reports whether the reader can carry native DESFire APDUs for the
	// transparent DESFire session (desfire_* WS messages). Advisory: the session
	// itself fails clean if a given reader/card combination can't complete the
	// exchange.
	Desfire   bool `json:"desfire,omitempty"`
	Display   bool `json:"display,omitempty"`
	Bluetooth bool `json:"bluetooth,omitempty"`
}

// SupportedReadersData is the root structure of the JSON file
type SupportedReadersData struct {
	Readers []SupportedReader `json:"readers"`
}

//go:embed supported_readers.json
var supportedReadersJSON []byte

// GetSupportedReaders returns the list of known-to-work NFC readers
func GetSupportedReaders() ([]SupportedReader, error) {
	var data SupportedReadersData
	if err := json.Unmarshal(supportedReadersJSON, &data); err != nil {
		return nil, err
	}
	return data.Readers, nil
}
