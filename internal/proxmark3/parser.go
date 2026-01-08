package proxmark3

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Compiled regex patterns for parsing pm3 output
var (
	uidRegex   = regexp.MustCompile(`UID:\s*((?:[0-9A-Fa-f]{2}\s*)+)`)
	uid15Regex = regexp.MustCompile(`UID\.+\s*((?:[0-9A-Fa-f]{2}\s*)+)`) // ISO 15693 format: "UID.... XX XX XX" or "UID....... XX XX XX"
	atqaRegex      = regexp.MustCompile(`ATQA:\s*([0-9A-Fa-f\s]+)`)
	sakRegex       = regexp.MustCompile(`SAK:\s*([0-9A-Fa-f]+)`)
	blockDataRegex = regexp.MustCompile(`(?:data|block\s+\d+):\s*((?:[0-9A-Fa-f]{2}\s*)+)`)
	pageDataRegex  = regexp.MustCompile(`block\s+\d+\s*\|\s*((?:[0-9A-Fa-f]{2}\s*)+)`)
	atsRegex       = regexp.MustCompile(`ATS:\s*((?:[0-9A-Fa-f]{2}\s*)+)`)
	type15Regex    = regexp.MustCompile(`TYPE MATCH\s+(.+)`) // ISO 15693 type: "TYPE MATCH NXP (Philips); IC SL2 ICS2602 ( SLIX2 )"
)

// CardInfo represents parsed card information from hf 14a info
type CardInfo struct {
	UID      []byte // Card UID (4, 7, or 10 bytes)
	ATQA     []byte // Answer To reQuest type A (2 bytes)
	SAK      byte   // Select Acknowledge
	ATS      []byte // Answer To Select (optional, for ISO 14443-4)
	CardType string // Detected card type (e.g., "NTAG215", "MIFARE Classic 1K")
}

// ParseHF14AInfo parses output from "hf 14a info" command
func ParseHF14AInfo(output string) (*CardInfo, error) {
	info := &CardInfo{}

	// Parse UID
	if match := uidRegex.FindStringSubmatch(output); len(match) > 1 {
		uidHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
		uid, err := hex.DecodeString(uidHex)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid UID hex: %s", ErrParseError, uidHex)
		}
		info.UID = uid
	}

	// Parse ATQA
	if match := atqaRegex.FindStringSubmatch(output); len(match) > 1 {
		atqaHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
		atqa, err := hex.DecodeString(atqaHex)
		if err == nil {
			info.ATQA = atqa
		}
	}

	// Parse SAK
	if match := sakRegex.FindStringSubmatch(output); len(match) > 1 {
		sakHex := strings.TrimSpace(match[1])
		sak, err := hex.DecodeString(sakHex)
		if err == nil && len(sak) > 0 {
			info.SAK = sak[0]
		}
	}

	// Parse ATS (if present)
	if match := atsRegex.FindStringSubmatch(output); len(match) > 1 {
		atsHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
		ats, err := hex.DecodeString(atsHex)
		if err == nil {
			info.ATS = ats
		}
	}

	// Detect card type from output text and SAK
	info.CardType = detectCardType(output, info.SAK)

	if len(info.UID) == 0 {
		return nil, fmt.Errorf("%w: no UID found in output", ErrParseError)
	}

	return info, nil
}

// ParseHF15Info parses output from "hf 15 info" command (ISO 15693)
func ParseHF15Info(output string) (*CardInfo, error) {
	info := &CardInfo{}

	// Parse UID (format: "UID....... E0 04 01 08 66 15 39 80")
	if match := uid15Regex.FindStringSubmatch(output); len(match) > 1 {
		uidHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
		uid, err := hex.DecodeString(uidHex)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid UID hex: %s", ErrParseError, uidHex)
		}
		info.UID = uid
	}

	// Parse type (format: "TYPE MATCH NXP (Philips); IC SL2 ICS2602 ( SLIX2 )")
	if match := type15Regex.FindStringSubmatch(output); len(match) > 1 {
		info.CardType = detectISO15693Type(strings.TrimSpace(match[1]))
	} else {
		info.CardType = "ISO 15693"
	}

	if len(info.UID) == 0 {
		return nil, fmt.Errorf("%w: no UID found in output", ErrParseError)
	}

	return info, nil
}

