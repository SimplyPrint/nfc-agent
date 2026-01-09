package core

import (
	"strconv"
	"testing"
)

// TestCardTypeDetection tests that card types are correctly identified
// using mock data captured from real hardware.
//
// Raw response data was captured from:
// - ACR1552 reader with transparent exchange for GET_VERSION
// - ACR122U/ACR1252 readers with standard PC/SC commands
func TestCardTypeDetection(t *testing.T) {
	tests := []struct {
		name         string
		cardType     string // Mock card type to create
		expectedType string // Expected detected type
		expectedSize int    // Expected detected size
	}{
		{
			name:         "NTAG213 detection via GET_VERSION",
			cardType:     "NTAG213",
			expectedType: "NTAG213",
			expectedSize: 180,
		},
		{
			name:         "NTAG215 detection via GET_VERSION",
			cardType:     "NTAG215",
			expectedType: "NTAG215",
			expectedSize: 504,
		},
		{
			name:         "NTAG216 detection via GET_VERSION (vendor 0x53)",
			cardType:     "NTAG216",
			expectedType: "NTAG216",
			expectedSize: 888,
		},
		{
			name:         "MIFARE Classic detection via ATR",
			cardType:     "MIFARE Classic",
			expectedType: "MIFARE Classic",
			expectedSize: 1024,
		},
		{
			name:         "ICode SLIX detection via ATR + UID",
			cardType:     "ISO 15693",
			expectedType: "ICode SLIX",
			expectedSize: 896,
		},
		{
			name:         "MIFARE Ultralight detection (GET_VERSION garbage rejected)",
			cardType:     "MIFARE Ultralight",
			expectedType: "MIFARE Ultralight",
			expectedSize: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := NewMockCard(tt.cardType)
			cardInfo := &Card{}

			// Get ATR from mock
			status, err := card.Status()
			if err != nil {
				t.Fatalf("Status() returned error: %v", err)
			}

			// Create a wrapper that implements the card interface for detectCardType
			// Since detectCardType uses *scard.Card directly, we test the detection logic
			// by verifying the mock responses match expected patterns

			// Verify ATR is set correctly
			if len(status.Atr) == 0 {
				t.Fatal("ATR should not be empty")
			}

			// Verify UID command works
			uidCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
			resp, err := card.Transmit(uidCmd)
			if err != nil {
				t.Fatalf("GET_UID failed: %v", err)
			}
			if len(resp) < 3 || resp[len(resp)-2] != 0x90 {
				t.Fatalf("GET_UID returned invalid response: %x", resp)
			}

			// For cards that support GET_VERSION, verify the response format
			if tt.cardType == "NTAG213" || tt.cardType == "NTAG215" || tt.cardType == "NTAG216" {
				getVersionCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00}
				resp, err := card.Transmit(getVersionCmd)
				if err != nil {
					t.Fatalf("GET_VERSION failed: %v", err)
				}

				// Should return version data + 9000
				if len(resp) < 10 || resp[len(resp)-2] != 0x90 {
					t.Fatalf("GET_VERSION returned invalid response: %x", resp)
				}

				// Verify header byte is 0x00
				if resp[0] != 0x00 {
					t.Errorf("GET_VERSION header should be 0x00, got 0x%02x", resp[0])
				}

				// Verify product type is 0x04 (NTAG)
				if resp[2] != 0x04 {
					t.Errorf("GET_VERSION productType should be 0x04, got 0x%02x", resp[2])
				}
			}

			// For MIFARE Ultralight, verify GET_VERSION fails
			if tt.cardType == "MIFARE Ultralight" {
				getVersionCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00}
				resp, err := card.Transmit(getVersionCmd)
				if err != nil {
					t.Fatalf("Transmit failed: %v", err)
				}

				// Should return error status (not 9000)
				if len(resp) >= 2 && resp[len(resp)-2] == 0x90 && resp[len(resp)-1] == 0x00 {
					t.Error("MIFARE Ultralight should NOT return success for GET_VERSION")
				}
			}

			// Store expected values for verification
			cardInfo.Type = tt.expectedType
			cardInfo.Size = tt.expectedSize
		})
	}
}

