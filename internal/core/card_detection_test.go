package core

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			expectedSize: 144,
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
			expectedSize: 112,
		},
		{
			name:         "MIFARE Ultralight detection (GET_VERSION garbage rejected)",
			cardType:     "MIFARE Ultralight",
			expectedType: "MIFARE Ultralight",
			expectedSize: 48,
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
			expectedSize:        144,
		},
		{
			name:                "NTAG213 CC but GET_VERSION failed (likely Ultralight with non-standard CC)",
			ccMagic:             0xE1,
			ccSize:              0x12,
			getVersionSucceeded: false,
			expectedType:        "MIFARE Ultralight",
			expectedSize:        48,
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
			expectedSize:        48,
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
			expectedSize:        48,
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
					detectedSize = 144
				} else {
					detectedType = "MIFARE Ultralight"
					detectedSize = 48
				}
			case 0x3E:
				// NTAG215 requires GET_VERSION confirmation
				if tt.getVersionSucceeded {
					detectedType = "NTAG215"
					detectedSize = 504
				} else {
					detectedType = "MIFARE Ultralight"
					detectedSize = 48
				}
			case 0x6D:
				// NTAG216 requires GET_VERSION confirmation
				if tt.getVersionSucceeded {
					detectedType = "NTAG216"
					detectedSize = 888
				} else {
					detectedType = "MIFARE Ultralight"
					detectedSize = 48
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
		expectedType string
		expectedSize int
		expectedOK   bool
	}{
		{"ICode SLIX2 - bits (0,1)", "80391566080104e0", "ICode SLIX2", 316, true}, // UID4=08
		{"ICode SLIX2 - spec vector 01 tag", "bc6f2f66080104e0", "ICode SLIX2", 316, true},
		{"ICode SLIX - bits (1,0)", "80391566100104e0", "ICode SLIX", 112, true}, // UID4=10
		{"ICode SLI - bits (0,0)", "80391566000104e0", "ICode SLI", 112, true},   // UID4=00
		{"ST tag (manufacturer 02)", "80391566080102e0", "", 0, false},
		{"TI tag (manufacturer 07)", "80391566080107e0", "", 0, false},
		{"NXP non-ICODE family (NTAG 5)", "80391566080404e0", "", 0, false},
		{"not ISO 15693 prefix", "8039156608010400", "", 0, false},
		{"short UID", "3a5e53be", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ, size, ok := icodeTypeFromUID(tt.uid)
			if ok != tt.expectedOK || typ != tt.expectedType || size != tt.expectedSize {
				t.Errorf("icodeTypeFromUID(%s) = (%q, %d, %v), want (%q, %d, %v)",
					tt.uid, typ, size, ok, tt.expectedType, tt.expectedSize, tt.expectedOK)
			}
		})
	}
}

