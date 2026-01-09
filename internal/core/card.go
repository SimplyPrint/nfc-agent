package core

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/config"
	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/SimplyPrint/nfc-agent/internal/openprinttag"
	"github.com/SimplyPrint/nfc-agent/internal/proxmark3"
	"github.com/ebfe/scard"
)

// Card represents an NFC card/tag.
type Card struct {
	UID         string `json:"uid"`
	ATR         string `json:"atr,omitempty"`
	Type        string `json:"type,omitempty"`        // e.g., "NTAG213", "NTAG215", "NTAG216", "MIFARE Classic"
	Protocol    string `json:"protocol,omitempty"`    // Short protocol: "NFC-A", "NFC-V"
	ProtocolISO string `json:"protocolISO,omitempty"` // Full ISO protocol: "ISO 14443-3A", "ISO 15693"
	Size        int    `json:"size,omitempty"`        // Memory size in bytes
	Writable    bool   `json:"writable,omitempty"`    // Whether the tag is writable
	URL         string `json:"url,omitempty"`         // URL from first NDEF record (if URI record)
	Data        string `json:"data,omitempty"`        // NDEF data read from the tag (if available)
	DataType    string `json:"dataType,omitempty"`    // Type of data: "text", "json", "binary", or "unknown"
}

// cardDetectionCache stores card detection results keyed by UID to avoid
// re-running expensive detection (GET_VERSION, CC reads, memory probes)
// on every poll cycle. The cache is cleared when a card is removed.
var (
	cardDetectionCache   = make(map[string]*Card)
	cardDetectionCacheMu sync.RWMutex
)

// ClearCardCache removes a card from the detection cache.
// Called when a card is removed from the reader.
func ClearCardCache(uid string) {
	cardDetectionCacheMu.Lock()
	delete(cardDetectionCache, uid)
	cardDetectionCacheMu.Unlock()
	logging.Debug(logging.CatCard, "Cleared card detection cache", map[string]any{"uid": uid})
}

// ClearAllCardCache clears the entire card detection cache.
func ClearAllCardCache() {
	cardDetectionCacheMu.Lock()
	cardDetectionCache = make(map[string]*Card)
	cardDetectionCacheMu.Unlock()
	logging.Debug(logging.CatCard, "Cleared all card detection cache", nil)
}

// GetCardUID connects to the specified reader and attempts to read the card UID.
// Returns an error if no card is present or if reading fails.
func GetCardUID(readerName string) (*Card, error) {
	// Dispatch to Proxmark3 if this is a Proxmark3 reader
	if IsProxmark3Reader(readerName) {
		return getCardUIDProxmark3(readerName)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	// Connect to the reader
	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Get the ATR (Answer To Reset)
	status, err := card.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get card status: %w", err)
	}

	// Send APDU command to get UID: FF CA 00 00 00
	// This is a common command for getting the UID from PC/SC readers
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}

	rsp, err := card.Transmit(getUIDCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to transmit get UID command: %w", err)
	}

	// Check response - should end with 90 00 (success)
	if len(rsp) < 2 {
		return nil, fmt.Errorf("invalid response length: %d", len(rsp))
	}

	sw1 := rsp[len(rsp)-2]
	sw2 := rsp[len(rsp)-1]

	if sw1 != 0x90 || sw2 != 0x00 {
		return nil, fmt.Errorf("command failed with status: %02X %02X", sw1, sw2)
	}

	// UID is everything except the last 2 bytes (status words)
	uid := rsp[:len(rsp)-2]
	uidHex := hex.EncodeToString(uid)

	// Check cache for existing detection results to avoid re-running expensive
	// detection commands (GET_VERSION, CC reads, memory probes) on every poll.
	// This significantly reduces reader command traffic and prevents reader
	// unresponsiveness on readers like ACR1252 that struggle with rapid commands.
	cardDetectionCacheMu.RLock()
	cachedCard, found := cardDetectionCache[uidHex]
	cardDetectionCacheMu.RUnlock()

	var cardInfo *Card
	if found {
		// Use cached detection results - only update ATR in case it changed
		logging.Debug(logging.CatCard, "Using cached card detection", map[string]any{"uid": uidHex})
		cardInfo = &Card{
			UID:         cachedCard.UID,
			ATR:         hex.EncodeToString(status.Atr),
			Type:        cachedCard.Type,
			Protocol:    cachedCard.Protocol,
			ProtocolISO: cachedCard.ProtocolISO,
			Size:        cachedCard.Size,
			Writable:    cachedCard.Writable,
		}
	} else {
		// No cache - run full detection
		cardInfo = &Card{
			UID: uidHex,
			ATR: hex.EncodeToString(status.Atr),
		}

		// Detect card type by reading version info (for NTAG cards)
		detectCardType(card, cardInfo)

		// Cache the detection result (excluding NDEF data which may change)
		cardDetectionCacheMu.Lock()
		cardDetectionCache[uidHex] = &Card{
			UID:         cardInfo.UID,
			ATR:         cardInfo.ATR,
			Type:        cardInfo.Type,
			Protocol:    cardInfo.Protocol,
			ProtocolISO: cardInfo.ProtocolISO,
			Size:        cardInfo.Size,
			Writable:    cardInfo.Writable,
		}
		cardDetectionCacheMu.Unlock()
		logging.Debug(logging.CatCard, "Cached card detection result", map[string]any{
			"uid":  uidHex,
			"type": cardInfo.Type,
			"size": cardInfo.Size,
		})
	}

	// Try to read NDEF data from the card
	readNDEFData(card, cardInfo)

	return cardInfo, nil
}

