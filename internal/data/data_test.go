package data

import (
	"encoding/json"
	"testing"
)

func TestGetSupportedReaders(t *testing.T) {
	readers, err := GetSupportedReaders()
	if err != nil {
		t.Fatalf("GetSupportedReaders() returned error: %v", err)
	}

	// Should return at least some readers
	if len(readers) == 0 {
		t.Error("GetSupportedReaders() returned empty list")
	}
}

func TestGetSupportedReaders_ValidData(t *testing.T) {
	readers, err := GetSupportedReaders()
	if err != nil {
		t.Fatalf("GetSupportedReaders() returned error: %v", err)
	}

	for i, reader := range readers {
		// Each reader should have required fields
		if reader.Name == "" {
			t.Errorf("reader[%d] has empty Name", i)
		}
		if reader.Manufacturer == "" {
			t.Errorf("reader[%d] (%s) has empty Manufacturer", i, reader.Name)
		}

		// SupportedTags should be non-empty for NFC readers
		if len(reader.SupportedTags) == 0 {
			t.Errorf("reader[%d] (%s) has no SupportedTags", i, reader.Name)
		}
	}
}

func TestGetSupportedReaders_Capabilities(t *testing.T) {
	readers, err := GetSupportedReaders()
	if err != nil {
		t.Fatalf("GetSupportedReaders() returned error: %v", err)
	}

	for _, reader := range readers {
		// All NFC readers should support read
		if !reader.Capabilities.Read {
			t.Errorf("reader %s should support read capability", reader.Name)
		}

		// Most NFC readers should support NDEF
		// (some very old readers might not, but for our supported list they should)
		if !reader.Capabilities.NDEF {
			t.Logf("Warning: reader %s does not support NDEF", reader.Name)
		}
	}
}

func TestSupportedReaderStruct(t *testing.T) {
	reader := SupportedReader{
		Name:          "Test Reader",
		Manufacturer:  "Test Corp",
		Description:   "A test reader",
		SupportedTags: []string{"NTAG213", "MIFARE Classic"},
		Capabilities: ReaderCapability{
			Read:      true,
			Write:     true,
			NDEF:      true,
			Display:   false,
			Bluetooth: false,
		},
		Limitations: []string{"Limited range"},
	}

	if reader.Name != "Test Reader" {
		t.Errorf("expected Name 'Test Reader', got %s", reader.Name)
	}
	if reader.Manufacturer != "Test Corp" {
		t.Errorf("expected Manufacturer 'Test Corp', got %s", reader.Manufacturer)
	}
	if len(reader.SupportedTags) != 2 {
		t.Errorf("expected 2 SupportedTags, got %d", len(reader.SupportedTags))
	}
	if !reader.Capabilities.Read {
		t.Error("expected Read capability to be true")
	}
	if len(reader.Limitations) != 1 {
		t.Errorf("expected 1 Limitation, got %d", len(reader.Limitations))
	}
}

func TestReaderCapabilityStruct(t *testing.T) {
	tests := []struct {
		name      string
		cap       ReaderCapability
		expectStr string
	}{
		{
			name: "full capabilities",
			cap: ReaderCapability{
				Read:      true,
				Write:     true,
				NDEF:      true,
				Display:   true,
				Bluetooth: true,
			},
		},
		{
			name: "read only",
			cap: ReaderCapability{
				Read:  true,
				Write: false,
				NDEF:  true,
			},
		},
		{
			name: "basic reader",
			cap: ReaderCapability{
				Read:  true,
				Write: true,
				NDEF:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify JSON serialization works
			data, err := json.Marshal(tt.cap)
			if err != nil {
				t.Fatalf("failed to marshal ReaderCapability: %v", err)
			}

			var decoded ReaderCapability
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal ReaderCapability: %v", err)
			}

			if decoded.Read != tt.cap.Read {
				t.Errorf("Read mismatch: got %v, want %v", decoded.Read, tt.cap.Read)
			}
			if decoded.Write != tt.cap.Write {
				t.Errorf("Write mismatch: got %v, want %v", decoded.Write, tt.cap.Write)
			}
			if decoded.NDEF != tt.cap.NDEF {
				t.Errorf("NDEF mismatch: got %v, want %v", decoded.NDEF, tt.cap.NDEF)
			}
		})
	}
}