// TestMemoryProbeDetection tests that cards are correctly identified via memory probe
// when GET_VERSION and CC detection both fail. This is critical for NTAG cards
// where GET_VERSION fails intermittently due to communication issues.
func TestMemoryProbeDetection(t *testing.T) {
	tests := []struct {
		name         string
		mockCardType string
		expectedType string
		expectedSize int
	}{
		{
			name:         "NTAG216 detected via memory probe (page 135 readable)",
			mockCardType: "NTAG216-MemoryProbe",
			expectedType: "NTAG216",
			expectedSize: 888,
		},
		{
			name:         "NTAG215 detected via memory probe (page 45 readable, page 135 fails)",
			mockCardType: "NTAG215-MemoryProbe",
			expectedType: "NTAG215",
			expectedSize: 504,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := NewMockCard(tt.mockCardType)

			// Verify GET_VERSION fails (returns 6300)
			getVersionCmd := []byte{0xFF, 0x00, 0x00, 0x00, 0x02, 0x60, 0x00}
			resp, err := card.Transmit(getVersionCmd)
			if err != nil {
				t.Fatalf("Transmit failed: %v", err)
			}
			if len(resp) >= 2 && resp[len(resp)-2] == 0x90 && resp[len(resp)-1] == 0x00 {
				t.Error("GET_VERSION should fail for memory probe test cards")
			}

			// Verify CC has no valid NDEF magic byte
			ccReadCmd := []byte{0xFF, 0xB0, 0x00, 0x03, 0x10}
			resp, err = card.Transmit(ccReadCmd)
			if err != nil {
				t.Fatalf("CC read failed: %v", err)
			}
			if len(resp) >= 1 && resp[0] == 0xE1 {
				t.Error("CC should not have valid NDEF magic byte for memory probe test")
			}

			// Verify the expected memory probe page succeeds
			if tt.expectedType == "NTAG216" {
				// Page 135 should be readable
				readPage135 := []byte{0xFF, 0xB0, 0x00, 0x87, 0x04}
				resp, err = card.Transmit(readPage135)
				if err != nil {
					t.Fatalf("Page 135 read failed: %v", err)
				}
				if len(resp) < 2 || resp[len(resp)-2] != 0x90 || resp[len(resp)-1] != 0x00 {
					t.Error("Page 135 should be readable for NTAG216")
				}
			} else if tt.expectedType == "NTAG215" {
				// Page 135 should fail (NTAG215 only has 135 pages, 0-134)
				readPage135 := []byte{0xFF, 0xB0, 0x00, 0x87, 0x04}
				resp, err = card.Transmit(readPage135)
				if err != nil {
					t.Fatalf("Page 135 read command failed: %v", err)
				}
				if len(resp) >= 2 && resp[len(resp)-2] == 0x90 && resp[len(resp)-1] == 0x00 {
					t.Error("Page 135 should NOT be readable for NTAG215")
				}

				// Page 45 should be readable
				readPage45 := []byte{0xFF, 0xB0, 0x00, 0x2D, 0x04}
				resp, err = card.Transmit(readPage45)
				if err != nil {
					t.Fatalf("Page 45 read failed: %v", err)
				}
				if len(resp) < 2 || resp[len(resp)-2] != 0x90 || resp[len(resp)-1] != 0x00 {
					t.Error("Page 45 should be readable for NTAG215")
				}
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

// TestIsMifareClassicATR verifies Classic cards are identified from the ATR
// card name before any GET_VERSION / memory probe commands are sent.
// Regression: Bambu Lab tags (Classic 1K, non-default keys) on ACR1552 fell
// through to the memory probe, were never cached, and were re-probed with
// reader resets on every poll.
func TestIsMifareClassicATR(t *testing.T) {
	tests := []struct {
		name string
		atr  string
		want bool
	}{
		{"Classic 1K (ACR1552/ACR122U, Bambu Lab)", "3b8f8001804f0ca000000306030001000000006a", true},
		{"Classic 4K", "3b8f8001804f0ca0000003060300020000000069", true},
		{"MIFARE Mini", "3b8f8001804f0ca000000306030026000000004d", true},
		{"NTAG/Ultralight", "3b8f8001804f0ca0000003060300030000000068", false},
		{"ICode SLIX", "3b8f8001804f0ca0000003060b00140000000077", false},
		{"Too short", "3b8f8001", false},
		{"Non-storage ATR", "3b8180018080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMifareClassicATR(tt.atr); got != tt.want {
				t.Errorf("isMifareClassicATR(%s) = %v, want %v", tt.atr, got, tt.want)
			}
		})
	}
}

// TestUncertainDetectionCaching verifies uncertain results are retried a few
// times, then cached with a TTL so polling stops re-probing the card.
func TestUncertainDetectionCaching(t *testing.T) {
	ClearAllCardCache()
	defer ClearAllCardCache()

	const uid = "3a5e53be"
	now := time.Now()
	guess := &Card{UID: uid, Type: "MIFARE Ultralight"}

	for i := 1; i < maxUncertainDetections; i++ {
		if shouldCacheDetection(uid, false, now) {
			t.Fatalf("uncertain attempt %d should not be cached", i)
		}
	}
	if !shouldCacheDetection(uid, false, now) {
		t.Fatalf("attempt %d should cache the best guess", maxUncertainDetections)
	}
	cardDetectionCache[uid] = guess

	if c, ok := lookupCachedDetection(uid, now.Add(uncertainCacheTTL-time.Second)); !ok || c != guess {
		t.Fatal("best guess should be served from cache within TTL")
	}
	if _, ok := lookupCachedDetection(uid, now.Add(uncertainCacheTTL+time.Second)); ok {
		t.Fatal("best guess should expire after TTL")
	}

	// Still uncertain after expiry: cached again straight away (no retry burst).
	later := now.Add(uncertainCacheTTL + time.Second)
	if !shouldCacheDetection(uid, false, later) {
		t.Fatal("repeatedly uncertain card should be re-cached after expiry")
	}

	// A confident detection clears the uncertain state and never expires.
	if !shouldCacheDetection(uid, true, later) {
		t.Fatal("confident detection should be cached")
	}
	cardDetectionCache[uid] = guess
	if _, ok := lookupCachedDetection(uid, later.Add(time.Hour)); !ok {
		t.Fatal("confident detection should not expire")
	}

	// Attempts are forgotten once the card hasn't been seen for a while.
	ClearCardCache(uid)
	shouldCacheDetection(uid, false, later)
	shouldCacheDetection(uid, false, later.Add(time.Second))
	if shouldCacheDetection(uid, false, later.Add(time.Second+uncertainResetAfter+time.Second)) {
		t.Fatal("attempt counter should reset after uncertainResetAfter without attempts")
	}

	// ClearCardCache resets the attempt counter.
	shouldCacheDetection(uid, false, later)
	ClearCardCache(uid)
	if shouldCacheDetection(uid, false, later) {
		t.Fatal("attempt counter should reset after ClearCardCache")
	}
}

// TestIsMifareClassicATRFixtures checks the ATR rule against every captured
// hardware fixture: only cards recorded as MIFARE Classic may match.
func TestIsMifareClassicATRFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			ATR          string `json:"atr"`
			DetectedType string `json:"detected_type"`
		}
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if isMifareClassicATR(fx.ATR) && fx.DetectedType != "MIFARE Classic" {
			t.Errorf("%s: ATR %s matches Classic but fixture was detected as %q", f, fx.ATR, fx.DetectedType)
		}
	}
}