// detectCardType attempts to determine the card type (NTAG213/215/216, MIFARE, etc.)
func detectCardType(card *scard.Card, cardInfo *Card) {
	// Log the final detection result when function returns
	defer func() {
		logging.Debug(logging.CatCard, "Card type detection complete", map[string]any{
			"uid":         cardInfo.UID,
			"type":        cardInfo.Type,
			"size":        cardInfo.Size,
			"atr":         cardInfo.ATR,
			"protocol":    cardInfo.Protocol,
			"protocolISO": cardInfo.ProtocolISO,
		})
	}()

	// Detect protocol from ATR patterns (set early, before type detection)
	atr := cardInfo.ATR
	if len(atr) >= 30 && (atr[0:4] == "3b8f" || atr[0:4] == "3b8b") {
		if contains(atr, "03060b") {
			// ISO 15693 (NFC-V) - ICode SLI/SLIX/SLIX2
			cardInfo.Protocol = "NFC-V"
			cardInfo.ProtocolISO = "ISO 15693"
		} else if contains(atr, "03060300") {
			// ISO 14443-3A (NFC-A) - NTAG, MIFARE Classic, MIFARE Ultralight
			cardInfo.Protocol = "NFC-A"
			cardInfo.ProtocolISO = "ISO 14443-3A"
		}
	}

	// Track if GET_VERSION ever succeeded - important for trusting CC-based detection later.
	// All NXP NTAG21x and Ultralight EV1 support GET_VERSION.
	// Plain MIFARE Ultralight does NOT support GET_VERSION.
	getVersionSucceeded := false

	// Track if CC detection found valid NDEF - used to gate ATR-based fallback detection.
	ccDetectionFoundNDEF := false

	// NOTE: Transparent exchange GET_VERSION (Method 0) was removed because it leaves
	// the ACR1552 reader in a state where subsequent standard PC/SC commands fail.
	// Standard methods 1a and 1b work on ACR1552 without this issue.

	// Method 1a: Try GET_VERSION with standard PC/SC passthrough
	getVersionCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00}
	rsp, err := card.Transmit(getVersionCmd)

	// Log GET_VERSION response for diagnostics
	if err == nil && len(rsp) >= 2 {
		logging.Debug(logging.CatCard, "GET_VERSION response", map[string]any{
			"method":   "1a",
			"response": hex.EncodeToString(rsp),
			"status":   fmt.Sprintf("%02x%02x", rsp[len(rsp)-2], rsp[len(rsp)-1]),
		})
	}

	if err == nil && len(rsp) >= 10 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		// Parse version response: [header, vendor, productType, subtype, major, minor, storage, protocol, SW1, SW2]
		// Validate header byte (0x00) - tags that don't support GET_VERSION may return garbage with status 9000
		header := rsp[0]
		productType := rsp[2]
		storageSize := rsp[6]

		logging.Debug(logging.CatCard, "GET_VERSION parsed (Method 1a)", map[string]any{
			"header":      fmt.Sprintf("0x%02x", header),
			"productType": fmt.Sprintf("0x%02x", productType),
			"storageSize": fmt.Sprintf("0x%02x", storageSize),
		})

		if header != 0x00 {
			logging.Debug(logging.CatCard, "Invalid GET_VERSION header (Method 1a), ignoring", map[string]any{
				"expected": "0x00",
				"got":      fmt.Sprintf("0x%02x", header),
			})
			// Fall through to Method 1b
		} else {
			getVersionSucceeded = true

			if productType == 0x04 { // NTAG family
				switch storageSize {
				case 0x0F: // NTAG213
					cardInfo.Type = "NTAG213"
					cardInfo.Size = 180
					cardInfo.Writable = true
					return
				case 0x11: // NTAG215
					cardInfo.Type = "NTAG215"
					cardInfo.Size = 504
					cardInfo.Writable = true
					return
				case 0x13: // NTAG216
					cardInfo.Type = "NTAG216"
					cardInfo.Size = 888
					cardInfo.Writable = true
					return
				}
			} else if productType == 0x03 { // MIFARE Ultralight family
				switch storageSize {
				case 0x0B: // Ultralight EV1 MF0UL11 (48 bytes)
					cardInfo.Type = "MIFARE Ultralight EV1"
					cardInfo.Size = 48
					cardInfo.Writable = true
					return
				case 0x0E: // Ultralight EV1 MF0UL21 (128 bytes)
					cardInfo.Type = "MIFARE Ultralight EV1"
					cardInfo.Size = 128
					cardInfo.Writable = true
					return
				default: // Unknown Ultralight variant
					cardInfo.Type = "MIFARE Ultralight"
					cardInfo.Size = 64
					cardInfo.Writable = true
					return
				}
			}
		}
	}

	// Method 1b: Try alternative GET_VERSION format (works on ACR1252U)
	// ACR1252U needs a delay after a failed command before it will accept another
	method1aFailed := err != nil || len(rsp) < 10 || rsp[len(rsp)-2] != 0x90
	if method1aFailed {
		time.Sleep(150 * time.Millisecond)
	}
	getVersionCmd2 := []byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60}
	rsp, err = card.Transmit(getVersionCmd2)

	// Log Method 1b response
	if err == nil && len(rsp) >= 2 {
		logging.Debug(logging.CatCard, "GET_VERSION response", map[string]any{
			"method":   "1b",
			"response": hex.EncodeToString(rsp),
			"status":   fmt.Sprintf("%02x%02x", rsp[len(rsp)-2], rsp[len(rsp)-1]),
		})
	}

	if err == nil && len(rsp) >= 10 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		// Parse version response: [header, vendor, productType, subtype, major, minor, storage, protocol, SW1, SW2]
		// Validate header byte (0x00) - tags that don't support GET_VERSION may return garbage with status 9000
		header := rsp[0]
		productType := rsp[2]
		storageSize := rsp[6]

		logging.Debug(logging.CatCard, "GET_VERSION parsed (Method 1b)", map[string]any{
			"header":      fmt.Sprintf("0x%02x", header),
			"productType": fmt.Sprintf("0x%02x", productType),
			"storageSize": fmt.Sprintf("0x%02x", storageSize),
		})

		if header != 0x00 {
			logging.Debug(logging.CatCard, "Invalid GET_VERSION header (Method 1b), ignoring", map[string]any{
				"expected": "0x00",
				"got":      fmt.Sprintf("0x%02x", header),
			})
			// Fall through to CC-based detection
		} else {
			getVersionSucceeded = true

			if productType == 0x04 { // NTAG family
				switch storageSize {
				case 0x0F: // NTAG213
					cardInfo.Type = "NTAG213"
					cardInfo.Size = 180
					cardInfo.Writable = true
					return
				case 0x11: // NTAG215
					cardInfo.Type = "NTAG215"
					cardInfo.Size = 504
					cardInfo.Writable = true
					return
				case 0x13: // NTAG216
					cardInfo.Type = "NTAG216"
					cardInfo.Size = 888
					cardInfo.Writable = true
					return
				}
			} else if productType == 0x03 { // MIFARE Ultralight family
				switch storageSize {
				case 0x0B: // Ultralight EV1 MF0UL11 (48 bytes)
					cardInfo.Type = "MIFARE Ultralight EV1"
					cardInfo.Size = 48
					cardInfo.Writable = true
					return
				case 0x0E: // Ultralight EV1 MF0UL21 (128 bytes)
					cardInfo.Type = "MIFARE Ultralight EV1"
					cardInfo.Size = 128
					cardInfo.Writable = true
					return
				default: // Unknown Ultralight variant
					cardInfo.Type = "MIFARE Ultralight"
					cardInfo.Size = 64
					cardInfo.Writable = true
					return
				}
			}
		}
	}

	// Method 2a: Try reading pages 1-4 (works on ACR1252U where direct page 3 read fails)
	// Page 3 contains the capability container at offset 8 in this 16-byte response
	readCmd1 := []byte{0xFF, 0xB0, 0x00, 0x01, 0x10} // Read 16 bytes from page 1
	rsp, err = card.Transmit(readCmd1)

	if err == nil && len(rsp) >= 12 && rsp[len(rsp)-2] == 0x90 {
		// CC is at bytes 8-11 (page 3 within the 4-page read)
		// CC byte 0 (index 8): NDEF magic (must be 0xE1 for valid NDEF)
		// CC byte 2 (index 10): Memory size indicator
		logging.Debug(logging.CatCard, "CC read (Method 2a)", map[string]any{
			"response":  hex.EncodeToString(rsp),
			"cc_bytes":  hex.EncodeToString(rsp[8:12]),
			"cc_magic":  fmt.Sprintf("0x%02x", rsp[8]),
			"cc_size":   fmt.Sprintf("0x%02x", rsp[10]),
		})
		if len(rsp) >= 11 && rsp[8] == 0xE1 { // Validate NDEF magic byte
			ccDetectionFoundNDEF = true
			ccSize := rsp[10]

			// IMPORTANT: For NTAG detection, we require GET_VERSION to have succeeded.
			// All NXP NTAG21x tags support GET_VERSION. Plain MIFARE Ultralight does NOT.
			// If GET_VERSION failed but CC size suggests NTAG, it's likely plain Ultralight
			// with non-standard CC (formatted with more capacity than it has).
			switch ccSize {
			case 0x06: // 48 bytes -> MIFARE Ultralight
				cardInfo.Type = "MIFARE Ultralight"
				cardInfo.Size = 48
				cardInfo.Writable = true
				return
			case 0x12: // 144 bytes -> NTAG213
				// CC size 0x12 is too large for plain Ultralight (48 bytes max)
				// so this must be NTAG213 regardless of GET_VERSION result
				cardInfo.Type = "NTAG213"
				cardInfo.Size = 180
				cardInfo.Writable = true
				return
			case 0x3E: // 496 bytes -> NTAG215
				// CC size 0x3E is too large for plain Ultralight (64 bytes max)
				// so this must be NTAG215 regardless of GET_VERSION result
				cardInfo.Type = "NTAG215"
				cardInfo.Size = 504
				cardInfo.Writable = true
				return
			case 0x6D: // 872 bytes -> NTAG216
				// CC size 0x6D is too large for plain Ultralight (64 bytes max)
				// so this must be NTAG216 regardless of GET_VERSION result
				cardInfo.Type = "NTAG216"
				cardInfo.Size = 888
				cardInfo.Writable = true
				return
			}
		}
	}

	// Method 2b: Try reading page 3 directly to get capability container (works on ACR122U)
	// Read 4 pages starting from page 3 (CC bytes)
	readCmd := []byte{0xFF, 0xB0, 0x00, 0x03, 0x10} // Read 16 bytes from page 3
	rsp, err = card.Transmit(readCmd)

	if err == nil && len(rsp) >= 6 && rsp[len(rsp)-2] == 0x90 {
		// Page 3 contains capability container
		// CC byte 0 (index 0): NDEF magic (must be 0xE1 for valid NDEF)
		// CC byte 2 (index 2): Memory size indicator
		if len(rsp) >= 3 && rsp[0] == 0xE1 { // Validate NDEF magic byte
			ccDetectionFoundNDEF = true
			ccSize := rsp[2]

			// IMPORTANT: For NTAG detection, we require GET_VERSION to have succeeded.
			// All NXP NTAG21x tags support GET_VERSION. Plain MIFARE Ultralight does NOT.
			switch ccSize {
			case 0x06: // 48 bytes -> MIFARE Ultralight
				cardInfo.Type = "MIFARE Ultralight"
				cardInfo.Size = 48
				cardInfo.Writable = true
				return
			case 0x12: // 144 bytes -> NTAG213
				// CC size 0x12 is too large for plain Ultralight (48 bytes max)
				// so this must be NTAG213 regardless of GET_VERSION result
				cardInfo.Type = "NTAG213"
				cardInfo.Size = 180
				cardInfo.Writable = true
				return
			case 0x3E: // 496 bytes -> NTAG215
				// CC size 0x3E is too large for plain Ultralight (64 bytes max)
				// so this must be NTAG215 regardless of GET_VERSION result
				cardInfo.Type = "NTAG215"
				cardInfo.Size = 504
				cardInfo.Writable = true
				return
			case 0x6D: // 872 bytes -> NTAG216
				// CC size 0x6D is too large for plain Ultralight (64 bytes max)
				// so this must be NTAG216 regardless of GET_VERSION result
				cardInfo.Type = "NTAG216"
				cardInfo.Size = 888
				cardInfo.Writable = true
				return
			}
		}
	}

	// Method 2c: MIFARE Classic authentication probe
	// Some readers (e.g., ACR1252U) return incorrect ATR byte 14 for MIFARE Classic,
	// causing them to be misidentified as Type 2 tags. This probe tries to authenticate
	// to sector 0 - Classic cards require this, NTAG/Ultralight don't support it.
	// Only try this if we haven't identified the card yet and ATR suggests ISO 14443-A.
	if contains(atr, "03060300") {
		// Load default transport key (FFFFFFFFFFFF) into reader's key slot
		loadKeyCmd := []byte{0xFF, 0x82, 0x00, 0x00, 0x06, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
		rsp, err = card.Transmit(loadKeyCmd)
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
			// Try to authenticate to block 0 with Key A (0x60)
			// If this succeeds, it's definitely MIFARE Classic
			authCmd := []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, 0x00, 0x60, 0x00}
			rsp, err = card.Transmit(authCmd)
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				logging.Debug(logging.CatCard, "MIFARE Classic detected via authentication probe", nil)
				cardInfo.Type = "MIFARE Classic"
				cardInfo.Writable = true
				cardInfo.Size = 1024
				return
			}
		}
	}

	// Method 3: Memory probe - determine card capacity by reading high page numbers
	// This is used when GET_VERSION and CC detection both failed but ATR suggests ISO 14443-3A Type 2.
	// Unlike ICode SLIX (ISO 15693) which has type indicator bits in the UID, NTAG21x cards
	// follow ISO 14443-3A which has NO type identifiers in the UID. We must probe actual memory.
	// Page boundaries: Ultralight=16, NTAG213=45, NTAG215=135, NTAG216=231
	if contains(atr, "03060300") && !ccDetectionFoundNDEF && !getVersionSucceeded {
		logging.Debug(logging.CatCard, "Starting memory probe detection", nil)

		// Try page 135 (only NTAG216 has this - it has 231 pages)
		if _, err := readNTAGPage(card, 135); err == nil {
			logging.Debug(logging.CatCard, "Memory probe: page 135 readable, detected NTAG216", nil)
			cardInfo.Type = "NTAG216"
			cardInfo.Size = 888
			cardInfo.Writable = true
			return
		}

		// Try page 45 (NTAG215 has 135 pages, NTAG213 has 45, Ultralight has 16)
		if _, err := readNTAGPage(card, 45); err == nil {
			logging.Debug(logging.CatCard, "Memory probe: page 45 readable, detected NTAG215", nil)
			cardInfo.Type = "NTAG215"
			cardInfo.Size = 504
			cardInfo.Writable = true
			return
		}

		// Try page 16 (NTAG213 has 45 pages, Ultralight only has 16)
		if _, err := readNTAGPage(card, 16); err == nil {
			logging.Debug(logging.CatCard, "Memory probe: page 16 readable, detected NTAG213", nil)
			cardInfo.Type = "NTAG213"
			cardInfo.Size = 180
			cardInfo.Writable = true
			return
		}

		// All probes failed - this is likely actual MIFARE Ultralight
		logging.Debug(logging.CatCard, "Memory probe: all high pages failed, assuming MIFARE Ultralight", nil)
		cardInfo.Type = "MIFARE Ultralight"
		cardInfo.Size = 64
		cardInfo.Writable = true
		return
	}

	// Method 4: Check ATR patterns for NTAG, MIFARE, and ISO 15693
	// Note: atr is already set at the start of detectCardType for protocol detection
	if len(atr) >= 30 && (atr[0:4] == "3b8f" || atr[0:4] == "3b8b") {
		// Check for ISO 15693 cards (ICode SLI, ICode Slix, ICode Slix 2)
		// Pattern: 03 06 0b at bytes 10-12 (hex string position 20-25)
		if contains(atr, "03060b") {
			// ISO 15693 family - check UID manufacturer byte to identify vendor
			// For ISO 15693, the UID is 8 bytes with manufacturer code in last byte (reversed order)
			// 0xE0 = NXP (ICode SLI/Slix/Slix-2)
			uid := cardInfo.UID
			if len(uid) >= 16 && uid[14:16] == "e0" {
				// NXP ICode family - differentiate SLI/SLIX/SLIX2 using type indicator bits
				// Per NXP datasheet (SL2S2602), UID4 (hex position 8-9) contains bits 40:33
				// Bits 37 and 36 indicate the ICODE type:
				//   Bit 37=0, Bit 36=0: ICODE SLI
				//   Bit 37=1, Bit 36=0: ICODE SLIX
				//   Bit 37=0, Bit 36=1: ICODE SLIX2
				//   Bit 37=1, Bit 36=1: RFU
				cardInfo.Writable = true
				if len(uid) >= 10 {
					uid4Byte, err := strconv.ParseUint(uid[8:10], 16, 8)
					if err == nil {
						bit37 := (uid4Byte >> 4) & 1 // bit 4 of the byte
						bit36 := (uid4Byte >> 3) & 1 // bit 3 of the byte
						switch {
						case bit37 == 0 && bit36 == 1:
							cardInfo.Type = "ICode SLIX2"
							cardInfo.Size = 320 // SLIX2 has 2560 bits = 320 bytes
							return
						case bit37 == 1 && bit36 == 0:
							cardInfo.Type = "ICode SLIX"
							cardInfo.Size = 896 // SLIX has 896 bytes
							return
						case bit37 == 0 && bit36 == 0:
							cardInfo.Type = "ICode SLI"
							cardInfo.Size = 112 // SLI has 896 bits = 112 bytes
							return
						}
					}
				}
				// Fallback if we couldn't parse type bits
				cardInfo.Type = "ICode SLIX"
				cardInfo.Size = 896
				return
			}
			// Generic ISO 15693
			cardInfo.Type = "ISO 15693"
			cardInfo.Writable = true
			cardInfo.Size = 1024
			return
		}

		// Both NTAG, MIFARE Ultralight, and MIFARE Classic can have ATRs starting with
		// 3b8f and containing 03060300. The key difference is at byte 14 (hex position 28-29):
		// - MIFARE Classic: 01
		// - NTAG / MIFARE Ultralight: 03 (can't distinguish by ATR alone)

		if contains(atr, "03060300") {
			// Check byte 14 to distinguish MIFARE Classic from other types
			if atr[28:30] == "01" {
				// MIFARE Classic
				cardInfo.Type = "MIFARE Classic"
				cardInfo.Writable = true
				cardInfo.Size = 1024
				return
			}
			// Note: For byte 14 == "03" (ISO 14443-3A Type 2 tags like NTAG/Ultralight),
			// detection is handled by Method 3 (memory probe) above when GET_VERSION and CC both failed.
			// If we reach here, either GET_VERSION or CC succeeded but didn't identify the type,
			// which means the CC size didn't match known types - fall through to unknown.
		}

		// Fallback: if ATR starts with 3b8f/3b8b but doesn't match above patterns
		// Could be older MIFARE or unknown card type
		cardInfo.Type = "Unknown ISO 14443/15693 tag"
		cardInfo.Writable = true
		return
	}

	// Default fallback
	cardInfo.Type = "NFC Tag (type unknown)"
	cardInfo.Writable = true
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findIndex(s, substr) >= 0
}

func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// WriteData writes data to an NFC tag. Supports JSON, text, binary, and URL data.
// For NTAG cards, data is written as NDEF records starting at page 4.
func WriteData(readerName string, data []byte, dataType string) error {
	return WriteDataWithURL(readerName, data, dataType, "")
}

// WriteDataWithURL writes data to an NTAG card with an optional URL as the first record.
// If url is non-empty, it creates a multi-record NDEF message with URL first, then data.
func WriteDataWithURL(readerName string, data []byte, dataType string, url string) error {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Get card status to obtain ATR for type detection
	status, err := card.Status()
	if err != nil {
		return fmt.Errorf("failed to get card status: %w", err)
	}

	// Detect card type to determine write method
	cardInfo := &Card{
		ATR: hex.EncodeToString(status.Atr),
	}
	detectCardType(card, cardInfo)

	var ndefMessage []byte

	// If URL is provided and there's also data, create multi-record message
	if url != "" && len(data) > 0 {
		ndefMessage = createMultiRecordNDEF(url, data, dataType)
	} else if url != "" {
		// URL only
		ndefMessage = createNDEFURIRecord(url)
	} else {
		// Data only - create NDEF message based on data type
		switch dataType {
		case "json":
			ndefMessage = createNDEFMimeRecord("application/json", data)
		case "text":
			ndefMessage = createNDEFTextRecord(string(data))
		case "binary":
			ndefMessage = createNDEFMimeRecord("application/octet-stream", data)
		case "url":
			ndefMessage = createNDEFURIRecord(string(data))
		case "openprinttag":
			// Parse JSON input and encode to CBOR
			var input openprinttag.Input
			if err := json.Unmarshal(data, &input); err != nil {
				return fmt.Errorf("invalid openprinttag JSON: %w", err)
			}
			cborPayload, err := input.Encode()
			if err != nil {
				return fmt.Errorf("failed to encode openprinttag: %w", err)
			}
			ndefMessage = createNDEFMimeRecord(openprinttag.MIMEType, cborPayload)
		default:
			return fmt.Errorf("unsupported data type: %s (use 'json', 'text', 'binary', 'url', or 'openprinttag')", dataType)
		}
	}

	// Write NDEF message based on card type
	if cardInfo.Type == "MIFARE Classic" {
		if err := writeMifareClassic(card, ndefMessage); err != nil {
			return fmt.Errorf("failed to write NDEF message: %w", err)
		}
	} else {
		// NTAG and other cards use page-based writes
		if err := writeNTAGPages(card, 4, ndefMessage); err != nil {
			return fmt.Errorf("failed to write NDEF message: %w", err)
		}
	}

	return nil
}