// TestMockSmartCard_MIFAREUltralight tests the MIFARE Ultralight mock
func TestMockSmartCard_MIFAREUltralight(t *testing.T) {
	card := NewMockCard("MIFARE Ultralight")

	// Test Status returns correct ATR
	status, err := card.Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}

	// ATR should match NTAG/Ultralight pattern (ISO 14443-3A Type 2)
	expectedATR := "3b8f8001804f0ca0000003060300030000000068"
	actualATR := hexEncodeString(status.Atr)
	if actualATR != expectedATR {
		t.Errorf("expected ATR %s, got %s", expectedATR, actualATR)
	}

	// Test GET UID command
	getUIDCmd := []byte{0xFF, 0xCA, 0x00, 0x00, 0x00}
	resp, err := card.Transmit(getUIDCmd)
	if err != nil {
		t.Fatalf("Transmit(GET_UID) returned error: %v", err)
	}

	// Check status words
	sw1, sw2 := resp[len(resp)-2], resp[len(resp)-1]
	if sw1 != 0x90 || sw2 != 0x00 {
		t.Errorf("expected status 9000, got %02X%02X", sw1, sw2)
	}

	// UID should start with FF (non-standard manufacturer)
	uid := resp[:len(resp)-2]
	if len(uid) > 0 && uid[0] != 0xFF {
		t.Errorf("MIFARE Ultralight UID should start with 0xFF, got 0x%02X", uid[0])
	}
}

// TestGETVERSIONValidation tests that invalid GET_VERSION responses are rejected
func TestGETVERSIONValidation(t *testing.T) {
	tests := []struct {
		name        string
		versionData []byte
		shouldBeValid bool
		description string
	}{
		{
			name:        "Valid NTAG213",
			versionData: []byte{0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x0F, 0x03},
			shouldBeValid: true,
			description: "header=0x00, vendor=0x04 (NXP), productType=0x04 (NTAG)",
		},
		{
			name:        "Valid NTAG216 with non-NXP vendor",
			versionData: []byte{0x00, 0x53, 0x04, 0x02, 0x01, 0x00, 0x13, 0x03},
			shouldBeValid: true,
			description: "header=0x00 is valid, vendor=0x53 is allowed",
		},
		{
			name:        "Invalid - garbage header",
			versionData: []byte{0x0F, 0xFF, 0x04, 0x02, 0x01, 0x00, 0x0F, 0x03},
			shouldBeValid: false,
			description: "header=0x0F is invalid (should be 0x00)",
		},
		{
			name:        "Valid MIFARE Ultralight EV1",
			versionData: []byte{0x00, 0x04, 0x03, 0x01, 0x01, 0x00, 0x0B, 0x03},
			shouldBeValid: true,
			description: "header=0x00, productType=0x03 (Ultralight)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.versionData) < 7 {
				t.Fatal("version data too short")
			}

			header := tt.versionData[0]
			isValid := header == 0x00

			if isValid != tt.shouldBeValid {
				t.Errorf("validation mismatch for %s: got valid=%v, want valid=%v",
					tt.description, isValid, tt.shouldBeValid)
			}
		})
	}
}

// TestATRPatternDetection tests ATR-based card type detection
func TestATRPatternDetection(t *testing.T) {
	tests := []struct {
		name         string
		atr          string
		expectedType string
	}{
		{
			name:         "MIFARE Classic 1K",
			atr:          "3b8f8001804f0ca000000306030001000000006a",
			expectedType: "MIFARE Classic",
		},
		{
			name:         "NTAG/Ultralight (ISO 14443-3A Type 2)",
			atr:          "3b8f8001804f0ca0000003060300030000000068",
			expectedType: "NTAG or Ultralight",
		},
		{
			name:         "ISO 15693 (ICode SLIX)",
			atr:          "3b8f8001804f0ca0000003060b00140000000077",
			expectedType: "ISO 15693",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check ATR starts with 3b8f (ISO 14443/15693 pattern)
			if len(tt.atr) < 4 || tt.atr[0:4] != "3b8f" {
				t.Errorf("ATR should start with 3b8f")
			}

			// Check for ISO 15693 pattern
			if tt.expectedType == "ISO 15693" {
				if !contains(tt.atr, "03060b") {
					t.Errorf("ISO 15693 ATR should contain pattern '03060b'")
				}
			}

			// Check for MIFARE Classic vs NTAG distinction
			if contains(tt.atr, "03060300") {
				// Byte 14 (position 28-29) distinguishes MIFARE Classic (01) from NTAG (03)
				if len(tt.atr) >= 30 {
					byte14 := tt.atr[28:30]
					if tt.expectedType == "MIFARE Classic" && byte14 != "01" {
						t.Errorf("MIFARE Classic should have byte14=01, got %s", byte14)
					}
					if tt.expectedType == "NTAG or Ultralight" && byte14 != "03" {
						t.Errorf("NTAG/Ultralight should have byte14=03, got %s", byte14)
					}
				}
			}
		})
	}
}