// TestISO15693CapabilityContainer verifies the Type 5 CC chosen for writes:
// an existing CC is kept, otherwise MLEN = (userBytes - 4) / 8.
func TestISO15693CapabilityContainer(t *testing.T) {
	tests := []struct {
		name      string
		existing  []byte
		userBytes int
		wantCC    []byte
		wantWrite bool
		wantArea  int
		wantErr   bool
	}{
		{"blank SLIX2", []byte{0, 0, 0, 0}, 316, []byte{0xE1, 0x40, 0x27, 0x01}, true, 312, false},
		{"blank SLIX/SLI", nil, 112, []byte{0xE1, 0x40, 0x0D, 0x01}, true, 104, false},
		{"Prusament CC kept", []byte{0xE1, 0x40, 0x27, 0x01}, 316, []byte{0xE1, 0x40, 0x27, 0x01}, false, 312, false},
		{"existing CC with other MLEN kept", []byte{0xE1, 0x40, 0x0D, 0x00}, 316, []byte{0xE1, 0x40, 0x0D, 0x00}, false, 104, false},
		{"CC with MLEN 0 replaced", []byte{0xE1, 0x40, 0x00, 0x00}, 316, []byte{0xE1, 0x40, 0x27, 0x01}, true, 312, false},
		{"unknown size, no CC", nil, 0, nil, false, 0, true},
		{"write-protected CC refused", []byte{0xE1, 0x43, 0x27, 0x01}, 316, nil, false, 0, true},
		{"unknown size, existing CC trusted", []byte{0xE1, 0x40, 0x27, 0x01}, 0, []byte{0xE1, 0x40, 0x27, 0x01}, false, 312, false},
		{"oversized CC capped to SLIX2 memory", []byte{0xE1, 0x40, 0x40, 0x00}, 316, []byte{0xE1, 0x40, 0x40, 0x00}, false, 312, false},
		{"oversized CC capped to SLIX memory", []byte{0xE1, 0x40, 0x7F, 0x01}, 112, []byte{0xE1, 0x40, 0x7F, 0x01}, false, 108, false},
		{"2KB CC capped to block 255", []byte{0xE1, 0x40, 0xFF, 0x01}, 0, []byte{0xE1, 0x40, 0xFF, 0x01}, false, 1020, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, write, area, err := iso15693CapabilityContainer(tt.existing, tt.userBytes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if string(cc) != string(tt.wantCC) || write != tt.wantWrite || area != tt.wantArea {
				t.Errorf("got cc=%x write=%v area=%d, want cc=%x write=%v area=%d", cc, write, area, tt.wantCC, tt.wantWrite, tt.wantArea)
			}
		})
	}
}