// createMultiRecordNDEF creates an NDEF message with URL as first record and data as second
func createMultiRecordNDEF(url string, data []byte, dataType string) []byte {
	// Create URI record (first record, MB=true, ME=false)
	prefixCode, remainder := findURIPrefix(url)
	uriPayload := []byte{prefixCode}
	uriPayload = append(uriPayload, []byte(remainder)...)
	uriRecord := createNDEFRecordRaw(0x01, []byte("U"), uriPayload, true, false)

	// Create data record (second record, MB=false, ME=true)
	var dataRecord []byte
	switch dataType {
	case "json":
		dataRecord = createNDEFRecordRaw(0x02, []byte("application/json"), data, false, true)
	case "text":
		textPayload := []byte{0x02}
		textPayload = append(textPayload, []byte("en")...)
		textPayload = append(textPayload, data...)
		dataRecord = createNDEFRecordRaw(0x01, []byte("T"), textPayload, false, true)
	case "binary":
		dataRecord = createNDEFRecordRaw(0x02, []byte("application/octet-stream"), data, false, true)
	case "openprinttag":
		// Data is already CBOR-encoded at this point
		dataRecord = createNDEFRecordRaw(0x02, []byte(openprinttag.MIMEType), data, false, true)
	default:
		textPayload := []byte{0x02}
		textPayload = append(textPayload, []byte("en")...)
		textPayload = append(textPayload, data...)
		dataRecord = createNDEFRecordRaw(0x01, []byte("T"), textPayload, false, true)
	}

	// Combine records
	ndefMessage := append(uriRecord, dataRecord...)

	// Wrap in TLV format
	tlv := []byte{0x03}
	if len(ndefMessage) < 255 {
		tlv = append(tlv, byte(len(ndefMessage)))
	} else {
		tlv = append(tlv, 0xFF)
		tlv = append(tlv, byte(len(ndefMessage)>>8))
		tlv = append(tlv, byte(len(ndefMessage)))
	}
	tlv = append(tlv, ndefMessage...)
	tlv = append(tlv, 0xFE)

	return tlv
}

// createNDEFRecordRaw creates a raw NDEF record without TLV wrapping
func createNDEFRecordRaw(tnf byte, recordType []byte, payload []byte, mb bool, me bool) []byte {
	header := tnf & 0x07
	if mb {
		header |= 0x80
	}
	if me {
		header |= 0x40
	}
	if len(payload) < 256 {
		header |= 0x10
	}

	record := []byte{header}
	record = append(record, byte(len(recordType)))

	if len(payload) < 256 {
		record = append(record, byte(len(payload)))
	} else {
		record = append(record, byte(len(payload)>>24))
		record = append(record, byte(len(payload)>>16))
		record = append(record, byte(len(payload)>>8))
		record = append(record, byte(len(payload)))
	}

	record = append(record, recordType...)
	record = append(record, payload...)

	return record
}

// createNDEFTextRecord creates an NDEF text record
func createNDEFTextRecord(text string) []byte {
	payload := []byte{0x02} // Language code length (2 for "en")
	payload = append(payload, []byte("en")...)
	payload = append(payload, []byte(text)...)

	return createNDEFRecord(0xD1, []byte("T"), payload, true, true)
}

// createNDEFMimeRecord creates an NDEF MIME type record
func createNDEFMimeRecord(mimeType string, data []byte) []byte {
	return createNDEFRecord(0xD2, []byte(mimeType), data, true, true)
}

// createNDEFURIRecord creates an NDEF URI record
func createNDEFURIRecord(uri string) []byte {
	// Find the best URI prefix to use
	prefixCode, remainder := findURIPrefix(uri)
	payload := []byte{prefixCode}
	payload = append(payload, []byte(remainder)...)
	return createNDEFRecord(0xD1, []byte("U"), payload, true, true)
}

// findURIPrefix finds the best matching URI prefix code for a given URL
func findURIPrefix(uri string) (byte, string) {
	prefixes := []struct {
		code   byte
		prefix string
	}{
		{0x04, "https://"},
		{0x03, "http://"},
		{0x02, "https://www."},
		{0x01, "http://www."},
		{0x05, "tel:"},
		{0x06, "mailto:"},
	}

	for _, p := range prefixes {
		if len(uri) >= len(p.prefix) && uri[:len(p.prefix)] == p.prefix {
			return p.code, uri[len(p.prefix):]
		}
	}
	return 0x00, uri // No prefix match, use full URI
}

// createNDEFRecord creates a basic NDEF record with TLV wrapping
func createNDEFRecord(tnf byte, recordType []byte, payload []byte, mb bool, me bool) []byte {
	// NDEF Record format:
	// Header byte: MB ME CF SR IL TNF
	header := tnf & 0x07 // TNF (Type Name Format)
	if mb {
		header |= 0x80 // MB (Message Begin)
	}
	if me {
		header |= 0x40 // ME (Message End)
	}
	if len(payload) < 256 {
		header |= 0x10 // SR (Short Record)
	}

	record := []byte{header}
	record = append(record, byte(len(recordType))) // Type length

	if len(payload) < 256 {
		record = append(record, byte(len(payload))) // Payload length (short)
	} else {
		// Long record format (not commonly needed for small data)
		record = append(record, byte(len(payload)>>24))
		record = append(record, byte(len(payload)>>16))
		record = append(record, byte(len(payload)>>8))
		record = append(record, byte(len(payload)))
	}

	record = append(record, recordType...)
	record = append(record, payload...)

	// Wrap in TLV (Type-Length-Value) format
	// TLV format: 0x03 (NDEF Message TLV), Length, Value, 0xFE (Terminator TLV)
	tlv := []byte{0x03} // NDEF Message TLV
	if len(record) < 255 {
		tlv = append(tlv, byte(len(record)))
	} else {
		tlv = append(tlv, 0xFF)
		tlv = append(tlv, byte(len(record)>>8))
		tlv = append(tlv, byte(len(record)))
	}
	tlv = append(tlv, record...)
	tlv = append(tlv, 0xFE) // Terminator TLV

	return tlv
}

// writeMifareClassic writes NDEF data to a MIFARE Classic card
// Tries multiple common keys for authentication
func writeMifareClassic(card *scard.Card, data []byte) error {
	// Common keys to try:
	// 1. Default transport key
	// 2. NFC Forum default key
	// 3. MAD key (for sector 0)
	// 4. NDEF key
	keys := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // Default transport
		{0xD3, 0xF7, 0xD3, 0xF7, 0xD3, 0xF7}, // NFC Forum default
		{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}, // MAD key
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // Zero key
	}

	// For MIFARE Classic 1K NDEF:
	// - Sector 0 contains MAD (skip it)
	// - NDEF data starts at sector 1, block 4
	// - Each sector has 4 blocks, last block is sector trailer (skip it)

	// Pad data to 16-byte blocks
	for len(data)%16 != 0 {
		data = append(data, 0x00)
	}

	blockNum := 4 // Start at sector 1, block 0 (absolute block 4)
	dataOffset := 0
	lastAuthSector := -1
	currentKeyIndex := -1

	for dataOffset < len(data) {
		// Skip sector trailers (every 4th block starting from block 3)
		if (blockNum+1)%4 == 0 {
			blockNum++
			continue
		}

		sector := blockNum / 4

		// Authenticate to the sector if we moved to a new sector
		if sector != lastAuthSector {
			authBlock := sector*4 + 3 // Sector trailer block
			authenticated := false

			// Try each key
			for keyIdx, key := range keys {
				// Load key into reader's key slot 0
				loadKeyCmd := []byte{0xFF, 0x82, 0x00, 0x00, 0x06}
				loadKeyCmd = append(loadKeyCmd, key...)

				rsp, err := card.Transmit(loadKeyCmd)
				if err != nil || len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
					continue
				}

				// Try Key A authentication
				authCmd := []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, byte(authBlock), 0x60, 0x00}
				rsp, err = card.Transmit(authCmd)
				if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
					authenticated = true
					currentKeyIndex = keyIdx
					break
				}

				// Try Key B authentication
				authCmd = []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, byte(authBlock), 0x61, 0x00}
				rsp, err = card.Transmit(authCmd)
				if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
					authenticated = true
					currentKeyIndex = keyIdx
					break
				}
			}

			if !authenticated {
				return fmt.Errorf("authentication failed for sector %d (block %d) - no valid key found", sector, blockNum)
			}
			lastAuthSector = sector
		}

		// Write 16 bytes to block
		blockData := data[dataOffset : dataOffset+16]
		writeCmd := []byte{0xFF, 0xD6, 0x00, byte(blockNum), 0x10}
		writeCmd = append(writeCmd, blockData...)

		rsp, err := card.Transmit(writeCmd)
		if err != nil {
			return fmt.Errorf("failed to write block %d: %w", blockNum, err)
		}
		if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
			return fmt.Errorf("write failed at block %d: status %02X %02X (key index %d)", blockNum, rsp[len(rsp)-2], rsp[len(rsp)-1], currentKeyIndex)
		}

		blockNum++
		dataOffset += 16
	}

	return nil
}

// writeNTAGPages writes data to NTAG card pages (4 bytes per page)
func writeNTAGPages(card *scard.Card, startPage int, data []byte) error {
	// Pad data to multiple of 4 bytes
	for len(data)%4 != 0 {
		data = append(data, 0x00)
	}

	// Write 4 bytes at a time (one page per write)
	for i := 0; i < len(data); i += 4 {
		pageNum := startPage + (i / 4)
		pageData := data[i : i+4]

		// Method 0: Raw NTAG WRITE command - some readers pass through raw NFC commands
		// Format: A2 [page] [4 bytes data]
		rawCmd := []byte{0xA2, byte(pageNum)}
		rawCmd = append(rawCmd, pageData...)
		rsp, err := card.Transmit(rawCmd)
		if err == nil && len(rsp) >= 1 {
			// NTAG write returns ACK (0x0A) on success
			if rsp[0] == 0x0A || (len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00) {
				logging.Debug(logging.CatCard, "NDEF page written", map[string]any{
					"page":   pageNum,
					"data":   hex.EncodeToString(pageData),
					"method": 0,
				})
				continue // Success, next page
			}
		}

		// Method 1: Standard UPDATE BINARY command (works on most readers including ACR1252U)
		// APDU: FF D6 00 [page] 04 [4 bytes]
		writeCmd := []byte{0xFF, 0xD6, 0x00, byte(pageNum), 0x04}
		writeCmd = append(writeCmd, pageData...)
		rsp, err = card.Transmit(writeCmd)
		logging.Debug(logging.CatCard, "NDEF write method 1", map[string]any{
			"page":     pageNum,
			"cmd":      hex.EncodeToString(writeCmd),
			"response": hex.EncodeToString(rsp),
			"err":      fmt.Sprintf("%v", err),
		})
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
			logging.Debug(logging.CatCard, "NDEF page written", map[string]any{
				"page":   pageNum,
				"data":   hex.EncodeToString(pageData),
				"method": 1,
			})
			continue // Success, next page
		}

		// Method 2: ACR122U InCommunicateThru with native WRITE command (0xA2)
		// Format: FF 00 00 00 08 D4 42 A2 [page] [4 bytes data]
		directCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x08, 0xD4, 0x42, 0xA2, byte(pageNum)}
		directCmd = append(directCmd, pageData...)
		rsp, err = card.Transmit(directCmd)
		logging.Debug(logging.CatCard, "NDEF write method 2", map[string]any{
			"page":     pageNum,
			"cmd":      hex.EncodeToString(directCmd),
			"response": hex.EncodeToString(rsp),
			"err":      fmt.Sprintf("%v", err),
		})
		if err == nil && len(rsp) >= 2 {
			sw1, sw2 := rsp[len(rsp)-2], rsp[len(rsp)-1]
			if sw1 == 0x90 && sw2 == 0x00 {
				// Check inner status if present (D5 43 XX format)
				if len(rsp) >= 3 && rsp[0] == 0xD5 && rsp[1] == 0x43 {
					if rsp[2] != 0x00 {
						return fmt.Errorf("write failed at page %d: card error %02X", pageNum, rsp[2])
					}
				}
				logging.Debug(logging.CatCard, "NDEF page written", map[string]any{
					"page":   pageNum,
					"data":   hex.EncodeToString(pageData),
					"method": 2,
				})
				continue // Success, next page
			}
		}

		// Method 3: ACR1552 Transparent Exchange with native WRITE command (0xA2)
		// Requires: start session, set protocol, send command, end session
		startSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x81, 0x00}
		setProtocol := []byte{0xFF, 0xC2, 0x00, 0x02, 0x04, 0x8F, 0x02, 0x00, 0x03}
		endSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x82, 0x00}

		// End any stale session first (ignore result)
		card.Transmit(endSession)

		rsp, err = card.Transmit(startSession)
		logging.Debug(logging.CatCard, "NDEF write method 3 - start session", map[string]any{
			"page":     pageNum,
			"response": hex.EncodeToString(rsp),
			"err":      fmt.Sprintf("%v", err),
		})
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
			// Set protocol to ISO 14443-A Layer 3
			rsp, err = card.Transmit(setProtocol)
			logging.Debug(logging.CatCard, "NDEF write method 3 - set protocol", map[string]any{
				"page":     pageNum,
				"response": hex.EncodeToString(rsp),
				"err":      fmt.Sprintf("%v", err),
			})
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				// Build transparent write command: A2 [page] [4 bytes]
				writeData := []byte{0xA2, byte(pageNum)}
				writeData = append(writeData, pageData...)
				// Wrap in transparent exchange: FF C2 00 01 [len+2] 95 [len] [data]
				transparentCmd := []byte{0xFF, 0xC2, 0x00, 0x01, byte(len(writeData) + 2), 0x95, byte(len(writeData))}
				transparentCmd = append(transparentCmd, writeData...)

				rsp, err = card.Transmit(transparentCmd)
				logging.Debug(logging.CatCard, "NDEF write method 3 - write cmd", map[string]any{
					"page":     pageNum,
					"cmd":      hex.EncodeToString(transparentCmd),
					"response": hex.EncodeToString(rsp),
					"err":      fmt.Sprintf("%v", err),
				})
				card.Transmit(endSession) // Always end session

				// Check for success - response contains status in TLV format
				if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
					logging.Debug(logging.CatCard, "NDEF page written", map[string]any{
						"page":   pageNum,
						"data":   hex.EncodeToString(pageData),
						"method": 3,
					})
					continue // Success, next page
				}
			} else {
				card.Transmit(endSession)
			}
		}

		return fmt.Errorf("write failed at page %d: no supported method worked", pageNum)
	}

	return nil
}