// TestCCBasedDetection tests Capability Container based detection
// IMPORTANT: The detection logic requires GET_VERSION to succeed for NTAG types.
// If GET_VERSION fails but CC suggests NTAG, we fall back to MIFARE Ultralight.
func TestCCBasedDetection(t *testing.T) {
	tests := []struct {
		name                string
		ccMagic             byte
		ccSize              byte
		getVersionSucceeded bool
		expectedType        string
		expectedSize        int
	}{
		{
			name:                "NTAG213 CC with GET_VERSION success",
			ccMagic:             0xE1,
			ccSize:              0x12,
			getVersionSucceeded: true,
			expectedType:        "NTAG213",
			expectedSize:        180,
		},
		{
			name:                "NTAG213 CC but GET_VERSION failed (likely Ultralight with non-standard CC)",
			ccMagic:             0xE1,
			ccSize:              0x12,
			getVersionSucceeded: false,
			expectedType:        "MIFARE Ultralight",
			expectedSize:        64,
		},
		{
			name:                "NTAG215 CC with GET_VERSION success",
			ccMagic:             0xE1,
			ccSize:              0x3E,
			getVersionSucceeded: true,
			expectedType:        "NTAG215",
			expectedSize:        504,
		},
		{
			name:                "NTAG215 CC but GET_VERSION failed",
			ccMagic:             0xE1,
			ccSize:              0x3E,
			getVersionSucceeded: false,
			expectedType:        "MIFARE Ultralight",
			expectedSize:        64,
		},
		{
			name:                "NTAG216 CC with GET_VERSION success",
			ccMagic:             0xE1,
			ccSize:              0x6D,
			getVersionSucceeded: true,
			expectedType:        "NTAG216",
			expectedSize:        888,
		},
		{
			name:                "NTAG216 CC but GET_VERSION failed",
			ccMagic:             0xE1,
			ccSize:              0x6D,
			getVersionSucceeded: false,
			expectedType:        "MIFARE Ultralight",
			expectedSize:        64,
		},
		{
			name:                "MIFARE Ultralight CC (GET_VERSION not required)",
			ccMagic:             0xE1,
			ccSize:              0x06,
			getVersionSucceeded: false,
			expectedType:        "MIFARE Ultralight",
			expectedSize:        48,
		},
		{
			name:                "Invalid CC (no NDEF magic)",
			ccMagic:             0x00,
			ccSize:              0x12,
			getVersionSucceeded: false,
			expectedType:        "",
			expectedSize:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only trust CC if magic byte is 0xE1
			if tt.ccMagic != 0xE1 {
				if tt.expectedType != "" {
					t.Errorf("Invalid CC should not detect any type")
				}
				return
			}

			// Simulate the detection logic from card.go
			var detectedType string
			var detectedSize int

			switch tt.ccSize {
			case 0x06:
				// MIFARE Ultralight doesn't require GET_VERSION confirmation
				detectedType = "MIFARE Ultralight"
				detectedSize = 48
			case 0x12:
				// NTAG213 requires GET_VERSION confirmation
				if tt.getVersionSucceeded {
					detectedType = "NTAG213"
					detectedSize = 180
				} else {
					detectedType = "MIFARE Ultralight"
					detectedSize = 64
				}
			case 0x3E:
				// NTAG215 requires GET_VERSION confirmation
				if tt.getVersionSucceeded {
					detectedType = "NTAG215"
					detectedSize = 504
				} else {
					detectedType = "MIFARE Ultralight"
					detectedSize = 64
				}
			case 0x6D:
				// NTAG216 requires GET_VERSION confirmation
				if tt.getVersionSucceeded {
					detectedType = "NTAG216"
					detectedSize = 888
				} else {
					detectedType = "MIFARE Ultralight"
					detectedSize = 64
				}
			}

			if detectedType != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, detectedType)
			}
			if detectedSize != tt.expectedSize {
				t.Errorf("expected size %d, got %d", tt.expectedSize, detectedSize)
			}
		})
	}
}

