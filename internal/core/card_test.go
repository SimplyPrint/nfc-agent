package core

import (
	"testing"
)

// Mock data from real NFC tags read from hardware:
// Reader 0 (ACR122U): MIFARE Classic - UID: 932bae0e, ATR: 3b8f8001804f0ca000000306030001000000006a
// Reader 1 (ACR1552): ISO 15693 (SLIX) - UID: 80391566080104e0, ATR: 3b8f8001804f0ca0000003060b00140000000077
// Reader 2 (ACR1252): NTAG213 - UID: 0442488a837280, ATR: 3b8f8001804f0ca0000003060300030000000068

// MockCardData represents mock NFC card data for testing
type MockCardData struct {
	ReaderName string
	UID        string
	ATR        string
	Type       string
	Size       int
	Writable   bool
	Data       string
	DataType   string
}

// Real mock data from actual hardware readings
var mockCards = []MockCardData{
	{
		ReaderName: "ACS ACR122U PICC Interface",
		UID:        "932bae0e",
		ATR:        "3b8f8001804f0ca000000306030001000000006a",
		Type:       "MIFARE Classic",
		Size:       1024,
		Writable:   true,
	},
	{
		ReaderName: "ACS ACR1552 1S CL Reader PICC",
		UID:        "80391566080104e0",
		ATR:        "3b8f8001804f0ca0000003060b00140000000077",
		Type:       "ISO 15693",
		Size:       1024,
		Writable:   true,
		Data:       "eyoooo",
		DataType:   "text",
	},
	{
		ReaderName: "ACS ACR1252 Dual Reader PICC",
		UID:        "0442488a837280",
		ATR:        "3b8f8001804f0ca0000003060300030000000068",
		Type:       "NTAG213",
		Size:       180,
		Writable:   true,
	},
}

func TestFindURIPrefix(t *testing.T) {
	// Note: The prefix matching order in findURIPrefix checks https:// before https://www.
	// So https://www.example.com matches https:// (code 0x04) with remainder "www.example.com"
	tests := []struct {
		uri          string
		expectedCode byte
		expectedRest string
	}{
		{"https://example.com", 0x04, "example.com"},
		{"http://example.com", 0x03, "example.com"},
		{"https://www.example.com", 0x04, "www.example.com"}, // matches https:// first
		{"http://www.example.com", 0x03, "www.example.com"},  // matches http:// first
		{"tel:+1234567890", 0x05, "+1234567890"},
		{"mailto:test@example.com", 0x06, "test@example.com"},
		{"custom://something", 0x00, "custom://something"},
		{"ftp://files.example.com", 0x00, "ftp://files.example.com"},
		{"", 0x00, ""},
		// Edge cases
		{"https://", 0x04, ""},
		{"http://", 0x03, ""},
		{"tel:", 0x05, ""},
		{"mailto:", 0x06, ""},
		{"HTTPS://EXAMPLE.COM", 0x00, "HTTPS://EXAMPLE.COM"}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			code, rest := findURIPrefix(tt.uri)
			if code != tt.expectedCode {
				t.Errorf("findURIPrefix(%q) code = 0x%02X, want 0x%02X", tt.uri, code, tt.expectedCode)
			}
			if rest != tt.expectedRest {
				t.Errorf("findURIPrefix(%q) rest = %q, want %q", tt.uri, rest, tt.expectedRest)
			}
		})
	}
}

