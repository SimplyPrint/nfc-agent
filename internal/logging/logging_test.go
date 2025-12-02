package logging

import (
	"fmt"
	"testing"
)

func TestLogger_BasicOperations(t *testing.T) {
	// Create a fresh logger for testing
	logger := &Logger{
		entries:  make([]Entry, 100),
		maxSize:  100,
		minLevel: LevelDebug,
	}

	// Test adding entries
	logger.Info(CatSystem, "Test message", map[string]any{"key": "value"})
	logger.Debug(CatCard, "Debug message", nil)
	logger.Warn(CatHTTP, "Warning message", nil)
	logger.Error(CatWebSocket, "Error message", nil)

	entries := logger.GetEntries(0, nil, nil)
	if len(entries) != 4 {
		t.Errorf("Expected 4 entries, got %d", len(entries))
	}

	// Verify order (newest first)
	if entries[0].Level != LevelError {
		t.Errorf("Expected newest entry to be ERROR, got %s", entries[0].Level)
	}
}

func TestLogger_RingBuffer(t *testing.T) {
	logger := &Logger{
		entries:  make([]Entry, 5),
		maxSize:  5,
		minLevel: LevelDebug,
	}

	// Add more entries than buffer size
	for i := 0; i < 10; i++ {
		logger.Info(CatSystem, fmt.Sprintf("Message %d", i), nil)
	}

	entries := logger.GetEntries(0, nil, nil)
	if len(entries) != 5 {
		t.Errorf("Expected 5 entries (ring buffer), got %d", len(entries))
	}

	// Verify oldest entries were overwritten
	if entries[0].Message != "Message 9" {
		t.Errorf("Expected newest message to be 'Message 9', got '%s'", entries[0].Message)
	}
}

func TestLogger_MinLevelFilter(t *testing.T) {
	logger := &Logger{
		entries:  make([]Entry, 100),
		maxSize:  100,
		minLevel: LevelWarn, // Only warn and error
	}

	logger.Debug(CatSystem, "Debug", nil)
	logger.Info(CatSystem, "Info", nil)
	logger.Warn(CatSystem, "Warn", nil)
	logger.Error(CatSystem, "Error", nil)

	entries := logger.GetEntries(0, nil, nil)
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries (warn and error only), got %d", len(entries))
	}
}

func TestLogger_GetEntriesWithFilters(t *testing.T) {
	logger := &Logger{
		entries:  make([]Entry, 100),
		maxSize:  100,
		minLevel: LevelDebug,
	}

	logger.Info(CatHTTP, "HTTP message", nil)
	logger.Warn(CatHTTP, "HTTP warning", nil)
	logger.Info(CatCard, "Card message", nil)
	logger.Error(CatCard, "Card error", nil)

	// Filter by level
	warnLevel := LevelWarn
	entries := logger.GetEntries(0, &warnLevel, nil)
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries with warn+ level, got %d", len(entries))
	}

	// Filter by category
	httpCat := CatHTTP
	entries = logger.GetEntries(0, nil, &httpCat)
	if len(entries) != 2 {
		t.Errorf("Expected 2 HTTP entries, got %d", len(entries))
	}

	// Filter by both
	entries = logger.GetEntries(0, &warnLevel, &httpCat)
	if len(entries) != 1 {
		t.Errorf("Expected 1 HTTP warn+ entry, got %d", len(entries))
	}
}

func TestLogger_Limit(t *testing.T) {
	logger := &Logger{
		entries:  make([]Entry, 100),
		maxSize:  100,
		minLevel: LevelDebug,
	}

	for i := 0; i < 50; i++ {
		logger.Info(CatSystem, "Message", nil)
	}

	entries := logger.GetEntries(10, nil, nil)
	if len(entries) != 10 {
		t.Errorf("Expected 10 entries with limit, got %d", len(entries))
	}
}

func TestLogger_Clear(t *testing.T) {
	logger := &Logger{
		entries:  make([]Entry, 100),
		maxSize:  100,
		minLevel: LevelDebug,
	}

	logger.Info(CatSystem, "Message", nil)
	logger.Clear()

	entries := logger.GetEntries(0, nil, nil)
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", len(entries))
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("Level.String() = %s, want %s", got, tt.expected)
		}
	}
}
