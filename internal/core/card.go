package core

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/ebfe/scard"
)

// Card represents an NFC card/tag.
type Card struct {
	UID      string `json:"uid"`
	ATR      string `json:"atr,omitempty"`
	Type     string `json:"type,omitempty"`     // e.g., "NTAG213", "NTAG215", "NTAG216", "MIFARE Classic"
	Size     int    `json:"size,omitempty"`     // Memory size in bytes
	Writable bool   `json:"writable,omitempty"` // Whether the tag is writable
	URL      string `json:"url,omitempty"`      // URL from first NDEF record (if URI record)
	Data     string `json:"data,omitempty"`     // NDEF data read from the tag (if available)
	DataType string `json:"dataType,omitempty"` // Type of data: "text", "json", "binary", or "unknown"
}

// GetCardUID connects to the specified reader and attempts to read the card UID.
// Returns an error if no card is present or if reading fails.
func GetCardUID(readerName string) (*Card, error) {
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

	cardInfo := &Card{
		UID: hex.EncodeToString(uid),
		ATR: hex.EncodeToString(status.Atr),
	}

	// Detect card type by reading version info (for NTAG cards)
	detectCardType(card, cardInfo)

	// Try to read NDEF data from the card
	readNDEFData(card, cardInfo)

	return cardInfo, nil
}