// WaitForCard waits for a card to be present on the specified reader.
// This is a blocking call that returns when a card is detected.
func WaitForCard(readerName string) error {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	// Create reader state for monitoring
	rs := []scard.ReaderState{
		{
			Reader:       readerName,
			CurrentState: scard.StateUnaware,
		},
	}

	// Wait for card present state
	err = ctx.GetStatusChange(rs, -1) // -1 means infinite timeout
	if err != nil {
		return fmt.Errorf("failed to get status change: %w", err)
	}

	// Check if card is present
	if rs[0].EventState&scard.StatePresent == 0 {
		return fmt.Errorf("no card detected")
	}

	return nil
}

// readMifareClassicBlock reads a 16-byte block from a MIFARE Classic card
// Handles authentication with multiple common keys
func readMifareClassicBlock(card *scard.Card, blockNum int, lastAuthSector *int) ([]byte, error) {
	keys := [][]byte{
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // Default transport
		{0xD3, 0xF7, 0xD3, 0xF7, 0xD3, 0xF7}, // NFC Forum default
		{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}, // MAD key
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // Zero key
	}

	sector := blockNum / 4

	// Authenticate if we're in a new sector
	if *lastAuthSector != sector {
		authBlock := sector*4 + 3
		authenticated := false

		for _, key := range keys {
			// Load key
			loadKeyCmd := []byte{0xFF, 0x82, 0x00, 0x00, 0x06}
			loadKeyCmd = append(loadKeyCmd, key...)
			rsp, err := card.Transmit(loadKeyCmd)
			if err != nil || len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
				continue
			}

			// Try Key A
			authCmd := []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, byte(authBlock), 0x60, 0x00}
			rsp, err = card.Transmit(authCmd)
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				authenticated = true
				break
			}

			// Try Key B
			authCmd = []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, byte(authBlock), 0x61, 0x00}
			rsp, err = card.Transmit(authCmd)
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				authenticated = true
				break
			}
		}

		if !authenticated {
			return nil, fmt.Errorf("authentication failed for sector %d", sector)
		}
		*lastAuthSector = sector
	}

	// Read block: FF B0 00 [block] 10
	readCmd := []byte{0xFF, 0xB0, 0x00, byte(blockNum), 0x10}
	rsp, err := card.Transmit(readCmd)
	if err != nil {
		return nil, err
	}
	if len(rsp) < 18 || rsp[len(rsp)-2] != 0x90 {
		return nil, fmt.Errorf("read failed: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	return rsp[:16], nil
}

// readNTAGPage reads a single 4-byte page from an NTAG card
// Uses fallback to ACR122U direct transmit if standard command fails
func readNTAGPage(card *scard.Card, pageNum int) ([]byte, error) {
	// Method 1: Standard READ BINARY command
	readCmd := []byte{0xFF, 0xB0, 0x00, byte(pageNum), 0x04}
	rsp, err := card.Transmit(readCmd)

	if err == nil && len(rsp) >= 6 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		return rsp[:len(rsp)-2], nil
	}

	// Method 2: ACR122U InCommunicateThru with NTAG READ command (0x30)
	// Format: FF 00 00 00 [len] D4 42 30 [page]
	// NTAG READ returns 16 bytes (4 pages starting from pageNum)
	directCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x04, 0xD4, 0x42, 0x30, byte(pageNum)}
	rsp, err = card.Transmit(directCmd)

	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	// ACR122U returns: D5 43 00 [16 bytes of data] 90 00
	if len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		if len(rsp) >= 19 && rsp[0] == 0xD5 && rsp[1] == 0x43 && rsp[2] == 0x00 {
			// Return first 4 bytes (one page) from the 16-byte response
			return rsp[3:7], nil
		}
	}

	return nil, fmt.Errorf("read failed with status: %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
}

// readNDEFData attempts to read NDEF data from a card
func readNDEFData(card *scard.Card, cardInfo *Card) {
	logging.Debug(logging.CatCard, "Reading NDEF data", map[string]any{
		"cardType": cardInfo.Type,
	})

	var allData []byte

	if cardInfo.Type == "MIFARE Classic" {
		// MIFARE Classic: read blocks starting from sector 1 (block 4)
		lastAuthSector := -1
		for blockNum := 4; blockNum < 64; blockNum++ { // MIFARE 1K has 64 blocks
			// Skip sector trailers
			if (blockNum+1)%4 == 0 {
				continue
			}

			blockData, err := readMifareClassicBlock(card, blockNum, &lastAuthSector)
			if err != nil {
				logging.Debug(logging.CatCard, "NDEF read failed", map[string]any{
					"block": blockNum,
					"error": err.Error(),
				})
				break
			}

			allData = append(allData, blockData...)

			// Check for NDEF terminator
			for _, b := range blockData {
				if b == 0xFE {
					goto done
				}
			}

			// Check if we have complete NDEF message
			if len(allData) > 2 && allData[0] == 0x03 {
				var ndefLength, ndefStart int
				if allData[1] == 0xFF && len(allData) >= 4 {
					ndefLength = int(allData[2])<<8 | int(allData[3])
					ndefStart = 4
				} else if allData[1] != 0xFF {
					ndefLength = int(allData[1])
					ndefStart = 2
				}
				if ndefStart > 0 && len(allData) >= ndefStart+ndefLength+1 {
					break
				}
			}
		}
	} else if cardInfo.Type == "ICode SLIX" || cardInfo.Type == "ICode SLIX2" || cardInfo.Type == "ICode SLI" || cardInfo.Type == "ISO 15693" {
		// ISO 15693 (Type 5) tags: NDEF starts at block 1 (after CC at block 0)
		maxBlocks := 79 // 80 blocks total, skip CC at block 0
		for blockNum := 1; blockNum < 1+maxBlocks; blockNum++ {
			blockData, err := readNTAGPage(card, blockNum)
			if err != nil {
				logging.Debug(logging.CatCard, "NDEF read failed", map[string]any{
					"block": blockNum,
					"error": err.Error(),
				})
				break
			}

			allData = append(allData, blockData...)

			// Check for NDEF terminator
			for _, b := range blockData {
				if b == 0xFE {
					goto done
				}
			}

			// Check if we have complete NDEF message
			if len(allData) > 2 && allData[0] == 0x03 {
				var ndefLength, ndefStart int
				if allData[1] == 0xFF && len(allData) >= 4 {
					ndefLength = int(allData[2])<<8 | int(allData[3])
					ndefStart = 4
				} else if allData[1] != 0xFF {
					ndefLength = int(allData[1])
					ndefStart = 2
				}
				if ndefStart > 0 && len(allData) >= ndefStart+ndefLength+1 {
					break
				}
			}
		}
	} else {
		// NTAG and other cards: read pages starting from page 4
		maxPages := 40
		if cardInfo.Type == "NTAG215" {
			maxPages = 126
		} else if cardInfo.Type == "NTAG216" {
			maxPages = 222
		}

		for pageNum := 4; pageNum < 4+maxPages; pageNum++ {
			pageData, err := readNTAGPage(card, pageNum)
			if err != nil {
				logging.Debug(logging.CatCard, "NDEF read failed", map[string]any{
					"page":  pageNum,
					"error": err.Error(),
				})
				break
			}

			allData = append(allData, pageData...)

			// Check for NDEF terminator
			for _, b := range pageData {
				if b == 0xFE {
					goto done
				}
			}

			// Check if we have complete NDEF message
			if len(allData) > 2 && allData[0] == 0x03 {
				var ndefLength, ndefStart int
				if allData[1] == 0xFF && len(allData) >= 4 {
					ndefLength = int(allData[2])<<8 | int(allData[3])
					ndefStart = 4
				} else if allData[1] != 0xFF {
					ndefLength = int(allData[1])
					ndefStart = 2
				}
				if ndefStart > 0 && len(allData) >= ndefStart+ndefLength+1 {
					break
				}
			}
		}
	}
done:

	logging.Debug(logging.CatCard, "NDEF data read complete", map[string]any{
		"totalBytes": len(allData),
	})

	if len(allData) < 3 {
		logging.Debug(logging.CatCard, "Not enough NDEF data", map[string]any{
			"bytes": len(allData),
		})
		return // Can't read data, leave fields empty
	}

	data := allData

	// Parse NDEF TLV format
	if len(data) < 3 || data[0] != 0x03 {
		return // Not NDEF format
	}

	// Get NDEF message length
	var ndefLength int
	var ndefStart int
	if data[1] == 0xFF {
		// 3-byte length format
		if len(data) < 5 {
			return
		}
		ndefLength = int(data[2])<<8 | int(data[3])
		ndefStart = 4
	} else {
		// 1-byte length format
		ndefLength = int(data[1])
		ndefStart = 2
	}

	if ndefStart+ndefLength > len(data) {
		return // Invalid length
	}

	ndefMessage := data[ndefStart : ndefStart+ndefLength]

	// Parse all NDEF records
	offset := 0
	for offset < len(ndefMessage) {
		if len(ndefMessage)-offset < 3 {
			break
		}

		header := ndefMessage[offset]
		tnf := header & 0x07
		sr := (header & 0x10) != 0
		me := (header & 0x40) != 0 // Message End flag

		typeLength := int(ndefMessage[offset+1])
		var payloadLength int
		var headerSize int

		if sr {
			// Short record
			payloadLength = int(ndefMessage[offset+2])
			headerSize = 3
		} else {
			// Long record
			if len(ndefMessage)-offset < 6 {
				break
			}
			payloadLength = int(ndefMessage[offset+2])<<24 | int(ndefMessage[offset+3])<<16 | int(ndefMessage[offset+4])<<8 | int(ndefMessage[offset+5])
			headerSize = 6
		}

		recordStart := offset + headerSize
		if recordStart+typeLength+payloadLength > len(ndefMessage) {
			break
		}

		recordType := ndefMessage[recordStart : recordStart+typeLength]
		payload := ndefMessage[recordStart+typeLength : recordStart+typeLength+payloadLength]

		// Process this record
		if tnf == 0x01 && len(recordType) == 1 && recordType[0] == 'U' {
			// URI record - store in URL field
			if len(payload) >= 1 {
				uriPrefix := getURIPrefix(payload[0])
				cardInfo.URL = uriPrefix + string(payload[1:])
			}
		} else if tnf == 0x01 && len(recordType) == 1 && recordType[0] == 'T' {
			// Text record
			if len(payload) >= 1 {
				langCodeLen := int(payload[0] & 0x3F)
				if 1+langCodeLen <= len(payload) {
					cardInfo.Data = string(payload[1+langCodeLen:])
					cardInfo.DataType = "text"
				}
			}
		} else if tnf == 0x02 {
			// MIME type record
			mimeType := string(recordType)
			if mimeType == "application/json" {
				cardInfo.Data = string(payload)
				cardInfo.DataType = "json"
			} else if mimeType == openprinttag.MIMEType || mimeType == "application/cbor" {
				// OpenPrintTag format (application/vnd.openprinttag or application/cbor)
				opt, err := openprinttag.Decode(payload)
				if err == nil {
					resp := opt.ToResponse()
					jsonData, _ := json.Marshal(resp)
					cardInfo.Data = string(jsonData)
					cardInfo.DataType = "openprinttag"
				} else {
					// Fallback to binary if CBOR decode fails
					cardInfo.Data = hex.EncodeToString(payload)
					cardInfo.DataType = "binary"
				}
			} else if mimeType == "application/octet-stream" {
				cardInfo.Data = hex.EncodeToString(payload)
				cardInfo.DataType = "binary"
			} else {
				cardInfo.Data = string(payload)
				cardInfo.DataType = "unknown"
			}
		}

		// Move to next record
		offset = recordStart + typeLength + payloadLength

		if me {
			break // Last record
		}
	}

	// If we only have a URL and no other data, put URL in Data field too for backwards compatibility
	if cardInfo.URL != "" && cardInfo.Data == "" {
		cardInfo.Data = cardInfo.URL
		cardInfo.DataType = "url"
	}
}

// getURIPrefix returns the URI prefix for NDEF URI record identifier codes
func getURIPrefix(code byte) string {
	prefixes := map[byte]string{
		0x00: "",
		0x01: "http://www.",
		0x02: "https://www.",
		0x03: "http://",
		0x04: "https://",
		0x05: "tel:",
		0x06: "mailto:",
		0x07: "ftp://anonymous:anonymous@",
		0x08: "ftp://ftp.",
		0x09: "ftps://",
		0x0A: "sftp://",
		0x0B: "smb://",
		0x0C: "nfs://",
		0x0D: "ftp://",
		0x0E: "dav://",
		0x0F: "news:",
		0x10: "telnet://",
		0x11: "imap:",
		0x12: "rtsp://",
		0x13: "urn:",
		0x14: "pop:",
		0x15: "sip:",
		0x16: "sips:",
		0x17: "tftp:",
		0x18: "btspp://",
		0x19: "btl2cap://",
		0x1A: "btgoep://",
		0x1B: "tcpobex://",
		0x1C: "irdaobex://",
		0x1D: "file://",
		0x1E: "urn:epc:id:",
		0x1F: "urn:epc:tag:",
		0x20: "urn:epc:pat:",
		0x21: "urn:epc:raw:",
		0x22: "urn:epc:",
		0x23: "urn:nfc:",
	}
	if prefix, ok := prefixes[code]; ok {
		return prefix
	}
	return ""
}

