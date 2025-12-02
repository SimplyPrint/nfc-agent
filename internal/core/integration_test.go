//go:build integration
// +build integration

// Integration tests for NFC hardware
//
// These tests require actual NFC reader hardware and tags to run.
// They are NOT run automatically in CI - use the following command on a
// machine with NFC readers connected:
//
//   go test -tags=integration -v ./internal/core/...
//
// For best results, have the following tags available:
//   - MIFARE Classic tag on one reader
//   - ISO 15693 / SLIX tag on another reader
//   - NTAG213/215/216 tag on a third reader

package core

import (
	"strings"
	"testing"
	"time"
)

// TestIntegration_ListReaders tests listing available NFC readers
func TestIntegration_ListReaders(t *testing.T) {
	readers := ListReaders()

	t.Logf("Found %d reader(s)", len(readers))
	for i, r := range readers {
		t.Logf("  [%d] %s (%s)", i, r.Name, r.Type)
	}

	if len(readers) == 0 {
		t.Skip("No readers found - skipping hardware tests")
	}
}

// TestIntegration_ReadCard tests reading a card from each connected reader
func TestIntegration_ReadCard(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	for i, reader := range readers {
		t.Run(reader.Name, func(t *testing.T) {
			card, err := GetCardUID(reader.Name)
			if err != nil {
				if strings.Contains(err.Error(), "connect") {
					t.Skipf("No card present on reader %d", i)
				}
				t.Logf("Reader %d (%s): %v", i, reader.Name, err)
				return
			}

			t.Logf("Reader %d (%s):", i, reader.Name)
			t.Logf("  UID: %s", card.UID)
			t.Logf("  ATR: %s", card.ATR)
			t.Logf("  Type: %s", card.Type)
			t.Logf("  Size: %d bytes", card.Size)
			t.Logf("  Writable: %v", card.Writable)
			if card.Data != "" {
				t.Logf("  Data: %s", card.Data)
				t.Logf("  DataType: %s", card.DataType)
			}
			if card.URL != "" {
				t.Logf("  URL: %s", card.URL)
			}

			// Validate card data
			if card.UID == "" {
				t.Error("UID should not be empty")
			}
			if card.ATR == "" {
				t.Error("ATR should not be empty")
			}
			if card.Type == "" {
				t.Error("Type should not be empty")
			}
		})
	}
}

// TestIntegration_DetectCardTypes tests card type detection
func TestIntegration_DetectCardTypes(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	// All NTAG types we have test data for
	expectedTypes := []string{
		"NTAG213", // UID: 0442488a837280
		"NTAG215", // UID: 04635d6bc22a81
		"NTAG216", // UID: 5397e01aa20001
		"MIFARE Classic", // UID: 932bae0e
		"ISO 15693",      // UID: 80391566080104e0
	}

	foundTypes := make(map[string]bool)

	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}
		foundTypes[card.Type] = true
	}

	t.Log("Found card types:")
	for cardType := range foundTypes {
		t.Logf("  - %s", cardType)
	}

	for _, expected := range expectedTypes {
		if foundTypes[expected] {
			t.Logf("✓ Found %s", expected)
		}
	}
}

// TestIntegration_WriteAndReadText tests writing and reading text data
// WARNING: This will modify the tag!
func TestIntegration_WriteAndReadText(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	// Find an NTAG reader with a card
	var testReader string
	var originalData string
	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}
		// Only test on NTAG cards (safer for testing)
		if strings.HasPrefix(card.Type, "NTAG") {
			testReader = reader.Name
			originalData = card.Data
			t.Logf("Testing on %s with %s", reader.Name, card.Type)
			break
		}
	}

	if testReader == "" {
		t.Skip("No NTAG card found for write test")
	}

	// Write test data
	testText := "Integration Test " + time.Now().Format("15:04:05")
	t.Logf("Writing: %s", testText)

	err := WriteData(testReader, []byte(testText), "text")
	if err != nil {
		t.Fatalf("WriteData failed: %v", err)
	}

	// Read back
	card, err := GetCardUID(testReader)
	if err != nil {
		t.Fatalf("GetCardUID after write failed: %v", err)
	}

	t.Logf("Read back: %s", card.Data)
	if card.Data != testText {
		t.Errorf("Data mismatch: wrote %q, read %q", testText, card.Data)
	}

	// Restore original data if there was any
	if originalData != "" {
		t.Logf("Restoring original data: %s", originalData)
		WriteData(testReader, []byte(originalData), "text")
	}
}

// TestIntegration_WriteURL tests writing a URL to the tag
// WARNING: This will modify the tag!
func TestIntegration_WriteURL(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	var testReader string
	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}
		if strings.HasPrefix(card.Type, "NTAG") {
			testReader = reader.Name
			break
		}
	}

	if testReader == "" {
		t.Skip("No NTAG card found for write test")
	}

	testURL := "https://example.com/test"
	t.Logf("Writing URL: %s", testURL)

	err := WriteData(testReader, []byte(testURL), "url")
	if err != nil {
		t.Fatalf("WriteData (URL) failed: %v", err)
	}

	card, err := GetCardUID(testReader)
	if err != nil {
		t.Fatalf("GetCardUID after write failed: %v", err)
	}

	t.Logf("Read URL: %s", card.URL)
	if card.URL != testURL {
		t.Errorf("URL mismatch: wrote %q, read %q", testURL, card.URL)
	}
}

