package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if s == nil {
		t.Fatal("DefaultSettings returned nil")
	}
	if s.CrashReporting != false {
		t.Error("CrashReporting should be false by default (opt-in)")
	}
}

func TestGet(t *testing.T) {
	// Reset package state
	mu.Lock()
	current = &Settings{CrashReporting: true}
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		current = nil
		mu.Unlock()
	})

	s := Get()
	if s == nil {
		t.Fatal("Get returned nil")
	}
	if s.CrashReporting != true {
		t.Error("Expected CrashReporting=true")
	}
}

func TestGetReturnsSettingsWhenNotLoaded(t *testing.T) {
	// Reset package state to nil
	mu.Lock()
	oldCurrent := current
	current = nil
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		current = oldCurrent
		mu.Unlock()
	})

	// Get should return valid settings (not panic)
	// Note: This will load from the real settings file if it exists,
	// or return defaults if it doesn't
	s := Get()
	if s == nil {
		t.Fatal("Get returned nil")
	}
	// Just verify we got a valid Settings struct (value doesn't matter
	// since it depends on whether a settings file exists on the test machine)
}

func TestIsCrashReportingEnabled(t *testing.T) {
	// Test when enabled
	mu.Lock()
	current = &Settings{CrashReporting: true}
	mu.Unlock()

	if !IsCrashReportingEnabled() {
		t.Error("Expected IsCrashReportingEnabled() to return true")
	}

	// Test when disabled
	mu.Lock()
	current = &Settings{CrashReporting: false}
	mu.Unlock()

	if IsCrashReportingEnabled() {
		t.Error("Expected IsCrashReportingEnabled() to return false")
	}

	// Cleanup
	mu.Lock()
	current = nil
	mu.Unlock()
}

func TestSettingsFileRoundTrip(t *testing.T) {
	// This test writes and reads from a temp file directly,
	// simulating what the real Load/Save do

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "settings.json")

	// Write settings
	settings := Settings{CrashReporting: true}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	if err := os.WriteFile(testFile, data, 0644); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read settings back
	readData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	var loaded Settings
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if loaded.CrashReporting != true {
		t.Error("Expected CrashReporting=true after round-trip")
	}
}

func TestSettingsJSONFormat(t *testing.T) {
	// Test JSON serialization format
	s := Settings{CrashReporting: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	expected := `{"crashReporting":true,"extendedLogging":false}`
	if string(data) != expected {
		t.Errorf("JSON format mismatch: got %s, want %s", string(data), expected)
	}

	// Test deserialization
	var loaded Settings
	if err := json.Unmarshal([]byte(`{"crashReporting":false}`), &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if loaded.CrashReporting != false {
		t.Error("Expected CrashReporting=false")
	}
}

func TestConcurrentGetAccess(t *testing.T) {
	// Set up initial state
	mu.Lock()
	current = &Settings{CrashReporting: true}
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		current = nil
		mu.Unlock()
	})

	// Test concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := Get()
			if s == nil {
				t.Error("Get returned nil during concurrent access")
			}
		}()
	}
	wg.Wait()

	// Should not panic or deadlock - test passes if we get here
}

func TestInvalidJSONReturnsDefault(t *testing.T) {
	// Test that unmarshaling invalid JSON gives us a zero-value Settings
	var s Settings
	err := json.Unmarshal([]byte("not json"), &s)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	// s should be zero-value
	if s.CrashReporting != false {
		t.Error("Zero-value Settings should have CrashReporting=false")
	}
}
