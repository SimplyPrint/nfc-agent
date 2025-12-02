package core

import "testing"

// Mock reader names from real hardware
var mockReaderNames = []string{
	"ACS ACR122U PICC Interface",
	"ACS ACR1552 1S CL Reader PICC",
	"ACS ACR1252 Dual Reader PICC",
}

func TestDetectReaderType(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		// SAM readers should be detected
		{"ACS ACR1252U SAM Interface", "sam"},
		{"ACS ACR1252 SAM Reader", "sam"},
		{"Reader SAM 0", "sam"},
		{"ACS ACR1552 1S CL Reader SAM 0", "sam"},
		{"SCM Microsystems SCL010 SAM Slot", "sam"},
		{"Gemalto SAM Module", "sam"},

		// PICC readers should be detected (real hardware names)
		{"ACS ACR122U PICC Interface", "picc"},
		{"ACS ACR1252 1S CL Reader PICC 0", "picc"},
		{"ACS ACR1255U-J1 PICC", "picc"},
		{"ACS ACR1552 1S CL Reader PICC", "picc"},
		{"ACS ACR1252 Dual Reader PICC", "picc"},

		// Readers without explicit type should default to PICC
		{"ACS ACR122U", "picc"},
		{"Generic USB Reader", "picc"},
		{"HID OMNIKEY 5022 CL", "picc"},
		{"Identiv uTrust 3700 F", "picc"},
		{"SCM Microsystems SCL011 Contactless Reader", "picc"},
		{"Feitian R502 CL", "picc"},
		{"ACS ACR39U ICC Reader", "picc"},
		{"Cherry SmartTerminal ST-1144", "picc"},

		// Edge cases
		{"", "picc"},                    // Empty string defaults to PICC
		{"SAM", "picc"},                 // Just "SAM" without space before or after
		{" SAM ", "sam"},                // SAM with spaces
		{"MySAMReader", "picc"},         // SAM embedded in word without spaces
		{"PICC Reader", "picc"},         // PICC at start
		{"Reader PICC", "picc"},         // PICC at end
		{"SamSung Reader", "picc"},      // Samsung (not SAM reader)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectReaderType(tt.name)
			if result != tt.expected {
				t.Errorf("detectReaderType(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

func TestDetectReaderType_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"ACS ACR1252U SAM Interface", "sam"},
		{"ACS ACR1252U sam Interface", "sam"},
		{"ACS ACR1252U Sam Interface", "sam"},
		{"ACS ACR1252U SAm Interface", "sam"},
		{"ACS ACR122U PICC Interface", "picc"},
		{"ACS ACR122U picc Interface", "picc"},
		{"ACS ACR122U Picc Interface", "picc"},
		{"ACS ACR122U PiCc Interface", "picc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectReaderType(tt.name)
			if result != tt.expected {
				t.Errorf("detectReaderType(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

func TestDetectReaderType_RealHardware(t *testing.T) {
	// Test with actual hardware reader names from the mock data
	for _, readerName := range mockReaderNames {
		t.Run(readerName, func(t *testing.T) {
			result := detectReaderType(readerName)
			if result != "picc" {
				t.Errorf("detectReaderType(%q) = %q, want 'picc'", readerName, result)
			}
		})
	}
}

func TestReaderStruct(t *testing.T) {
	tests := []struct {
		id       string
		name     string
		readerType string
	}{
		{"reader-0", "ACS ACR122U PICC Interface", "picc"},
		{"reader-1", "ACS ACR1552 1S CL Reader PICC", "picc"},
		{"reader-2", "ACS ACR1252 Dual Reader PICC", "picc"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			reader := Reader{
				ID:   tt.id,
				Name: tt.name,
				Type: tt.readerType,
			}

			if reader.ID != tt.id {
				t.Errorf("expected ID %s, got %s", tt.id, reader.ID)
			}
			if reader.Name != tt.name {
				t.Errorf("expected Name %s, got %s", tt.name, reader.Name)
			}
			if reader.Type != tt.readerType {
				t.Errorf("expected Type %s, got %s", tt.readerType, reader.Type)
			}
		})
	}
}

func TestDetectReaderType_CommonManufacturers(t *testing.T) {
	// Test various manufacturers that are commonly used
	manufacturers := []struct {
		name     string
		expected string
	}{
		// ACS readers
		{"ACS ACR122U", "picc"},
		{"ACS ACR1252U-A1", "picc"},
		{"ACS ACR1252 1S CL Reader PICC 0", "picc"},
		{"ACS ACR1552 1S CL Reader PICC", "picc"},
		{"ACS ACR1552 1S CL Reader SAM 0", "sam"},
		{"ACS ACR38U", "picc"},
		{"ACS ACR39U ICC Reader", "picc"},

		// HID readers
		{"HID OMNIKEY 5022 CL", "picc"},
		{"HID OMNIKEY 5025 CL", "picc"},
		{"HID OMNIKEY 5427 CK", "picc"},
		{"HID OMNIKEY 3021", "picc"},

		// Identiv readers
		{"Identiv uTrust 3700 F", "picc"},
		{"Identiv uTrust 4700 F", "picc"},
		{"Identiv SCR3310v2.0 USB Smart Card Reader", "picc"},

		// SCM readers
		{"SCM Microsystems SCL011 Contactless Reader", "picc"},
		{"SCM Microsystems SCL010 SAM Slot", "sam"},

		// Gemalto readers
		{"Gemalto PC Twin Reader", "picc"},
		{"Gemalto IDBridge CT40", "picc"},

		// Feitian readers
		{"Feitian R502 CL", "picc"},
		{"Feitian bR500", "picc"},
	}

	for _, tt := range manufacturers {
		t.Run(tt.name, func(t *testing.T) {
			result := detectReaderType(tt.name)
			if result != tt.expected {
				t.Errorf("detectReaderType(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

// Benchmark test for reader type detection
func BenchmarkDetectReaderType(b *testing.B) {
	readerName := "ACS ACR1252 Dual Reader PICC"
	for i := 0; i < b.N; i++ {
		detectReaderType(readerName)
	}
}

func BenchmarkDetectReaderType_NoMatch(b *testing.B) {
	readerName := "Generic USB Smart Card Reader"
	for i := 0; i < b.N; i++ {
		detectReaderType(readerName)
	}
}