func TestGetURIPrefix(t *testing.T) {
	tests := []struct {
		code     byte
		expected string
	}{
		{0x00, ""},
		{0x01, "http://www."},
		{0x02, "https://www."},
		{0x03, "http://"},
		{0x04, "https://"},
		{0x05, "tel:"},
		{0x06, "mailto:"},
		{0x07, "ftp://anonymous:anonymous@"},
		{0x08, "ftp://ftp."},
		{0x09, "ftps://"},
		{0x0A, "sftp://"},
		{0x0B, "smb://"},
		{0x0C, "nfs://"},
		{0x0D, "ftp://"},
		{0x0E, "dav://"},
		{0x0F, "news:"},
		{0x10, "telnet://"},
		{0x11, "imap:"},
		{0x12, "rtsp://"},
		{0x13, "urn:"},
		{0x14, "pop:"},
		{0x15, "sip:"},
		{0x16, "sips:"},
		{0x17, "tftp:"},
		{0x18, "btspp://"},
		{0x19, "btl2cap://"},
		{0x1A, "btgoep://"},
		{0x1B, "tcpobex://"},
		{0x1C, "irdaobex://"},
		{0x1D, "file://"},
		{0x1E, "urn:epc:id:"},
		{0x1F, "urn:epc:tag:"},
		{0x20, "urn:epc:pat:"},
		{0x21, "urn:epc:raw:"},
		{0x22, "urn:epc:"},
		{0x23, "urn:nfc:"},
		{0xFF, ""}, // Unknown code
		{0x24, ""}, // Out of defined range
		{0x50, ""}, // Out of defined range
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			result := getURIPrefix(tt.code)
			if result != tt.expected {
				t.Errorf("getURIPrefix(0x%02X) = %q, want %q", tt.code, result, tt.expected)
			}
		})
	}
}

func TestCreateNDEFTextRecord(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"simple text", "Hello, World!"},
		{"empty text", ""},
		{"unicode text", "Hello, 世界!"},
		{"long text", "This is a longer text that spans multiple bytes and tests the record creation properly."},
		{"special chars", "!@#$%^&*()_+-=[]{}|;':\",./<>?"},
		{"newlines", "Line1\nLine2\nLine3"},
		{"data from real tag", mockCards[1].Data}, // "eyoooo" from ISO 15693 tag
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := createNDEFTextRecord(tt.text)

			// Verify TLV format
			if record[0] != 0x03 {
				t.Errorf("expected NDEF TLV type 0x03, got 0x%02X", record[0])
			}

			// Verify terminator
			if record[len(record)-1] != 0xFE {
				t.Errorf("expected terminator 0xFE, got 0x%02X", record[len(record)-1])
			}

			// Verify it's a valid length (TLV + header + type + payload + terminator)
			if len(record) < 10 {
				t.Errorf("record too short: %d bytes", len(record))
			}
		})
	}
}

func TestCreateNDEFURIRecord(t *testing.T) {
	tests := []struct {
		uri string
	}{
		{"https://example.com"},
		{"http://www.google.com"},
		{"tel:+1234567890"},
		{"mailto:test@test.com"},
		{"https://simplyprint.io"},
		{"ftp://files.example.com"},
		{"custom://app/path"},
		{""},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			record := createNDEFURIRecord(tt.uri)

			// Verify TLV format
			if record[0] != 0x03 {
				t.Errorf("expected NDEF TLV type 0x03, got 0x%02X", record[0])
			}

			// Verify terminator
			if record[len(record)-1] != 0xFE {
				t.Errorf("expected terminator 0xFE, got 0x%02X", record[len(record)-1])
			}

			// Record should be non-empty
			if len(record) < 5 {
				t.Errorf("record too short: %d bytes", len(record))
			}
		})
	}
}