func TestSupportedReadersData_JSONStructure(t *testing.T) {
	// Test that the embedded JSON has the expected structure
	var data SupportedReadersData
	if err := json.Unmarshal(supportedReadersJSON, &data); err != nil {
		t.Fatalf("failed to unmarshal embedded JSON: %v", err)
	}

	if data.Readers == nil {
		t.Error("Readers slice should not be nil")
	}
}

func TestGetSupportedReaders_KnownReaders(t *testing.T) {
	readers, err := GetSupportedReaders()
	if err != nil {
		t.Fatalf("GetSupportedReaders() returned error: %v", err)
	}

	// Check for some known popular readers
	knownReaders := []string{
		"ACR122U",
		"ACR1252",
	}

	for _, known := range knownReaders {
		found := false
		for _, reader := range readers {
			if containsString(reader.Name, known) {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Note: known reader %s not found in supported list", known)
		}
	}
}

func TestGetSupportedReaders_TagTypes(t *testing.T) {
	readers, err := GetSupportedReaders()
	if err != nil {
		t.Fatalf("GetSupportedReaders() returned error: %v", err)
	}

	// Common tag types that should be supported by most readers
	commonTags := []string{"NTAG213", "NTAG215", "NTAG216", "MIFARE Classic"}

	for _, tag := range commonTags {
		foundSupport := false
		for _, reader := range readers {
			for _, supportedTag := range reader.SupportedTags {
				if supportedTag == tag {
					foundSupport = true
					break
				}
			}
			if foundSupport {
				break
			}
		}
		if !foundSupport {
			t.Logf("Note: tag type %s not explicitly supported by any reader", tag)
		}
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// JSON serialization tests
func TestSupportedReader_JSONSerialization(t *testing.T) {
	reader := SupportedReader{
		Name:          "ACS ACR122U",
		Manufacturer:  "ACS",
		Description:   "Popular USB NFC reader",
		SupportedTags: []string{"NTAG213", "NTAG215", "NTAG216", "MIFARE Classic"},
		Capabilities: ReaderCapability{
			Read:  true,
			Write: true,
			NDEF:  true,
		},
		Limitations: []string{"Requires PC/SC driver"},
	}

	// Serialize
	data, err := json.Marshal(reader)
	if err != nil {
		t.Fatalf("failed to marshal SupportedReader: %v", err)
	}

	// Deserialize
	var decoded SupportedReader
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SupportedReader: %v", err)
	}

	// Verify fields
	if decoded.Name != reader.Name {
		t.Errorf("Name mismatch: got %s, want %s", decoded.Name, reader.Name)
	}
	if decoded.Manufacturer != reader.Manufacturer {
		t.Errorf("Manufacturer mismatch: got %s, want %s", decoded.Manufacturer, reader.Manufacturer)
	}
	if len(decoded.SupportedTags) != len(reader.SupportedTags) {
		t.Errorf("SupportedTags length mismatch: got %d, want %d", len(decoded.SupportedTags), len(reader.SupportedTags))
	}
	if decoded.Capabilities.Read != reader.Capabilities.Read {
		t.Errorf("Capabilities.Read mismatch: got %v, want %v", decoded.Capabilities.Read, reader.Capabilities.Read)
	}
}

// Benchmark tests
func BenchmarkGetSupportedReaders(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetSupportedReaders()
	}
}

func BenchmarkSupportedReaderJSON(b *testing.B) {
	reader := SupportedReader{
		Name:          "Test Reader",
		Manufacturer:  "Test",
		SupportedTags: []string{"NTAG213", "NTAG215"},
		Capabilities: ReaderCapability{
			Read:  true,
			Write: true,
			NDEF:  true,
		},
	}

	b.Run("Marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			json.Marshal(reader)
		}
	})

	data, _ := json.Marshal(reader)
	b.Run("Unmarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var r SupportedReader
			json.Unmarshal(data, &r)
		}
	})
}