// EraseCard erases all NDEF data from an NFC tag by writing an empty NDEF message
func EraseCard(readerName string) error {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Write an empty NDEF message (just TLV header and terminator)
	// 0x03 = NDEF TLV, 0x00 = length, 0xFE = terminator
	emptyNDEF := []byte{0x03, 0x00, 0xFE, 0x00}

	if err := writeNTAGPages(card, 4, emptyNDEF); err != nil {
		return fmt.Errorf("failed to erase card: %w", err)
	}

	return nil
}

// LockCard makes an NTAG card permanently read-only by setting the lock bits
// WARNING: This is IRREVERSIBLE! Once locked, the card cannot be written to again.
func LockCard(readerName string) error {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// For NTAG cards, page 2 contains the static lock bytes at bytes 2-3
	// Setting bits in these bytes locks pages permanently

	// First, read page 2 to preserve UID bytes
	readCmd := []byte{0xFF, 0xB0, 0x00, 0x02, 0x04}
	rsp, err := card.Transmit(readCmd)
	if err != nil {
		return fmt.Errorf("failed to read page 2: %w", err)
	}
	if len(rsp) < 6 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to read page 2: bad response")
	}

	// Keep bytes 0-1 (UID), set bytes 2-3 (lock bytes) to lock all pages
	page2Data := []byte{rsp[0], rsp[1], 0xFF, 0xFF}

	// Write the lock bytes
	writeCmd := []byte{0xFF, 0xD6, 0x00, 0x02, 0x04}
	writeCmd = append(writeCmd, page2Data...)

	rsp, err = card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write lock bytes: %w", err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to write lock bytes: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	// For NTAG cards, there are also dynamic lock bytes at page 40 (NTAG213)
	// or page 130 (NTAG215) or page 226 (NTAG216)
	// We'll set these too for complete locking

	// Detect card type to know where dynamic lock bytes are
	cardInfo := &Card{}
	detectCardType(card, cardInfo)

	var dynamicLockPage int
	switch cardInfo.Type {
	case "NTAG213":
		dynamicLockPage = 40
	case "NTAG215":
		dynamicLockPage = 130
	case "NTAG216":
		dynamicLockPage = 226
	default:
		// Unknown card type, skip dynamic locks
		return nil
	}

	// Write dynamic lock bytes (all 1s = all pages locked)
	dynamicLockData := []byte{0xFF, 0xFF, 0xFF, 0x00}
	writeCmd = []byte{0xFF, 0xD6, 0x00, byte(dynamicLockPage), 0x04}
	writeCmd = append(writeCmd, dynamicLockData...)

	_, err = card.Transmit(writeCmd)
	if err != nil {
		// Dynamic locks may fail on some cards, but static locks were set
		logging.Warn(logging.CatCard, "Failed to set dynamic lock bytes", map[string]any{
			"error": err.Error(),
		})
	}

	return nil
}

// SetPassword sets a password on an NTAG card (NTAG213/215/216 only)
// The password protects pages from the specified startPage onwards
// Note: Password is 4 bytes, PACK (password acknowledge) is 2 bytes
func SetPassword(readerName string, password []byte, pack []byte, startPage byte) error {
	if len(password) != 4 {
		return fmt.Errorf("password must be exactly 4 bytes")
	}
	if len(pack) != 2 {
		return fmt.Errorf("PACK must be exactly 2 bytes")
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Detect card type to find config pages
	cardInfo := &Card{}
	detectCardType(card, cardInfo)

	var pwdPage, packPage, authPage int
	switch cardInfo.Type {
	case "NTAG213":
		pwdPage = 43
		packPage = 44
		authPage = 41
	case "NTAG215":
		pwdPage = 133
		packPage = 134
		authPage = 131
	case "NTAG216":
		pwdPage = 229
		packPage = 230
		authPage = 227
	default:
		return fmt.Errorf("password protection not supported for card type: %s", cardInfo.Type)
	}

	// Write password to PWD page
	writeCmd := []byte{0xFF, 0xD6, 0x00, byte(pwdPage), 0x04}
	writeCmd = append(writeCmd, password...)
	rsp, err := card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write password: %w", err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to write password: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	// Write PACK to PACK page (only 2 bytes, pad with zeros)
	packData := []byte{pack[0], pack[1], 0x00, 0x00}
	writeCmd = []byte{0xFF, 0xD6, 0x00, byte(packPage), 0x04}
	writeCmd = append(writeCmd, packData...)
	rsp, err = card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write PACK: %w", err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to write PACK: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	// Read AUTH0 page to preserve other config bits
	readCmd := []byte{0xFF, 0xB0, 0x00, byte(authPage), 0x04}
	rsp, err = card.Transmit(readCmd)
	if err != nil {
		return fmt.Errorf("failed to read AUTH0 page: %w", err)
	}
	if len(rsp) < 6 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to read AUTH0 page")
	}

	// Set AUTH0 (byte 3) to the start page for password protection
	// Pages >= AUTH0 require password
	authData := []byte{rsp[0], rsp[1], rsp[2], startPage}
	writeCmd = []byte{0xFF, 0xD6, 0x00, byte(authPage), 0x04}
	writeCmd = append(writeCmd, authData...)
	rsp, err = card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write AUTH0: %w", err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to write AUTH0: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	return nil
}

// RemovePassword removes password protection from an NTAG card
// Requires the current password to authenticate first
func RemovePassword(readerName string, password []byte) error {
	if len(password) != 4 {
		return fmt.Errorf("password must be exactly 4 bytes")
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Detect card type
	cardInfo := &Card{}
	detectCardType(card, cardInfo)

	var authPage int
	switch cardInfo.Type {
	case "NTAG213":
		authPage = 41
	case "NTAG215":
		authPage = 131
	case "NTAG216":
		authPage = 227
	default:
		return fmt.Errorf("password protection not supported for card type: %s", cardInfo.Type)
	}

	// Authenticate with current password (PWD_AUTH command via pseudo-APDU)
	authCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x07, 0xD4, 0x42, 0x1B}
	authCmd = append(authCmd, password...)
	rsp, err := card.Transmit(authCmd)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	// Check for successful auth (response contains PACK)
	if len(rsp) < 2 || (rsp[len(rsp)-2] != 0x90 && rsp[len(rsp)-2] != 0xD5) {
		return fmt.Errorf("authentication failed: wrong password")
	}

	// Set AUTH0 to 0xFF to disable password protection (all pages unprotected)
	readCmd := []byte{0xFF, 0xB0, 0x00, byte(authPage), 0x04}
	rsp, err = card.Transmit(readCmd)
	if err != nil {
		return fmt.Errorf("failed to read AUTH0 page: %w", err)
	}
	if len(rsp) < 6 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to read AUTH0 page")
	}

	// Set AUTH0 to 0xFF (no pages protected)
	authData := []byte{rsp[0], rsp[1], rsp[2], 0xFF}
	writeCmd := []byte{0xFF, 0xD6, 0x00, byte(authPage), 0x04}
	writeCmd = append(writeCmd, authData...)
	rsp, err = card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to disable password: %w", err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("failed to disable password: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	return nil
}

// WriteMultipleRecords writes multiple NDEF records to a card
type NDEFRecord struct {
	Type     string `json:"type"`               // "url", "text", "json", "binary", "mime"
	Data     string `json:"data"`               // Data content
	MimeType string `json:"mimeType,omitempty"` // For generic mime records (e.g., "application/vnd.openprinttag")
	DataType string `json:"dataType,omitempty"` // "binary" for base64-encoded data
}

func WriteMultipleRecords(readerName string, records []NDEFRecord) error {
	if len(records) == 0 {
		return fmt.Errorf("no records to write")
	}

	// Dispatch to Proxmark3 if this is a Proxmark3 reader
	if IsProxmark3Reader(readerName) {
		return writeMultipleRecordsProxmark3(readerName, records)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Get ATR to detect card type
	status, err := card.Status()
	if err != nil {
		return fmt.Errorf("failed to get card status: %w", err)
	}
	atr := hex.EncodeToString(status.Atr)
	isISO15693 := contains(atr, "03060b")

	// Build multi-record NDEF message
	var ndefRecords []byte
	for i, rec := range records {
		isFirst := i == 0
		isLast := i == len(records)-1

		var recordBytes []byte
		switch rec.Type {
		case "url":
			prefixCode, remainder := findURIPrefix(rec.Data)
			payload := []byte{prefixCode}
			payload = append(payload, []byte(remainder)...)
			recordBytes = createNDEFRecordRaw(0x01, []byte("U"), payload, isFirst, isLast)
		case "text":
			payload := []byte{0x02}
			payload = append(payload, []byte("en")...)
			payload = append(payload, []byte(rec.Data)...)
			recordBytes = createNDEFRecordRaw(0x01, []byte("T"), payload, isFirst, isLast)
		case "json":
			recordBytes = createNDEFRecordRaw(0x02, []byte("application/json"), []byte(rec.Data), isFirst, isLast)
		case "binary":
			// Decode hex
			decoded, err := hex.DecodeString(rec.Data)
			if err != nil {
				return fmt.Errorf("invalid binary data in record %d: %w", i, err)
			}
			recordBytes = createNDEFRecordRaw(0x02, []byte("application/octet-stream"), decoded, isFirst, isLast)
		case "mime":
			if rec.MimeType == "" {
				return fmt.Errorf("mimeType required for mime record type in record %d", i)
			}
			var payload []byte
			if rec.DataType == "binary" {
				// Data is base64 encoded
				decoded, err := base64.StdEncoding.DecodeString(rec.Data)
				if err != nil {
					return fmt.Errorf("invalid base64 data in mime record %d: %w", i, err)
				}
				payload = decoded
			} else {
				payload = []byte(rec.Data)
			}
			recordBytes = createNDEFRecordRaw(0x02, []byte(rec.MimeType), payload, isFirst, isLast)
		default:
			return fmt.Errorf("unsupported record type: %s", rec.Type)
		}
		ndefRecords = append(ndefRecords, recordBytes...)
	}

	// Wrap in TLV format
	tlv := []byte{0x03}
	if len(ndefRecords) < 255 {
		tlv = append(tlv, byte(len(ndefRecords)))
	} else {
		tlv = append(tlv, 0xFF)
		tlv = append(tlv, byte(len(ndefRecords)>>8))
		tlv = append(tlv, byte(len(ndefRecords)))
	}
	tlv = append(tlv, ndefRecords...)
	tlv = append(tlv, 0xFE)

	if isISO15693 {
		// ISO 15693 (Type 5) tags: CC at block 0, NDEF at block 1
		// CC format: E1 [version/access] [size/8] [features]
		// - 0xE1: Magic number
		// - 0x40: Version 1.0 (4), read/write access (0)
		// - Size: Available memory / 8 (we'll use 0x40 = 512 bytes, conservative)
		// - 0x00: No special features
		cc := []byte{0xE1, 0x40, 0x40, 0x00}

		// Write CC at block 0
		if err := writeNTAGPages(card, 0, cc); err != nil {
			return fmt.Errorf("failed to write CC block: %w", err)
		}

		// Write NDEF TLV starting at block 1
		if err := writeNTAGPages(card, 1, tlv); err != nil {
			return fmt.Errorf("failed to write NDEF records: %w", err)
		}
	} else {
		// NTAG (Type 2) tags: NDEF at page 4
		if err := writeNTAGPages(card, 4, tlv); err != nil {
			return fmt.Errorf("failed to write NDEF records: %w", err)
		}
	}

	return nil
}

// defaultMifareKeys contains common MIFARE Classic authentication keys
var defaultMifareKeys = [][]byte{
	{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, // Default transport
	{0xD3, 0xF7, 0xD3, 0xF7, 0xD3, 0xF7}, // NFC Forum default
	{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}, // MAD key
	{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // Zero key
}

// isSectorTrailer returns true if the block is a sector trailer (contains keys and access bits)
func isSectorTrailer(block int) bool {
	// For MIFARE Classic 1K (sectors 0-15, 4 blocks each), trailer is every 4th block starting at 3
	// For MIFARE Classic 4K, sectors 0-31 have 4 blocks, sectors 32-39 have 16 blocks
	if block < 128 {
		return (block+1)%4 == 0
	}
	// Large sectors (4K only): trailer at blocks 127+16n+15
	return (block-128+1)%16 == 0
}

// authenticateMifareBlock authenticates to the sector containing the given block
// If key is nil/empty, tries all default keys. keyType should be 0x60 (Key A) or 0x61 (Key B)
func authenticateMifareBlock(card *scard.Card, blockNum int, key []byte, keyType byte) error {
	sector := blockNum / 4
	if blockNum >= 128 {
		sector = 32 + (blockNum-128)/16
	}

	// Determine auth block (sector trailer)
	var authBlock int
	if blockNum < 128 {
		authBlock = sector*4 + 3
	} else {
		authBlock = 128 + (sector-32)*16 + 15
	}

	// Default to Key A if not specified
	if keyType != 0x60 && keyType != 0x61 {
		keyType = 0x60
	}

	// Determine which keys to try
	var keysToTry [][]byte
	if len(key) == 6 {
		keysToTry = [][]byte{key}
	} else {
		keysToTry = defaultMifareKeys
	}

	for _, k := range keysToTry {
		// Load key into reader's key slot 0
		loadKeyCmd := []byte{0xFF, 0x82, 0x00, 0x00, 0x06}
		loadKeyCmd = append(loadKeyCmd, k...)
		rsp, err := card.Transmit(loadKeyCmd)
		if err != nil || len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
			continue
		}

		// Try specified key type first
		authCmd := []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, byte(authBlock), keyType, 0x00}
		rsp, err = card.Transmit(authCmd)
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
			return nil // Success
		}

		// If using default keys, try the other key type as fallback
		if len(key) != 6 {
			otherKeyType := byte(0x61)
			if keyType == 0x61 {
				otherKeyType = 0x60
			}
			authCmd = []byte{0xFF, 0x86, 0x00, 0x00, 0x05, 0x01, 0x00, byte(authBlock), otherKeyType, 0x00}
			rsp, err = card.Transmit(authCmd)
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				return nil // Success
			}
		}
	}

	return fmt.Errorf("authentication failed for sector %d (block %d)", sector, blockNum)
}