// detectISO15693Type extracts a readable card type from ISO 15693 TYPE MATCH output
func detectISO15693Type(typeMatch string) string {
	lower := strings.ToLower(typeMatch)

	// Check for ICODE SLIX variants
	if strings.Contains(lower, "slix2") {
		return "ICODE SLIX2"
	}
	if strings.Contains(lower, "slix-s") {
		return "ICODE SLIX-S"
	}
	if strings.Contains(lower, "slix-l") {
		return "ICODE SLIX-L"
	}
	if strings.Contains(lower, "slix") {
		return "ICODE SLIX"
	}
	if strings.Contains(lower, "icode") {
		return "ICODE"
	}

	// Check for other ISO 15693 types
	if strings.Contains(lower, "tag-it") {
		return "Tag-it"
	}
	if strings.Contains(lower, "lri") {
		return "LRI"
	}

	// Default to generic ISO 15693 with manufacturer info if available
	if strings.Contains(lower, "nxp") {
		return "ISO 15693 (NXP)"
	}
	if strings.Contains(lower, "texas") || strings.Contains(lower, "ti") {
		return "ISO 15693 (TI)"
	}
	if strings.Contains(lower, "st") {
		return "ISO 15693 (ST)"
	}

	return "ISO 15693"
}

// ParseBlockData parses output from MIFARE block read commands
func ParseBlockData(output string) ([]byte, error) {
	match := blockDataRegex.FindStringSubmatch(output)
	if len(match) < 2 {
		return nil, fmt.Errorf("%w: no block data found", ErrParseError)
	}

	dataHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid hex data: %s", ErrParseError, dataHex)
	}

	return data, nil
}

// ParseMFUPage parses output from MIFARE Ultralight/NTAG page read commands
func ParseMFUPage(output string) ([]byte, error) {
	// Try the page format first: "block X | AA BB CC DD | ...."
	match := pageDataRegex.FindStringSubmatch(output)
	if len(match) >= 2 {
		dataHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
		data, err := hex.DecodeString(dataHex)
		if err == nil {
			return data, nil
		}
	}

	// Fall back to block data format
	return ParseBlockData(output)
}

// ParseMultiplePages parses multiple pages from hf mfu rdbl output
func ParseMultiplePages(output string) ([][]byte, error) {
	var pages [][]byte
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if match := pageDataRegex.FindStringSubmatch(line); len(match) >= 2 {
			dataHex := strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
			data, err := hex.DecodeString(dataHex)
			if err == nil {
				pages = append(pages, data)
			}
		}
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: no pages found", ErrParseError)
	}

	return pages, nil
}

// detectCardType determines the card type from pm3 output and SAK byte
func detectCardType(output string, sak byte) string {
	lower := strings.ToLower(output)

	// Check for specific NTAG variants
	if strings.Contains(lower, "ntag213") {
		return "NTAG213"
	}
	if strings.Contains(lower, "ntag215") {
		return "NTAG215"
	}
	if strings.Contains(lower, "ntag216") {
		return "NTAG216"
	}
	if strings.Contains(lower, "ntag21") {
		return "NTAG21x"
	}

	// Check for MIFARE Ultralight variants
	if strings.Contains(lower, "ultralight ev1") {
		return "MIFARE Ultralight EV1"
	}
	if strings.Contains(lower, "ultralight c") {
		return "MIFARE Ultralight C"
	}
	if strings.Contains(lower, "ultralight") {
		return "MIFARE Ultralight"
	}

	// Check for MIFARE Classic by SAK
	switch sak {
	case 0x08:
		return "MIFARE Classic 1K"
	case 0x18:
		return "MIFARE Classic 4K"
	case 0x09:
		return "MIFARE Mini"
	case 0x00:
		// SAK 0x00 is typically NTAG/Ultralight, but we already checked those
		if strings.Contains(lower, "mifare") {
			return "MIFARE Ultralight"
		}
	case 0x20:
		if strings.Contains(lower, "desfire") {
			return "MIFARE DESFire"
		}
		return "ISO 14443-4"
	}

	// Check for other card types in output
	if strings.Contains(lower, "mifare classic") {
		if strings.Contains(lower, "4k") {
			return "MIFARE Classic 4K"
		}
		return "MIFARE Classic 1K"
	}
	if strings.Contains(lower, "desfire") {
		return "MIFARE DESFire"
	}

	return "Unknown"
}

// IsWriteSuccess checks if a write command output indicates success
func IsWriteSuccess(output string) bool {
	lower := strings.ToLower(output)
	successPatterns := []string{
		"ok",
		"successful",
		"done",
		"write block successful",
	}

	for _, pattern := range successPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// ParseDeviceList parses output from pm3 --list to get connected devices
func ParseDeviceList(output string) []string {
	var devices []string
	lines := strings.Split(output, "\n")

	// Look for lines containing device paths
	devicePatterns := []string{
		"/dev/tty",    // Linux/macOS
		"/dev/cu.",    // macOS
		"COM",         // Windows
		"/dev/serial", // Linux udev symlinks
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, pattern := range devicePatterns {
			if strings.Contains(line, pattern) {
				// Extract the device path
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.Contains(part, pattern) {
						devices = append(devices, part)
						break
					}
				}
			}
		}
	}

	return devices
}