// detectCardType attempts to determine the card type (NTAG213/215/216, MIFARE, etc.)
func detectCardType(card *scard.Card, cardInfo *Card) {
	// Method 1a: Try GET_VERSION (works on ACR1552U)
	getVersionCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00}
	rsp, err := card.Transmit(getVersionCmd)

	if err == nil && len(rsp) >= 10 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		// Parse version response (8 bytes + status)
		storageSize := rsp[6]

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
	}

	// Method 1b: Try alternative GET_VERSION format (works on ACR1252U)
	// ACR1252U needs a delay after a failed command before it will accept another
	method1aFailed := err != nil || len(rsp) < 10 || rsp[len(rsp)-2] != 0x90
	if method1aFailed {
		time.Sleep(150 * time.Millisecond)
	}
	getVersionCmd2 := []byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0x60}
	rsp, err = card.Transmit(getVersionCmd2)

	if err == nil && len(rsp) >= 10 && rsp[len(rsp)-2] == 0x90 && rsp[len(rsp)-1] == 0x00 {
		// Parse version response (8 bytes + status)
		storageSize := rsp[6]

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
	}

	// Method 2a: Try reading pages 1-4 (works on ACR1252U where direct page 3 read fails)
	// Page 3 contains the capability container at offset 8 in this 16-byte response
	readCmd1 := []byte{0xFF, 0xB0, 0x00, 0x01, 0x10} // Read 16 bytes from page 1
	rsp, err = card.Transmit(readCmd1)

	if err == nil && len(rsp) >= 12 && rsp[len(rsp)-2] == 0x90 {
		// CC is at bytes 8-11 (page 3 within the 4-page read)
		// Byte 10 (index 10 in response) indicates memory size
		if len(rsp) >= 11 {
			ccSize := rsp[10]

			switch ccSize {
			case 0x12: // 144 bytes -> NTAG213
				cardInfo.Type = "NTAG213"
				cardInfo.Size = 180
				cardInfo.Writable = true
				return
			case 0x3E: // 496 bytes -> NTAG215
				cardInfo.Type = "NTAG215"
				cardInfo.Size = 504
				cardInfo.Writable = true
				return
			case 0x6D: // 872 bytes -> NTAG216
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
		// Byte 2 of CC indicates memory size
		if len(rsp) >= 3 {
			ccSize := rsp[2]

			switch ccSize {
			case 0x12: // 144 bytes -> NTAG213
				cardInfo.Type = "NTAG213"
				cardInfo.Size = 180
				cardInfo.Writable = true
				return
			case 0x3E: // 496 bytes -> NTAG215
				cardInfo.Type = "NTAG215"
				cardInfo.Size = 504
				cardInfo.Writable = true
				return
			case 0x6D: // 872 bytes -> NTAG216
				cardInfo.Type = "NTAG216"
				cardInfo.Size = 888
				cardInfo.Writable = true
				return
			}
		}
	}

	// Method 3: Check ATR patterns for NTAG, MIFARE, and ISO 15693
	atr := cardInfo.ATR
	if len(atr) >= 30 && (atr[0:4] == "3b8f" || atr[0:4] == "3b8b") {
		// Check for ISO 15693 cards (ICode SLI, ICode Slix, ICode Slix 2)
		// Pattern: 03 06 0b at bytes 10-12 (hex string position 20-25)
		if contains(atr, "03060b") {
			// ISO 15693 family (ICode SLI/Slix/Slix-2)
			cardInfo.Type = "ISO 15693"
			cardInfo.Writable = true
			cardInfo.Size = 1024 // ICode Slix 2 has variable sizes, default to 1KB
			return
		}

		// Both NTAG and MIFARE can have ATRs starting with 3b8f and containing 03060300
		// The key difference is at byte 14 (hex string position 28-29):
		// - MIFARE Classic: 01
		// - NTAG: 03

		if contains(atr, "03060300") {
			// Check byte 14 to distinguish NTAG from MIFARE
			if atr[28:30] == "01" {
				// MIFARE Classic
				cardInfo.Type = "MIFARE Classic"
				cardInfo.Writable = true
				cardInfo.Size = 1024
				return
			} else if atr[28:30] == "03" {
				// NTAG family
				cardInfo.Type = "NTAG (type unknown)"
				cardInfo.Writable = true
				cardInfo.Size = 180
				return
			}
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
		default:
			return fmt.Errorf("unsupported data type: %s (use 'json', 'text', 'binary', or 'url')", dataType)
		}
	}

	// Write NDEF message to NTAG card
	if err := writeNTAGPages(card, 4, ndefMessage); err != nil {
		return fmt.Errorf("failed to write NDEF message: %w", err)
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

		// APDU command to write 4 bytes to a page: FF D6 00 [page] 04 [4 bytes]
		writeCmd := []byte{0xFF, 0xD6, 0x00, byte(pageNum), 0x04}
		writeCmd = append(writeCmd, pageData...)

		rsp, err := card.Transmit(writeCmd)
		if err != nil {
			return fmt.Errorf("failed to write page %d: %w", pageNum, err)
		}

		// Check response
		if len(rsp) < 2 || rsp[len(rsp)-2] != 0x90 || rsp[len(rsp)-1] != 0x00 {
			return fmt.Errorf("write failed at page %d with status: %02X %02X", pageNum, rsp[len(rsp)-2], rsp[len(rsp)-1])
		}
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

// readNDEFData attempts to read NDEF data from an NTAG card
func readNDEFData(card *scard.Card, cardInfo *Card) {
	logging.Debug(logging.CatCard, "Reading NDEF data", map[string]any{
		"cardType": cardInfo.Type,
	})
	// Read pages 4-43 (NTAG213 has 45 pages total, pages 0-3 are reserved)
	// Read 16 pages (64 bytes) at a time to avoid buffer issues
	var allData []byte
	maxPages := 40
	if cardInfo.Type == "NTAG215" {
		maxPages = 126
	} else if cardInfo.Type == "NTAG216" {
		maxPages = 222
	}

	// Read in chunks of 16 pages (64 bytes)
	for startPage := 4; startPage < 4+maxPages; startPage += 16 {
		pagesToRead := 16
		if startPage+pagesToRead > 4+maxPages {
			pagesToRead = 4 + maxPages - startPage
		}

		readCmd := []byte{0xFF, 0xB0, 0x00, byte(startPage), byte(pagesToRead * 4)}
		rsp, err := card.Transmit(readCmd)

		if err != nil || len(rsp) < 3 {
			logging.Debug(logging.CatCard, "NDEF read failed", map[string]any{
				"page":  startPage,
				"error": fmt.Sprintf("%v", err),
			})
			break // Stop on error
		}

		// Check status
		if rsp[len(rsp)-2] != 0x90 || rsp[len(rsp)-1] != 0x00 {
			logging.Debug(logging.CatCard, "NDEF read bad status", map[string]any{
				"page":   startPage,
				"status": fmt.Sprintf("%02X %02X", rsp[len(rsp)-2], rsp[len(rsp)-1]),
			})
			break
		}

		// Append data (without status bytes)
		allData = append(allData, rsp[:len(rsp)-2]...)

		// If we got less data than expected, we've reached the end
		if len(rsp)-2 < pagesToRead*4 {
			break
		}

		// Check if we've found the terminator TLV
		if len(allData) > 2 && allData[0] == 0x03 {
			var ndefLength int
			if allData[1] == 0xFF && len(allData) >= 4 {
				ndefLength = int(allData[2])<<8 | int(allData[3])
				if len(allData) >= 4+ndefLength+1 {
					break // We have all the data
				}
			} else if allData[1] != 0xFF {
				ndefLength = int(allData[1])
				if len(allData) >= 2+ndefLength+1 {
					break // We have all the data
				}
			}
		}
	}

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
	Type string `json:"type"` // "url", "text", "json", "binary"
	Data string `json:"data"` // Data content (base64 for binary)
}

func WriteMultipleRecords(readerName string, records []NDEFRecord) error {
	if len(records) == 0 {
		return fmt.Errorf("no records to write")
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
			// Decode base64
			decoded, err := hex.DecodeString(rec.Data)
			if err != nil {
				return fmt.Errorf("invalid binary data in record %d: %w", i, err)
			}
			recordBytes = createNDEFRecordRaw(0x02, []byte("application/octet-stream"), decoded, isFirst, isLast)
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

	if err := writeNTAGPages(card, 4, tlv); err != nil {
		return fmt.Errorf("failed to write NDEF records: %w", err)
	}

	return nil
}