// ReadMifareBlock reads a 16-byte block from a MIFARE Classic card.
// If key is nil/empty, tries default keys (FFFFFFFFFFFF, D3F7D3F7D3F7, etc.)
// keyType should be 'A' or 'B' (defaults to 'A')
func ReadMifareBlock(readerName string, block int, key []byte, keyType byte) ([]byte, error) {
	if block < 0 || block > 255 {
		return nil, fmt.Errorf("invalid block number: %d (must be 0-255)", block)
	}
	if isSectorTrailer(block) {
		return nil, fmt.Errorf("cannot read sector trailer block %d (contains authentication keys)", block)
	}

	// Dispatch to Proxmark3 if this is a Proxmark3 reader
	if IsProxmark3Reader(readerName) {
		return readMifareBlockProxmark3(readerName, block, key, keyType)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Convert key type character to APDU byte
	var keyTypeByte byte = 0x60 // Default Key A
	if keyType == 'B' || keyType == 'b' || keyType == 0x61 {
		keyTypeByte = 0x61
	}

	// Authenticate
	if err := authenticateMifareBlock(card, block, key, keyTypeByte); err != nil {
		return nil, err
	}

	// Read block: FF B0 00 [block] 10
	readCmd := []byte{0xFF, 0xB0, 0x00, byte(block), 0x10}
	rsp, err := card.Transmit(readCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %w", block, err)
	}
	if len(rsp) < 18 || rsp[len(rsp)-2] != 0x90 {
		return nil, fmt.Errorf("read failed for block %d: status %02X %02X", block, rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	logging.Info(logging.CatCard, "MIFARE block read", map[string]any{
		"block": block,
		"data":  hex.EncodeToString(rsp[:16]),
	})

	return rsp[:16], nil
}

// WriteMifareBlock writes 16 bytes to a MIFARE Classic block.
// If key is nil/empty, tries default keys (FFFFFFFFFFFF, D3F7D3F7D3F7, etc.)
// keyType should be 'A' or 'B' (defaults to 'A')
func WriteMifareBlock(readerName string, block int, data []byte, key []byte, keyType byte) error {
	if block < 0 || block > 255 {
		return fmt.Errorf("invalid block number: %d (must be 0-255)", block)
	}
	if isSectorTrailer(block) {
		return fmt.Errorf("cannot write to sector trailer block %d (contains authentication keys)", block)
	}
	if len(data) != 16 {
		return fmt.Errorf("data must be exactly 16 bytes, got %d", len(data))
	}

	// Dispatch to Proxmark3 if this is a Proxmark3 reader
	if IsProxmark3Reader(readerName) {
		return writeMifareBlockProxmark3(readerName, block, data, key, keyType)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Convert key type character to APDU byte
	var keyTypeByte byte = 0x60 // Default Key A
	if keyType == 'B' || keyType == 'b' || keyType == 0x61 {
		keyTypeByte = 0x61
	}

	// Authenticate
	if err := authenticateMifareBlock(card, block, key, keyTypeByte); err != nil {
		return err
	}

	// Write block: FF D6 00 [block] 10 [16 bytes]
	writeCmd := []byte{0xFF, 0xD6, 0x00, byte(block), 0x10}
	writeCmd = append(writeCmd, data...)
	rsp, err := card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write block %d: %w", block, err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("write failed for block %d: status %02X %02X", block, rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	logging.Info(logging.CatCard, "MIFARE block written", map[string]any{
		"block": block,
		"data":  hex.EncodeToString(data),
	})

	return nil
}

// ReadUltralightPage reads a 4-byte page from a MIFARE Ultralight card.
// page: Page number (0-255, actual range depends on card variant)
// password: Optional 4-byte password for EV1 variants (nil = no auth)
// Returns 4 bytes of page data.
func ReadUltralightPage(readerName string, page int, password []byte) ([]byte, error) {
	if page < 0 || page > 255 {
		return nil, fmt.Errorf("invalid page number: %d (must be 0-255)", page)
	}

	// Dispatch to Proxmark3 if this is a Proxmark3 reader
	if IsProxmark3Reader(readerName) {
		return readUltralightPageProxmark3(readerName, page, password)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Authenticate with password if provided (for Ultralight EV1)
	if len(password) > 0 {
		if err := authenticateUltralight(card, password); err != nil {
			return nil, err
		}
	}

	// Method 1: Standard READ BINARY command (works on most readers including ACR1252U)
	// APDU: FF B0 00 [page] 10 (reads 16 bytes = 4 pages)
	readCmd := []byte{0xFF, 0xB0, 0x00, byte(page), 0x10}
	rsp, err := card.Transmit(readCmd)
	if err == nil && len(rsp) >= 6 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		logging.Info(logging.CatCard, "Ultralight page read", map[string]any{
			"page":   page,
			"data":   hex.EncodeToString(rsp[:4]),
			"method": 1,
		})
		return rsp[:4], nil
	}

	// Method 2: ACR122U InCommunicateThru with native READ command (0x30)
	// Format: FF 00 00 00 04 D4 42 30 [page]
	directCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x04, 0xD4, 0x42, 0x30, byte(page)}
	rsp, err = card.Transmit(directCmd)
	if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		if len(rsp) >= 19 && rsp[0] == 0xD5 && rsp[1] == 0x43 && rsp[2] == 0x00 {
			logging.Info(logging.CatCard, "Ultralight page read", map[string]any{
				"page":   page,
				"data":   hex.EncodeToString(rsp[3:7]),
				"method": 2,
			})
			return rsp[3:7], nil
		}
	}

	// Method 3: ACR1552 Transparent Exchange with native READ command (0x30)
	// Requires: start session, set protocol, send command, end session
	startSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x81, 0x00}
	setProtocol := []byte{0xFF, 0xC2, 0x00, 0x02, 0x04, 0x8F, 0x02, 0x00, 0x03}
	endSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x82, 0x00}

	rsp, err = card.Transmit(startSession)
	if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
		// Set protocol to ISO 14443-A Layer 3
		rsp, err = card.Transmit(setProtocol)
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
			// Native READ command: 30 [page]
			transparentCmd := []byte{0xFF, 0xC2, 0x00, 0x01, 0x04, 0x95, 0x02, 0x30, byte(page)}
			rsp, err = card.Transmit(transparentCmd)
			card.Transmit(endSession) // Always end session

			// Response contains data in TLV format with tag 0x97
			// Parse to find the 16 bytes of page data
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				// Look for tag 0x97 containing the read data
				for i := 0; i < len(rsp)-2; i++ {
					if rsp[i] == 0x97 && i+1 < len(rsp) {
						tagLen := int(rsp[i+1])
						if i+2+tagLen <= len(rsp) && tagLen >= 4 {
							logging.Info(logging.CatCard, "Ultralight page read", map[string]any{
								"page":   page,
								"data":   hex.EncodeToString(rsp[i+2 : i+2+4]),
								"method": 3,
							})
							return rsp[i+2 : i+2+4], nil
						}
					}
				}
			}
		} else {
			card.Transmit(endSession)
		}
	}

	return nil, fmt.Errorf("read failed for page %d: no supported method worked", page)
}

// WriteUltralightPage writes 4 bytes to a MIFARE Ultralight page.
// page: Page number to write (minimum 4 for user data to protect system pages)
// data: Exactly 4 bytes to write
// password: Optional 4-byte password for EV1 variants (nil = no auth)
func WriteUltralightPage(readerName string, page int, data []byte, password []byte) error {
	if page < 0 || page > 255 {
		return fmt.Errorf("invalid page number: %d (must be 0-255)", page)
	}
	if page < 4 {
		return fmt.Errorf("cannot write to system pages 0-3 (use page 4 or higher for user data)")
	}
	if len(data) != 4 {
		return fmt.Errorf("data must be exactly 4 bytes, got %d", len(data))
	}

	// Dispatch to Proxmark3 if this is a Proxmark3 reader
	if IsProxmark3Reader(readerName) {
		return writeUltralightPageProxmark3(readerName, page, data, password)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Authenticate with password if provided (for Ultralight EV1)
	if len(password) > 0 {
		if err := authenticateUltralight(card, password); err != nil {
			return err
		}
	}

	// Method 0: Raw NTAG WRITE command - some readers pass through raw NFC commands
	// Format: A2 [page] [4 bytes data]
	rawCmd := []byte{0xA2, byte(page)}
	rawCmd = append(rawCmd, data...)
	rsp, err := card.Transmit(rawCmd)
	if err == nil && len(rsp) >= 1 {
		// NTAG write returns ACK (0x0A) on success
		if rsp[0] == 0x0A || (len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00) {
			logging.Info(logging.CatCard, "Ultralight page written", map[string]any{
				"page":   page,
				"data":   hex.EncodeToString(data),
				"method": 0,
			})
			return nil
		}
	}

	// Method 1: Standard UPDATE BINARY command (works on most readers including ACR1252U)
	// APDU: FF D6 00 [page] 04 [4 bytes]
	writeCmd := []byte{0xFF, 0xD6, 0x00, byte(page), 0x04}
	writeCmd = append(writeCmd, data...)
	rsp, err = card.Transmit(writeCmd)
	logging.Debug(logging.CatCard, "Ultralight write method 1", map[string]any{
		"page":     page,
		"cmd":      hex.EncodeToString(writeCmd),
		"response": hex.EncodeToString(rsp),
		"err":      fmt.Sprintf("%v", err),
	})
	if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		logging.Info(logging.CatCard, "Ultralight page written", map[string]any{
			"page":   page,
			"data":   hex.EncodeToString(data),
			"method": 1,
		})
		return nil
	}

	// Method 2: ACR122U InCommunicateThru with native WRITE command (0xA2)
	// Format: FF 00 00 00 08 D4 42 A2 [page] [4 bytes data]
	directCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x08, 0xD4, 0x42, 0xA2, byte(page)}
	directCmd = append(directCmd, data...)
	rsp, err = card.Transmit(directCmd)
	logging.Debug(logging.CatCard, "Ultralight write method 2", map[string]any{
		"page":     page,
		"cmd":      hex.EncodeToString(directCmd),
		"response": hex.EncodeToString(rsp),
		"err":      fmt.Sprintf("%v", err),
	})
	if err == nil && len(rsp) >= 2 {
		sw1, sw2 := rsp[len(rsp)-2], rsp[len(rsp)-1]
		if sw1 == 0x90 && sw2 == 0x00 {
			// Check inner status if present (D5 43 XX format)
			if len(rsp) >= 3 && rsp[0] == 0xD5 && rsp[1] == 0x43 {
				if rsp[2] != 0x00 {
					return fmt.Errorf("write failed for page %d: card error %02X", page, rsp[2])
				}
			}
			logging.Info(logging.CatCard, "Ultralight page written", map[string]any{
				"page":   page,
				"data":   hex.EncodeToString(data),
				"method": 2,
			})
			return nil
		}
	}

	// Method 3: ACR1552 Transparent Exchange with native WRITE command (0xA2)
	// Requires: start session, set protocol, send command, end session
	startSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x81, 0x00}
	setProtocol := []byte{0xFF, 0xC2, 0x00, 0x02, 0x04, 0x8F, 0x02, 0x00, 0x03}
	endSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x82, 0x00}

	// End any stale session first (ignore result)
	card.Transmit(endSession)

	rsp, err = card.Transmit(startSession)
	logging.Debug(logging.CatCard, "Ultralight write method 3 - start session", map[string]any{
		"response": hex.EncodeToString(rsp),
		"err":      fmt.Sprintf("%v", err),
	})
	if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
		// Set protocol to ISO 14443-A Layer 3
		rsp, err = card.Transmit(setProtocol)
		logging.Debug(logging.CatCard, "Ultralight write method 3 - set protocol", map[string]any{
			"response": hex.EncodeToString(rsp),
			"err":      fmt.Sprintf("%v", err),
		})
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
			// Build transparent write command: A2 [page] [4 bytes]
			writeData := []byte{0xA2, byte(page)}
			writeData = append(writeData, data...)
			// Wrap in transparent exchange: FF C2 00 01 [len+2] 95 [len] [data]
			transparentCmd := []byte{0xFF, 0xC2, 0x00, 0x01, byte(len(writeData) + 2), 0x95, byte(len(writeData))}
			transparentCmd = append(transparentCmd, writeData...)

			rsp, err = card.Transmit(transparentCmd)
			logging.Debug(logging.CatCard, "Ultralight write method 3 - write cmd", map[string]any{
				"cmd":      hex.EncodeToString(transparentCmd),
				"response": hex.EncodeToString(rsp),
				"err":      fmt.Sprintf("%v", err),
			})
			card.Transmit(endSession) // Always end session

			// Check for success - response contains status in TLV format
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				logging.Info(logging.CatCard, "Ultralight page written", map[string]any{
					"page":   page,
					"data":   hex.EncodeToString(data),
					"method": 3,
				})
				return nil
			}
		} else {
			card.Transmit(endSession)
		}
	}

	return fmt.Errorf("write failed for page %d: no supported method worked", page)
}

// UltralightPageWrite represents a single page write operation.
type UltralightPageWrite struct {
	Page int    `json:"page"`
	Data []byte `json:"data"`
}