// TestIntegration_WriteMultiRecord tests writing multiple NDEF records
// WARNING: This will modify the tag!
func TestIntegration_WriteMultiRecord(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	var testReader string
	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}
		if strings.HasPrefix(card.Type, "NTAG") {
			testReader = reader.Name
			break
		}
	}

	if testReader == "" {
		t.Skip("No NTAG card found for write test")
	}

	records := []NDEFRecord{
		{Type: "url", Data: "https://simplyprint.io"},
		{Type: "text", Data: "Hello from integration test"},
	}

	t.Log("Writing multi-record NDEF message")
	err := WriteMultipleRecords(testReader, records)
	if err != nil {
		t.Fatalf("WriteMultipleRecords failed: %v", err)
	}

	card, err := GetCardUID(testReader)
	if err != nil {
		t.Fatalf("GetCardUID after write failed: %v", err)
	}

	t.Logf("Read URL: %s", card.URL)
	t.Logf("Read Data: %s", card.Data)

	if card.URL != "https://simplyprint.io" {
		t.Errorf("URL mismatch: expected 'https://simplyprint.io', got %q", card.URL)
	}
}

// TestIntegration_EraseCard tests erasing a card
// WARNING: This will modify the tag!
func TestIntegration_EraseCard(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	var testReader string
	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}
		if strings.HasPrefix(card.Type, "NTAG") {
			testReader = reader.Name
			break
		}
	}

	if testReader == "" {
		t.Skip("No NTAG card found for erase test")
	}

	// Write some data first
	WriteData(testReader, []byte("Test data to erase"), "text")

	// Erase
	t.Log("Erasing card...")
	err := EraseCard(testReader)
	if err != nil {
		t.Fatalf("EraseCard failed: %v", err)
	}

	// Verify erased
	card, err := GetCardUID(testReader)
	if err != nil {
		t.Fatalf("GetCardUID after erase failed: %v", err)
	}

	if card.Data != "" {
		t.Errorf("Card should be empty after erase, got: %s", card.Data)
	}
	t.Log("Card erased successfully")
}

// TestIntegration_ATRPatterns logs ATR patterns for debugging
func TestIntegration_ATRPatterns(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	t.Log("ATR patterns for connected cards:")
	t.Log("================================")

	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}

		t.Logf("\nReader: %s", reader.Name)
		t.Logf("  Card Type: %s", card.Type)
		t.Logf("  UID: %s", card.UID)
		t.Logf("  ATR: %s", card.ATR)
		t.Logf("  ATR Length: %d hex chars", len(card.ATR))

		// Check for known patterns
		if contains(card.ATR, "03060b") {
			t.Log("  Pattern: ISO 15693 (ICode SLI/Slix)")
		}
		if contains(card.ATR, "03060300") {
			if len(card.ATR) >= 30 {
				byte14 := card.ATR[28:30]
				t.Logf("  Pattern: ISO 14443 (byte14=%s)", byte14)
				if byte14 == "01" {
					t.Log("  -> MIFARE Classic")
				} else if byte14 == "03" {
					t.Log("  -> NTAG family")
				}
			}
		}
	}
}

// TestIntegration_CardSizes validates detected card sizes
func TestIntegration_CardSizes(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	expectedSizes := map[string]int{
		"NTAG213":        180,  // Real UID: 0442488a837280
		"NTAG215":        504,  // Real UID: 04635d6bc22a81
		"NTAG216":        888,  // Real UID: 5397e01aa20001
		"MIFARE Classic": 1024, // Real UID: 932bae0e
	}

	for _, reader := range readers {
		card, err := GetCardUID(reader.Name)
		if err != nil {
			continue
		}

		expectedSize, ok := expectedSizes[card.Type]
		if ok {
			if card.Size != expectedSize {
				t.Errorf("%s: expected size %d, got %d", card.Type, expectedSize, card.Size)
			} else {
				t.Logf("%s: size %d bytes ✓", card.Type, card.Size)
			}
		}
	}
}

// TestIntegration_Benchmark runs performance tests on real hardware
func TestIntegration_Benchmark(t *testing.T) {
	readers := ListReaders()
	if len(readers) == 0 {
		t.Skip("No readers found")
	}

	var testReader string
	for _, reader := range readers {
		_, err := GetCardUID(reader.Name)
		if err == nil {
			testReader = reader.Name
			break
		}
	}

	if testReader == "" {
		t.Skip("No card present")
	}

	// Benchmark card reads
	iterations := 10
	start := time.Now()
	for i := 0; i < iterations; i++ {
		GetCardUID(testReader)
	}
	elapsed := time.Since(start)

	avgRead := elapsed / time.Duration(iterations)
	t.Logf("Average read time: %v (%d iterations)", avgRead, iterations)

	if avgRead > 500*time.Millisecond {
		t.Logf("Warning: Read time is slow (>500ms)")
	}
}