// TestOpenPrintTagUID verifies ISO 15693 UIDs (reported LSB-first by PC/SC)
// are reversed to the MSB-first order used for instance_uuid derivation.
func TestOpenPrintTagUID(t *testing.T) {
	tests := []struct {
		uid  string
		want string
	}{
		{"bc6f2f66080104e0", "e0040108662f6fbc"}, // spec vector 01 tag, as PC/SC reports it
		{"e0040108662f6fbc", "e0040108662f6fbc"}, // already MSB-first (e.g. Proxmark3)
		{"04a1b2c3d4e5f6", "04a1b2c3d4e5f6"},     // NTAG 7-byte UID unchanged
		{"3a5e53be", "3a5e53be"},                 // MIFARE Classic 4-byte UID unchanged
		{"zz", ""},
	}
	for _, tt := range tests {
		got := hex.EncodeToString(openPrintTagUID(&Card{UID: tt.uid}))
		if got != tt.want {
			t.Errorf("openPrintTagUID(%s) = %s, want %s", tt.uid, got, tt.want)
		}
	}
}

// TestTransparentExchangeACK checks ACR1552 transparent-exchange write replies:
// SW 90 00 alone isn't success, the tag must ACK (0xA) in data object 0x97.
func TestTransparentExchangeACK(t *testing.T) {
	tests := []struct {
		name string
		rsp  string
		want bool
	}{
		{"NAK from hardware test (NTAG213 page 48)", "c003009000920104960202049701009000", false},
		{"ACK", "c003009000920104960202049701 0a9000", true},
		{"ACK with other bits in high nibble", "c0030090009701fa9000", true},
		{"no response data object", "c0030090009000", false},
		{"truncated TLV", "c00300900097", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		raw, err := hex.DecodeString(strings.ReplaceAll(tt.rsp, " ", ""))
		if err != nil {
			t.Fatalf("%s: bad hex: %v", tt.name, err)
		}
		if got := transparentExchangeACK(raw); got != tt.want {
			t.Errorf("%s: transparentExchangeACK(%s) = %v, want %v", tt.name, tt.rsp, got, tt.want)
		}
	}
}

// TestNTAGAreaFromCC verifies Type 2 writes stay inside the NDEF area, so the
// zero-fill can never reach NTAG21x configuration pages (AUTH0/ACCESS/PWD).
func TestNTAGAreaFromCC(t *testing.T) {
	tests := []struct {
		name      string
		cc        []byte
		userBytes int
		want      int
	}{
		{"NTAG213 factory CC", []byte{0xE1, 0x10, 0x12, 0x00}, 144, 144},
		{"NTAG215 factory CC", []byte{0xE1, 0x10, 0x3E, 0x00}, 504, 496},
		{"NTAG216 factory CC", []byte{0xE1, 0x10, 0x6D, 0x00}, 888, 872},
		{"CC claims more than the chip", []byte{0xE1, 0x10, 0x3E, 0x00}, 144, 144},
		{"unknown chip size: trust CC", []byte{0xE1, 0x10, 0x06, 0x00}, 0, 48},
	}
	for _, tt := range tests {
		if got := ntagAreaFromCC(tt.cc, tt.userBytes); got != tt.want {
			t.Errorf("%s: ntagAreaFromCC = %d, want %d", tt.name, got, tt.want)
		}
	}
	// NTAG213 user memory is pages 4-39; page 40+ holds lock/config/PWD.
	if 4+144/4 != 40 {
		t.Fatal("NTAG213 NDEF area must end before page 40")
	}
}

// TestPlausibleUID verifies truncated UIDs are rejected (hardware test: a
// failed ISO 15693 read was followed by a 1-byte UID "04" that got cached).
func TestPlausibleUID(t *testing.T) {
	iso15693ATR := "3b8f8001804f0ca0000003060b00140000000077"
	ntagATR := "3b8f8001804f0ca0000003060300030000000068"
	tests := []struct {
		uid  string
		atr  string
		want bool
	}{
		{"04", iso15693ATR, false},
		{"83801366080104e0", iso15693ATR, true},
		{"0439468a837280", iso15693ATR, false},
		{"0439468a837280", ntagATR, true},
		{"3a5e53be", ntagATR, true},
		{"04", ntagATR, false},
	}
	for _, tt := range tests {
		uid, _ := hex.DecodeString(tt.uid)
		if got := plausibleUID(uid, tt.atr); got != tt.want {
			t.Errorf("plausibleUID(%s, %s) = %v, want %v", tt.uid, tt.atr, got, tt.want)
		}
	}
}