// UltralightWriteResult represents the result of a single page write.
type UltralightWriteResult struct {
	Page    int    `json:"page"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// WriteUltralightPages writes multiple pages to a MIFARE Ultralight / NTAG card
// in a single card session. This is more efficient and reliable than multiple
// individual WriteUltralightPage calls.
func WriteUltralightPages(readerName string, pages []UltralightPageWrite, password []byte) ([]UltralightWriteResult, error) {
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages to write")
	}

	// Validate all pages before connecting
	for _, p := range pages {
		if p.Page < 0 || p.Page > 255 {
			return nil, fmt.Errorf("invalid page number: %d (must be 0-255)", p.Page)
		}
		if p.Page < 4 {
			return nil, fmt.Errorf("cannot write to system pages 0-3 (page %d)", p.Page)
		}
		if len(p.Data) != 4 {
			return nil, fmt.Errorf("page %d: data must be exactly 4 bytes, got %d", p.Page, len(p.Data))
		}
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Authenticate with password if provided (for Ultralight EV1)
	if len(password) > 0 {
		if err := authenticateUltralight(card, password); err != nil {
			return nil, err
		}
	}

	results := make([]UltralightWriteResult, len(pages))

	for i, p := range pages {
		results[i].Page = p.Page

		// Try Method 1: Standard UPDATE BINARY (works on most readers)
		writeCmd := []byte{0xFF, 0xD6, 0x00, byte(p.Page), 0x04}
		writeCmd = append(writeCmd, p.Data...)
		rsp, err := card.Transmit(writeCmd)

		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
			results[i].Success = true
			logging.Info(logging.CatCard, "Ultralight page written (batch)", map[string]any{
				"page": p.Page,
				"data": hex.EncodeToString(p.Data),
			})
			continue
		}

		// Try Method 2: ACR122U InCommunicateThru
		directCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x08, 0xD4, 0x42, 0xA2, byte(p.Page)}
		directCmd = append(directCmd, p.Data...)
		rsp, err = card.Transmit(directCmd)

		if err == nil && len(rsp) >= 2 {
			sw1, sw2 := rsp[len(rsp)-2], rsp[len(rsp)-1]
			if sw1 == 0x90 && sw2 == 0x00 {
				if len(rsp) >= 3 && rsp[0] == 0xD5 && rsp[1] == 0x43 && rsp[2] != 0x00 {
					results[i].Error = fmt.Sprintf("card error %02X", rsp[2])
					continue
				}
				results[i].Success = true
				logging.Info(logging.CatCard, "Ultralight page written (batch)", map[string]any{
					"page": p.Page,
					"data": hex.EncodeToString(p.Data),
				})
				continue
			}
		}

		// Try Method 3: ACR1552 Transparent Exchange with native WRITE command (0xA2)
		startSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x81, 0x00}
		setProtocol := []byte{0xFF, 0xC2, 0x00, 0x02, 0x04, 0x8F, 0x02, 0x00, 0x03}
		endSession := []byte{0xFF, 0xC2, 0x00, 0x00, 0x02, 0x82, 0x00}

		// End any stale session first (ignore result)
		card.Transmit(endSession)

		rsp, err = card.Transmit(startSession)
		if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
			rsp, err = card.Transmit(setProtocol)
			if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
				// Build transparent write command: A2 [page] [4 bytes]
				writeData := []byte{0xA2, byte(p.Page)}
				writeData = append(writeData, p.Data...)
				// Wrap in transparent exchange: FF C2 00 01 [len+2] 95 [len] [data]
				transparentCmd := []byte{0xFF, 0xC2, 0x00, 0x01, byte(len(writeData) + 2), 0x95, byte(len(writeData))}
				transparentCmd = append(transparentCmd, writeData...)

				rsp, err = card.Transmit(transparentCmd)
				card.Transmit(endSession) // Always end session

				if err == nil && len(rsp) >= 2 && rsp[len(rsp)-2] == 0x90 {
					results[i].Success = true
					logging.Info(logging.CatCard, "Ultralight page written (batch)", map[string]any{
						"page":   p.Page,
						"data":   hex.EncodeToString(p.Data),
						"method": 3,
					})
					continue
				}
			} else {
				card.Transmit(endSession)
			}
		}

		// All methods failed
		if err != nil {
			results[i].Error = err.Error()
		} else {
			results[i].Error = fmt.Sprintf("write failed: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
		}
	}

	return results, nil
}

// MifareBlockWrite represents a single block write operation.
type MifareBlockWrite struct {
	Block int    `json:"block"`
	Data  []byte `json:"data"` // Must be 16 bytes
}

// MifareWriteResult represents the result of a single block write.
type MifareWriteResult struct {
	Block   int    `json:"block"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// WriteMifareBlocks writes multiple blocks to a MIFARE Classic card
// in a single card session. This is more efficient and reliable than multiple
// individual WriteMifareBlock calls. Re-authenticates when crossing sectors.
func WriteMifareBlocks(readerName string, blocks []MifareBlockWrite, key []byte, keyType byte) ([]MifareWriteResult, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks to write")
	}

	// Validate all blocks before connecting
	for _, b := range blocks {
		if b.Block < 0 || b.Block > 255 {
			return nil, fmt.Errorf("invalid block number: %d (must be 0-255)", b.Block)
		}
		if isSectorTrailer(b.Block) {
			return nil, fmt.Errorf("cannot write to sector trailer block %d", b.Block)
		}
		if len(b.Data) != 16 {
			return nil, fmt.Errorf("block %d: data must be exactly 16 bytes, got %d", b.Block, len(b.Data))
		}
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Convert key type character to APDU byte
	var keyTypeByte byte = 0x60 // Default Key A
	if keyType == 'B' || keyType == 'b' || keyType == 0x61 {
		keyTypeByte = 0x61
	}

	results := make([]MifareWriteResult, len(blocks))
	lastAuthSector := -1

	for i, b := range blocks {
		results[i].Block = b.Block

		// Calculate sector for this block
		sector := b.Block / 4
		if b.Block >= 128 {
			sector = 32 + (b.Block-128)/16
		}

		// Re-authenticate if sector changed
		if sector != lastAuthSector {
			if err := authenticateMifareBlock(card, b.Block, key, keyTypeByte); err != nil {
				results[i].Error = err.Error()
				continue
			}
			lastAuthSector = sector
		}

		// Write block: FF D6 00 [block] 10 [16 bytes]
		writeCmd := []byte{0xFF, 0xD6, 0x00, byte(b.Block), 0x10}
		writeCmd = append(writeCmd, b.Data...)
		rsp, err := card.Transmit(writeCmd)
		if err != nil {
			results[i].Error = fmt.Sprintf("transmit error: %v", err)
			lastAuthSector = -1 // Force re-auth on next block
			continue
		}
		if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
			results[i].Error = fmt.Sprintf("write failed: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
			lastAuthSector = -1 // Force re-auth on next block
			continue
		}

		results[i].Success = true
		logging.Info(logging.CatCard, "MIFARE block written (batch)", map[string]any{
			"block": b.Block,
			"data":  hex.EncodeToString(b.Data),
		})
	}

	return results, nil
}

// authenticateUltralight performs PWD_AUTH on Ultralight EV1 cards.
// password must be exactly 4 bytes.
func authenticateUltralight(card *scard.Card, password []byte) error {
	if len(password) != 4 {
		return fmt.Errorf("password must be exactly 4 bytes, got %d", len(password))
	}

	// PWD_AUTH command via pseudo-APDU: FF 00 00 00 07 D4 42 1B [4-byte password]
	authCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x07, 0xD4, 0x42, 0x1B}
	authCmd = append(authCmd, password...)
	rsp, err := card.Transmit(authCmd)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Check for successful response (D5 43 00 + PACK bytes + 90 00)
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("authentication failed: wrong password or unsupported card")
	}

	logging.Debug(logging.CatCard, "Ultralight authenticated", nil)
	return nil
}

// aesECBEncrypt performs AES-128-ECB encryption on a single 16-byte block.
// Both key and data must be exactly 16 bytes.
func aesECBEncrypt(key, data []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("AES key must be 16 bytes, got %d", len(key))
	}
	if len(data) != 16 {
		return nil, fmt.Errorf("data must be 16 bytes, got %d", len(data))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	encrypted := make([]byte, 16)
	block.Encrypt(encrypted, data)

	return encrypted, nil
}