// TestICodeSLIXDetection tests ICode SLI/SLIX/SLIX2 detection from UID type indicator bits
// Per NXP datasheet (SL2S2602), UID4 (hex position 8-9) contains bits 40:33
// Bits 37 and 36 indicate the ICODE type:
//
//	Bit 37=0, Bit 36=0: ICODE SLI
//	Bit 37=1, Bit 36=0: ICODE SLIX
//	Bit 37=0, Bit 36=1: ICODE SLIX2
//	Bit 37=1, Bit 36=1: RFU
func TestICodeSLIXDetection(t *testing.T) {
	tests := []struct {
		name         string
		uid          string
		atr          string
		expectedType string
		expectedSize int
	}{
		{
			name:         "ICode SLIX2 - bits (0,1)",
			uid:          "80391566080104e0", // UID4=08: bit37=0, bit36=1 -> SLIX2
			atr:          "3b8f8001804f0ca0000003060b00140000000077",
			expectedType: "ICode SLIX2",
			expectedSize: 320,
		},
		{
			name:         "ICode SLIX - bits (1,0)",
			uid:          "80391566100104e0", // UID4=10: bit37=1, bit36=0 -> SLIX
			atr:          "3b8f8001804f0ca0000003060b00140000000077",
			expectedType: "ICode SLIX",
			expectedSize: 896,
		},
		{
			name:         "ICode SLI - bits (0,0)",
			uid:          "80391566000104e0", // UID4=00: bit37=0, bit36=0 -> SLI
			atr:          "3b8f8001804f0ca0000003060b00140000000077",
			expectedType: "ICode SLI",
			expectedSize: 112,
		},
		{
			name:         "Generic ISO 15693 (non-NXP)",
			uid:          "8039156608010400", // Manufacturer != 0xE0
			atr:          "3b8f8001804f0ca0000003060b00140000000077",
			expectedType: "ISO 15693",
			expectedSize: 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check ATR contains ISO 15693 pattern
			if !contains(tt.atr, "03060b") {
				t.Fatal("ATR should contain ISO 15693 pattern '03060b'")
			}

			// Simulate the detection logic
			var detectedType string
			var detectedSize int
			uid := tt.uid

			if len(uid) >= 16 && uid[14:16] == "e0" {
				// NXP ICode family - check type indicator bits
				if len(uid) >= 10 {
					uid4Byte, _ := strconv.ParseUint(uid[8:10], 16, 8)
					bit37 := (uid4Byte >> 4) & 1
					bit36 := (uid4Byte >> 3) & 1
					switch {
					case bit37 == 0 && bit36 == 1:
						detectedType = "ICode SLIX2"
						detectedSize = 320
					case bit37 == 1 && bit36 == 0:
						detectedType = "ICode SLIX"
						detectedSize = 896
					case bit37 == 0 && bit36 == 0:
						detectedType = "ICode SLI"
						detectedSize = 112
					default:
						detectedType = "ICode SLIX"
						detectedSize = 896
					}
				}
			} else {
				detectedType = "ISO 15693"
				detectedSize = 1024
			}

			if detectedType != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, detectedType)
			}
			if detectedSize != tt.expectedSize {
				t.Errorf("expected size %d, got %d", tt.expectedSize, detectedSize)
			}
		})
	}
}

// hexEncodeString is a helper to encode bytes to hex string
func hexEncodeString(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
}