func TestCreateNDEFMimeRecord(t *testing.T) {
	tests := []struct {
		mimeType string
		data     []byte
	}{
		{"application/json", []byte(`{"key":"value"}`)},
		{"application/octet-stream", []byte{0x01, 0x02, 0x03, 0x04}},
		{"text/plain", []byte("Hello")},
		{"application/xml", []byte("<root><item>test</item></root>")},
		{"image/png", []byte{0x89, 0x50, 0x4E, 0x47}}, // PNG magic bytes
		{"application/json", []byte(`{"uid":"932bae0e","type":"MIFARE Classic"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			record := createNDEFMimeRecord(tt.mimeType, tt.data)

			// Verify TLV format
			if record[0] != 0x03 {
				t.Errorf("expected NDEF TLV type 0x03, got 0x%02X", record[0])
			}

			// Verify terminator
			if record[len(record)-1] != 0xFE {
				t.Errorf("expected terminator 0xFE, got 0x%02X", record[len(record)-1])
			}
		})
	}
}

func TestCreateMultiRecordNDEF(t *testing.T) {
	url := "https://example.com"
	data := []byte("test data")

	tests := []struct {
		dataType string
	}{
		{"text"},
		{"json"},
		{"binary"},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			record := createMultiRecordNDEF(url, data, tt.dataType)

			// Verify TLV format
			if record[0] != 0x03 {
				t.Errorf("expected NDEF TLV type 0x03, got 0x%02X", record[0])
			}

			// Verify terminator
			if record[len(record)-1] != 0xFE {
				t.Errorf("expected terminator 0xFE, got 0x%02X", record[len(record)-1])
			}

			// Multi-record should be longer than single record
			if len(record) < 20 {
				t.Errorf("multi-record too short: %d bytes", len(record))
			}
		})
	}
}

func TestCreateMultiRecordNDEF_WithMockData(t *testing.T) {
	// Test using real card UIDs as data
	for _, card := range mockCards {
		t.Run(card.Type, func(t *testing.T) {
			url := "https://simplyprint.io/nfc/" + card.UID
			data := []byte(`{"uid":"` + card.UID + `","type":"` + card.Type + `"}`)

			record := createMultiRecordNDEF(url, data, "json")

			if record[0] != 0x03 {
				t.Errorf("expected NDEF TLV type 0x03, got 0x%02X", record[0])
			}

			if record[len(record)-1] != 0xFE {
				t.Errorf("expected terminator 0xFE, got 0x%02X", record[len(record)-1])
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "xyz", false},
		{"", "test", false},
		{"test", "", true},
		{"abc", "abcd", false},
		// Test with real ATR patterns
		{mockCards[0].ATR, "03060300", true},  // MIFARE Classic ATR contains this pattern
		{mockCards[1].ATR, "03060b", true},    // ISO 15693 ATR contains this pattern
		{mockCards[2].ATR, "03060300", true},  // NTAG213 ATR contains this pattern
		{mockCards[0].ATR, "03060b", false},   // MIFARE Classic doesn't have ISO 15693 pattern
		{"3b8f8001804f0ca000000306030001000000006a", "01", true},  // byte 14 = 01 for MIFARE
		{"3b8f8001804f0ca0000003060300030000000068", "03", true},  // byte 14 = 03 for NTAG
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestFindIndex(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected int
	}{
		{"hello world", "world", 6},
		{"hello world", "hello", 0},
		{"hello world", "xyz", -1},
		{"abcabc", "bc", 1},
		{"", "test", -1},
		// Test with real ATR patterns
		{mockCards[0].ATR, "03060300", 20}, // Pattern position in MIFARE ATR
		{mockCards[1].ATR, "03060b", 20},   // Pattern position in ISO 15693 ATR
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := findIndex(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("findIndex(%q, %q) = %d, want %d", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestCreateNDEFRecordRaw(t *testing.T) {
	// Test with MB=true, ME=true (single record message)
	record := createNDEFRecordRaw(0x01, []byte("T"), []byte("test"), true, true)

	// Check header byte: MB(0x80) + ME(0x40) + SR(0x10) + TNF(0x01) = 0xD1
	if record[0] != 0xD1 {
		t.Errorf("expected header 0xD1, got 0x%02X", record[0])
	}

	// Test with MB=true, ME=false (first record in multi-record)
	record = createNDEFRecordRaw(0x01, []byte("U"), []byte("test"), true, false)

	// Check header byte: MB(0x80) + SR(0x10) + TNF(0x01) = 0x91
	if record[0] != 0x91 {
		t.Errorf("expected header 0x91, got 0x%02X", record[0])
	}

	// Test with MB=false, ME=true (last record in multi-record)
	record = createNDEFRecordRaw(0x02, []byte("application/json"), []byte("{}"), false, true)

	// Check header byte: ME(0x40) + SR(0x10) + TNF(0x02) = 0x52
	if record[0] != 0x52 {
		t.Errorf("expected header 0x52, got 0x%02X", record[0])
	}

	// Test with MB=false, ME=false (middle record)
	record = createNDEFRecordRaw(0x01, []byte("T"), []byte("middle"), false, false)

	// Check header byte: SR(0x10) + TNF(0x01) = 0x11
	if record[0] != 0x11 {
		t.Errorf("expected header 0x11, got 0x%02X", record[0])
	}
}

func TestNDEFRecord_TypeLengthPayloadLength(t *testing.T) {
	typeBytes := []byte("U")
	payload := []byte{0x04, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'}

	record := createNDEFRecordRaw(0x01, typeBytes, payload, true, true)

	// Record format: header(1) + type_length(1) + payload_length(1 for SR) + type + payload
	// record[1] should be type length
	if record[1] != byte(len(typeBytes)) {
		t.Errorf("expected type length %d, got %d", len(typeBytes), record[1])
	}

	// record[2] should be payload length (for short records)
	if record[2] != byte(len(payload)) {
		t.Errorf("expected payload length %d, got %d", len(payload), record[2])
	}
}

func TestCreateNDEFRecordRaw_LongPayload(t *testing.T) {
	// Create a payload larger than 255 bytes to test long record format
	longPayload := make([]byte, 300)
	for i := range longPayload {
		longPayload[i] = byte(i % 256)
	}

	record := createNDEFRecordRaw(0x02, []byte("application/octet-stream"), longPayload, true, true)

	// For long records, SR bit should NOT be set
	// Header should be: MB(0x80) + ME(0x40) + TNF(0x02) = 0xC2
	if record[0] != 0xC2 {
		t.Errorf("expected header 0xC2 for long record, got 0x%02X", record[0])
	}

	// Payload length should be 4 bytes (big-endian)
	expectedLen := len(longPayload)
	actualLen := int(record[2])<<24 | int(record[3])<<16 | int(record[4])<<8 | int(record[5])
	if actualLen != expectedLen {
		t.Errorf("expected payload length %d, got %d", expectedLen, actualLen)
	}
}

func TestCardStruct(t *testing.T) {
	// Test Card struct with mock data
	for _, mock := range mockCards {
		card := &Card{
			UID:      mock.UID,
			ATR:      mock.ATR,
			Type:     mock.Type,
			Size:     mock.Size,
			Writable: mock.Writable,
			Data:     mock.Data,
			DataType: mock.DataType,
		}

		if card.UID != mock.UID {
			t.Errorf("expected UID %s, got %s", mock.UID, card.UID)
		}
		if card.Type != mock.Type {
			t.Errorf("expected Type %s, got %s", mock.Type, card.Type)
		}
		if card.Size != mock.Size {
			t.Errorf("expected Size %d, got %d", mock.Size, card.Size)
		}
	}
}

func TestNDEFRecordStruct(t *testing.T) {
	tests := []struct {
		recType string
		data    string
	}{
		{"url", "https://example.com"},
		{"text", "Hello World"},
		{"json", `{"key":"value"}`},
		{"binary", "48656c6c6f"}, // hex encoded "Hello"
	}

	for _, tt := range tests {
		t.Run(tt.recType, func(t *testing.T) {
			record := NDEFRecord{
				Type: tt.recType,
				Data: tt.data,
			}

			if record.Type != tt.recType {
				t.Errorf("expected Type %s, got %s", tt.recType, record.Type)
			}
			if record.Data != tt.data {
				t.Errorf("expected Data %s, got %s", tt.data, record.Data)
			}
		})
	}
}

// TestATRPatternMatching tests the ATR pattern matching logic used for card type detection
func TestATRPatternMatching(t *testing.T) {
	tests := []struct {
		name         string
		atr          string
		shouldMatch  string
		expectedType string
	}{
		{
			name:         "MIFARE Classic ATR",
			atr:          mockCards[0].ATR,
			shouldMatch:  "03060300",
			expectedType: "MIFARE Classic",
		},
		{
			name:         "ISO 15693 ATR",
			atr:          mockCards[1].ATR,
			shouldMatch:  "03060b",
			expectedType: "ISO 15693",
		},
		{
			name:         "NTAG213 ATR",
			atr:          mockCards[2].ATR,
			shouldMatch:  "03060300",
			expectedType: "NTAG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !contains(tt.atr, tt.shouldMatch) {
				t.Errorf("ATR %s should contain pattern %s", tt.atr, tt.shouldMatch)
			}

			// Check ATR starts with 3b8f (common for ISO 14443 cards)
			if tt.atr[0:4] != "3b8f" {
				t.Errorf("ATR %s should start with 3b8f", tt.atr)
			}

			// Check length is sufficient for pattern matching
			if len(tt.atr) < 30 {
				t.Errorf("ATR %s too short for pattern matching (len=%d)", tt.atr, len(tt.atr))
			}
		})
	}
}

// TestMIFAREvsNTAGDistinction tests the byte 14 distinction between MIFARE and NTAG
func TestMIFAREvsNTAGDistinction(t *testing.T) {
	// MIFARE Classic should have byte 14 (position 28-29 in hex string) = "01"
	mifareATR := mockCards[0].ATR // 3b8f8001804f0ca000000306030001000000006a
	if len(mifareATR) >= 30 && mifareATR[28:30] != "01" {
		t.Errorf("MIFARE ATR byte 14 should be 01, got %s", mifareATR[28:30])
	}

	// NTAG should have byte 14 (position 28-29 in hex string) = "03"
	ntagATR := mockCards[2].ATR // 3b8f8001804f0ca0000003060300030000000068
	if len(ntagATR) >= 30 && ntagATR[28:30] != "03" {
		t.Errorf("NTAG ATR byte 14 should be 03, got %s", ntagATR[28:30])
	}
}

// TestURIPrefixRoundtrip tests that URI encoding/decoding is consistent
func TestURIPrefixRoundtrip(t *testing.T) {
	uris := []string{
		"https://example.com",
		"http://example.com",
		"tel:+1234567890",
		"mailto:test@example.com",
	}

	for _, uri := range uris {
		t.Run(uri, func(t *testing.T) {
			code, remainder := findURIPrefix(uri)
			prefix := getURIPrefix(code)
			reconstructed := prefix + remainder

			if reconstructed != uri {
				t.Errorf("URI roundtrip failed: %s -> %s", uri, reconstructed)
			}
		})
	}
}

// Benchmark tests
func BenchmarkFindURIPrefix(b *testing.B) {
	uri := "https://example.com/path/to/resource?query=value"
	for i := 0; i < b.N; i++ {
		findURIPrefix(uri)
	}
}

func BenchmarkGetURIPrefix(b *testing.B) {
	for i := 0; i < b.N; i++ {
		getURIPrefix(0x04)
	}
}

func BenchmarkCreateNDEFTextRecord(b *testing.B) {
	text := "Hello, World! This is a test message for benchmarking."
	for i := 0; i < b.N; i++ {
		createNDEFTextRecord(text)
	}
}

func BenchmarkCreateNDEFURIRecord(b *testing.B) {
	uri := "https://example.com/path"
	for i := 0; i < b.N; i++ {
		createNDEFURIRecord(uri)
	}
}

func BenchmarkContains(b *testing.B) {
	s := mockCards[0].ATR
	substr := "03060300"
	for i := 0; i < b.N; i++ {
		contains(s, substr)
	}
}