// DeriveUIDKeyAES derives a 6-byte MIFARE sector key from the card's UID using AES-128-ECB.
// The algorithm:
//  1. Get 4-byte UID from card
//  2. Expand to 16 bytes by repeating: [uid0,uid1,uid2,uid3,uid0,uid1,uid2,uid3,...]
//  3. AES-128-ECB encrypt with the provided key
//  4. Return first 6 bytes as the derived MIFARE key
//
// aesKey must be exactly 16 bytes (the AES-128 encryption key).
func DeriveUIDKeyAES(readerName string, aesKey []byte) ([]byte, error) {
	if len(aesKey) != 16 {
		return nil, fmt.Errorf("AES key must be 16 bytes, got %d", len(aesKey))
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Get UID using standard command: FF CA 00 00 00
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
	rsp, err := card.Transmit(getUIDCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get UID: %w", err)
	}

	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 || rsp[len(rsp)-1] != 0x00 {
		return nil, fmt.Errorf("get UID failed with status: %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	uid := rsp[:len(rsp)-2]
	if len(uid) != 4 {
		return nil, fmt.Errorf("expected 4-byte UID for key derivation, got %d bytes", len(uid))
	}

	// Expand UID to 16 bytes by repeating
	expandedUID := make([]byte, 16)
	for i := 0; i < 16; i++ {
		expandedUID[i] = uid[i%4]
	}

	// AES-ECB encrypt
	encrypted, err := aesECBEncrypt(aesKey, expandedUID)
	if err != nil {
		return nil, fmt.Errorf("AES encryption failed: %w", err)
	}

	// Return first 6 bytes as the MIFARE key
	derivedKey := encrypted[:6]

	logging.Info(logging.CatCard, "Derived UID key via AES", map[string]any{
		"uid": hex.EncodeToString(uid),
		"key": hex.EncodeToString(derivedKey),
	})

	return derivedKey, nil
}

// AESEncryptAndWriteBlock encrypts 16 bytes with AES-128-ECB and writes to a MIFARE Classic block.
// data: 16 bytes of plaintext data to encrypt and write
// aesKey: 16-byte AES encryption key
// authKey: 6-byte MIFARE sector authentication key
// authKeyType: 'A' or 'B' (defaults to 'A')
func AESEncryptAndWriteBlock(readerName string, block int, data, aesKey, authKey []byte, authKeyType byte) error {
	if len(data) != 16 {
		return fmt.Errorf("data must be exactly 16 bytes, got %d", len(data))
	}
	if len(aesKey) != 16 {
		return fmt.Errorf("AES key must be 16 bytes, got %d", len(aesKey))
	}
	if block < 0 || block > 255 {
		return fmt.Errorf("invalid block number: %d (must be 0-255)", block)
	}
	if isSectorTrailer(block) {
		return fmt.Errorf("cannot write to sector trailer block %d", block)
	}

	// AES-ECB encrypt the data
	encrypted, err := aesECBEncrypt(aesKey, data)
	if err != nil {
		return fmt.Errorf("AES encryption failed: %w", err)
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Convert key type character to APDU byte
	var keyTypeByte byte = 0x60 // Default Key A
	if authKeyType == 'B' || authKeyType == 'b' || authKeyType == 0x61 {
		keyTypeByte = 0x61
	}

	// Authenticate to sector
	if err := authenticateMifareBlock(card, block, authKey, keyTypeByte); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Write encrypted data: FF D6 00 [block] 10 [16 bytes]
	writeCmd := []byte{0xFF, 0xD6, 0x00, byte(block), 0x10}
	writeCmd = append(writeCmd, encrypted...)
	rsp, err := card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write block %d: %w", block, err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("write failed for block %d: status %02X %02X", block, rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	logging.Info(logging.CatCard, "AES encrypted block written", map[string]any{
		"block":     block,
		"plaintext": hex.EncodeToString(data),
		"encrypted": hex.EncodeToString(encrypted),
	})

	return nil
}

// WriteSectorTrailer updates a MIFARE Classic sector trailer with new keys while preserving access bits.
// Unlike WriteMifareBlock, this function is specifically designed for sector trailer writes.
// block: Must be a sector trailer (3, 7, 11, 15, ... for 1K; or 127+16n+15 for 4K large sectors)
// keyA: New 6-byte Key A
// keyB: New 6-byte Key B
// authKey: 6-byte key for authentication to the sector
// authKeyType: 'A' or 'B' (defaults to 'A')
// WriteSectorTrailer writes a MIFARE Classic sector trailer with new keys and optional access bits.
// If accessBits is nil, the existing access bits are preserved.
// If accessBits is 3 bytes, a 0x00 user data byte is appended.
// If accessBits is 4 bytes, it's used as-is.
func WriteSectorTrailer(readerName string, block int, keyA, keyB, accessBits, authKey []byte, authKeyType byte) error {
	if !isSectorTrailer(block) {
		return fmt.Errorf("block %d is not a sector trailer", block)
	}
	if len(keyA) != 6 {
		return fmt.Errorf("keyA must be 6 bytes, got %d", len(keyA))
	}
	if len(keyB) != 6 {
		return fmt.Errorf("keyB must be 6 bytes, got %d", len(keyB))
	}
	if accessBits != nil && len(accessBits) != 3 && len(accessBits) != 4 {
		return fmt.Errorf("accessBits must be 3 or 4 bytes, got %d", len(accessBits))
	}

	ctx, err := scard.EstablishContext()
	if err != nil {
		return fmt.Errorf("failed to establish context: %w", err)
	}
	defer ctx.Release()

	card, err := ctx.Connect(readerName, scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return fmt.Errorf("failed to connect to reader: %w", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	// Convert auth key type character to APDU byte
	var keyTypeByte byte = 0x60 // Default Key A
	if authKeyType == 'B' || authKeyType == 'b' || authKeyType == 0x61 {
		keyTypeByte = 0x61
	}

	// Authenticate with provided key
	if err := authenticateMifareBlock(card, block, authKey, keyTypeByte); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Determine access bits to use
	var finalAccessBits []byte
	if accessBits != nil {
		// Use provided access bits
		if len(accessBits) == 3 {
			// Append 0x00 as user data byte
			finalAccessBits = append(accessBits, 0x00)
		} else {
			finalAccessBits = accessBits
		}
	} else {
		// Read current sector trailer to preserve existing access bits
		readCmd := []byte{0xFF, 0xB0, 0x00, byte(block), 0x10}
		rsp, err := card.Transmit(readCmd)
		if err != nil {
			return fmt.Errorf("failed to read sector trailer: %w", err)
		}
		if len(rsp) < 18 || rsp[len(rsp)-2] != 0x90 {
			return fmt.Errorf("read failed for sector trailer: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
		}
		// Extract access bits (bytes 6-9 of the 16-byte sector trailer)
		// Sector trailer format: [KeyA (6)] [Access bits (4)] [KeyB (6)]
		finalAccessBits = rsp[6:10]
	}

	// Build new sector trailer: [newKeyA (6)] + [accessBits (4)] + [newKeyB (6)]
	newTrailer := make([]byte, 16)
	copy(newTrailer[0:6], keyA)
	copy(newTrailer[6:10], finalAccessBits)
	copy(newTrailer[10:16], keyB)

	// Write new sector trailer
	writeCmd := []byte{0xFF, 0xD6, 0x00, byte(block), 0x10}
	writeCmd = append(writeCmd, newTrailer...)
	rsp, err := card.Transmit(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write sector trailer: %w", err)
	}
	if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 {
		return fmt.Errorf("write failed for sector trailer: status %02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1])
	}

	logging.Info(logging.CatCard, "Sector trailer updated", map[string]any{
		"block":      block,
		"keyA":       hex.EncodeToString(keyA),
		"keyB":       hex.EncodeToString(keyB),
		"accessBits": hex.EncodeToString(finalAccessBits),
	})

	return nil
}

// =============================================================================
// Proxmark3 Implementation Functions
// =============================================================================

// Singleton Proxmark3 client (shared across all calls to prevent concurrent access)
var (
	pm3Client     proxmark3.PM3Executor
	pm3ClientOnce sync.Once
	pm3Config     *config.Proxmark3Config
)

// getProxmark3Client returns the singleton Proxmark3 client using environment config.
func getProxmark3Client() proxmark3.PM3Executor {
	pm3ClientOnce.Do(func() {
		// Load config if not already set
		if pm3Config == nil {
			cfg := config.Load()
			pm3Config = &cfg.Proxmark3
		}

		pm3Path := pm3Config.Path
		if pm3Path == "" {
			var err error
			pm3Path, err = proxmark3.DetectPM3()
			if err != nil {
				logging.Warn(logging.CatReader, "Failed to detect pm3 binary", map[string]any{
					"error": err.Error(),
				})
			}
		}
		logging.Info(logging.CatReader, "Initializing Proxmark3 client", map[string]any{
			"pm3Path":        pm3Path,
			"port":           pm3Config.Port,
			"persistentMode": pm3Config.PersistentMode,
		})

		// Create persistent or single-shot client based on config
		if pm3Config.PersistentMode {
			pm3Client = proxmark3.NewPersistentClient(proxmark3.PersistentConfig{
				PM3Path:     pm3Path,
				Port:        pm3Config.Port,
				IdleTimeout: pm3Config.IdleTimeout,
			})
		} else {
			pm3Client = proxmark3.NewClient(proxmark3.Config{
				PM3Path: pm3Path,
				Port:    pm3Config.Port,
			})
		}
	})
	return pm3Client
}

// InitProxmark3Eager starts the Proxmark3 subprocess eagerly at startup.
// This ensures the first card read is fast instead of waiting for subprocess startup.
func InitProxmark3Eager(cfg *config.Config) {
	if !cfg.Proxmark3.Enabled {
		return
	}

	// Store config for getProxmark3Client
	pm3Config = &cfg.Proxmark3

	go func() {
		client := getProxmark3Client()
		if client == nil {
			return
		}

		// For persistent client, start the subprocess eagerly
		if pc, ok := client.(*proxmark3.PersistentClient); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			if err := pc.Start(ctx); err != nil {
				logging.Warn(logging.CatReader, "Failed to start Proxmark3 subprocess eagerly", map[string]any{
					"error": err.Error(),
				})
			} else {
				logging.Info(logging.CatReader, "Proxmark3 subprocess started eagerly for fast first read", nil)
			}
		} else {
			// For regular client, just verify connectivity
			if client.IsConnected() {
				logging.Info(logging.CatReader, "Proxmark3 device connected", nil)
			}
		}
	}()
}

// ShutdownProxmark3 gracefully shuts down the Proxmark3 subprocess.
func ShutdownProxmark3() {
	if pm3Client == nil {
		return
	}

	if pc, ok := pm3Client.(*proxmark3.PersistentClient); ok {
		pc.Stop()
	}
}

// getCardUIDProxmark3 reads card UID using Proxmark3.
func getCardUIDProxmark3(readerName string) (*Card, error) {
	client := getProxmark3Client()
	ctx := context.Background()

	info, err := client.GetCardInfo(ctx)
	if err != nil {
		if proxmark3.IsNoCardError(err) {
			return nil, fmt.Errorf("no card present")
		}
		return nil, fmt.Errorf("failed to get card info: %w", err)
	}

	cardInfo := &Card{
		UID:      hex.EncodeToString(info.UID),
		Type:     info.CardType,
		Writable: true, // Assume writable for now
	}

	// Set protocol based on card type
	if info.CardType == "MIFARE Classic 1K" || info.CardType == "MIFARE Classic 4K" {
		cardInfo.Protocol = "NFC-A"
		cardInfo.ProtocolISO = "ISO 14443-3A"
		if info.CardType == "MIFARE Classic 1K" {
			cardInfo.Size = 1024
		} else {
			cardInfo.Size = 4096
		}
	} else if info.CardType == "NTAG213" {
		cardInfo.Protocol = "NFC-A"
		cardInfo.ProtocolISO = "ISO 14443-3A"
		cardInfo.Size = 180
	} else if info.CardType == "NTAG215" {
		cardInfo.Protocol = "NFC-A"
		cardInfo.ProtocolISO = "ISO 14443-3A"
		cardInfo.Size = 504
	} else if info.CardType == "NTAG216" {
		cardInfo.Protocol = "NFC-A"
		cardInfo.ProtocolISO = "ISO 14443-3A"
		cardInfo.Size = 888
	} else if info.CardType == "MIFARE Ultralight" || info.CardType == "MIFARE Ultralight EV1" {
		cardInfo.Protocol = "NFC-A"
		cardInfo.ProtocolISO = "ISO 14443-3A"
		cardInfo.Size = 64
	}

	// Try to read NDEF data for Ultralight/NTAG cards
	if cardInfo.Type == "NTAG213" || cardInfo.Type == "NTAG215" || cardInfo.Type == "NTAG216" ||
		cardInfo.Type == "MIFARE Ultralight" || cardInfo.Type == "MIFARE Ultralight EV1" {
		if ndefData, err := client.ReadNDEF(ctx); err == nil && len(ndefData) > 0 {
			cardInfo.Data = hex.EncodeToString(ndefData)
			cardInfo.DataType = "binary"
		}
	}

	return cardInfo, nil
}

// readMifareBlockProxmark3 reads a MIFARE Classic block using Proxmark3.
func readMifareBlockProxmark3(readerName string, block int, key []byte, keyType byte) ([]byte, error) {
	client := getProxmark3Client()
	ctx := context.Background()

	return client.ReadMifareBlock(ctx, block, key, keyType)
}

// writeMifareBlockProxmark3 writes a MIFARE Classic block using Proxmark3.
func writeMifareBlockProxmark3(readerName string, block int, data []byte, key []byte, keyType byte) error {
	client := getProxmark3Client()
	ctx := context.Background()

	return client.WriteMifareBlock(ctx, block, data, key, keyType)
}

// readUltralightPageProxmark3 reads a MIFARE Ultralight/NTAG page using Proxmark3.
func readUltralightPageProxmark3(readerName string, page int, password []byte) ([]byte, error) {
	client := getProxmark3Client()
	ctx := context.Background()

	return client.ReadUltralightPage(ctx, page, password)
}

// writeUltralightPageProxmark3 writes a MIFARE Ultralight/NTAG page using Proxmark3.
func writeUltralightPageProxmark3(readerName string, page int, data []byte, password []byte) error {
	client := getProxmark3Client()
	ctx := context.Background()

	return client.WriteUltralightPage(ctx, page, data, password)
}

// writeMultipleRecordsProxmark3 writes multiple NDEF records to an ISO 15693 tag using Proxmark3.
func writeMultipleRecordsProxmark3(readerName string, records []NDEFRecord) error {
	logging.Info(logging.CatReader, "writeMultipleRecordsProxmark3 called", map[string]any{
		"readerName":  readerName,
		"recordCount": len(records),
	})

	client := getProxmark3Client()
	ctx := context.Background()

	// Build multi-record NDEF message (same logic as WriteMultipleRecords)
	var ndefRecords []byte
	for i, rec := range records {
		isFirst := i == 0
		isLast := i == len(records)-1

		var recordBytes []byte
		switch rec.Type {
		case "url":
			prefixCode, remainder := findURIPrefix(rec.Data)
			payload := []byte{prefixCode}
			payload = append(payload, []byte(remainder)...)
			recordBytes = createNDEFRecordRaw(0x01, []byte("U"), payload, isFirst, isLast)
		case "text":
			payload := []byte{0x02}
			payload = append(payload, []byte("en")...)
			payload = append(payload, []byte(rec.Data)...)
			recordBytes = createNDEFRecordRaw(0x01, []byte("T"), payload, isFirst, isLast)
		case "json":
			recordBytes = createNDEFRecordRaw(0x02, []byte("application/json"), []byte(rec.Data), isFirst, isLast)
		case "binary":
			decoded, err := hex.DecodeString(rec.Data)
			if err != nil {
				return fmt.Errorf("invalid binary data in record %d: %w", i, err)
			}
			recordBytes = createNDEFRecordRaw(0x02, []byte("application/octet-stream"), decoded, isFirst, isLast)
		case "mime":
			if rec.MimeType == "" {
				return fmt.Errorf("mimeType required for mime record type in record %d", i)
			}
			var payload []byte
			if rec.DataType == "binary" {
				decoded, err := base64.StdEncoding.DecodeString(rec.Data)
				if err != nil {
					return fmt.Errorf("invalid base64 data in mime record %d: %w", i, err)
				}
				payload = decoded
			} else {
				payload = []byte(rec.Data)
			}
			recordBytes = createNDEFRecordRaw(0x02, []byte(rec.MimeType), payload, isFirst, isLast)
		default:
			return fmt.Errorf("unsupported record type: %s", rec.Type)
		}
		ndefRecords = append(ndefRecords, recordBytes...)
	}

	// Wrap in TLV format
	tlv := []byte{0x03} // NDEF TLV type
	if len(ndefRecords) < 255 {
		tlv = append(tlv, byte(len(ndefRecords)))
	} else {
		tlv = append(tlv, 0xFF)
		tlv = append(tlv, byte(len(ndefRecords)>>8))
		tlv = append(tlv, byte(len(ndefRecords)))
	}
	tlv = append(tlv, ndefRecords...)
	tlv = append(tlv, 0xFE) // Terminator TLV

	// ISO 15693 (Type 5) tags: CC at block 0, NDEF at block 1
	// Calculate size for CC (available memory / 8)
	totalSize := len(tlv) + 4 // +4 for CC block
	ccSize := byte((totalSize + 7) / 8)
	if ccSize < 0x10 {
		ccSize = 0x10 // Minimum reasonable size
	}

	// CC format: E1 40 <size/8> 05
	// - 0xE1: Magic number (NFC Forum Type 5)
	// - 0x40: Version 1.0 (4), read/write access (0)
	// - Size: Available memory / 8
	// - 0x05: Multiple block read supported, no special frame format
	cc := []byte{0xE1, 0x40, ccSize, 0x05}

	// Write CC at block 0
	logging.Info(logging.CatReader, "Writing CC block", map[string]any{
		"block": 0,
		"data":  hex.EncodeToString(cc),
	})
	if err := client.WriteISO15693Block(ctx, 0, cc); err != nil {
		logging.Error(logging.CatReader, "Failed to write CC", map[string]any{"error": err.Error()})
		return fmt.Errorf("failed to write CC: %w", err)
	}
	logging.Info(logging.CatReader, "CC block written successfully", nil)

	// Write NDEF data starting at block 1
	// ISO 15693 blocks are 4 bytes each
	block := 1
	totalBlocks := (len(tlv) + 3) / 4
	logging.Info(logging.CatReader, "Writing NDEF data", map[string]any{
		"tlvLength":   len(tlv),
		"totalBlocks": totalBlocks,
	})

	for i := 0; i < len(tlv); i += 4 {
		blockData := make([]byte, 4)
		end := i + 4
		if end > len(tlv) {
			end = len(tlv)
		}
		copy(blockData, tlv[i:end])

		logging.Debug(logging.CatReader, "Writing block", map[string]any{
			"block": block,
			"data":  hex.EncodeToString(blockData),
		})
		if err := client.WriteISO15693Block(ctx, block, blockData); err != nil {
			logging.Error(logging.CatReader, "Failed to write block", map[string]any{
				"block": block,
				"error": err.Error(),
			})
			return fmt.Errorf("failed to write block %d: %w", block, err)
		}
		block++
	}

	logging.Info(logging.CatReader, "All blocks written successfully", map[string]any{"totalBlocks": block - 1})
	return nil
}
