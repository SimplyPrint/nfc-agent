package logging

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Level represents the severity of a log entry.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func (l Level) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
}

// Category groups related log entries.
type Category string

const (
	CatHTTP      Category = "http"
	CatWebSocket Category = "websocket"
	CatReader    Category = "reader"
	CatCard      Category = "card"
	CatSystem    Category = "system"
)

// Entry represents a single log entry.
type Entry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     Level             `json:"level"`
	Category  Category          `json:"category"`
	Message   string            `json:"message"`
	Data      map[string]any    `json:"data,omitempty"`
}

// Logger provides a ring buffer-based logging system.
type Logger struct {
	mu       sync.RWMutex
	entries  []Entry
	maxSize  int
	head     int // next write position
	count    int // number of entries (up to maxSize)
	minLevel Level
}

const (
	DefaultMaxEntries = 1000
	DefaultMinLevel   = LevelDebug
)

var (
	globalLogger *Logger
	once         sync.Once
)

// Init initializes the global logger. Safe to call multiple times.
func Init(maxEntries int, minLevel Level) {
	once.Do(func() {
		if maxEntries <= 0 {
			maxEntries = DefaultMaxEntries
		}
		globalLogger = &Logger{
			entries:  make([]Entry, maxEntries),
			maxSize:  maxEntries,
			minLevel: minLevel,
		}
	})
}

// Get returns the global logger instance, initializing with defaults if needed.
func Get() *Logger {
	if globalLogger == nil {
		Init(DefaultMaxEntries, DefaultMinLevel)
	}
	return globalLogger
}

// SetMinLevel changes the minimum log level.
func (l *Logger) SetMinLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// Log adds an entry to the ring buffer.
func (l *Logger) Log(level Level, category Category, message string, data map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.minLevel {
		return
	}

	entry := Entry{
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Message:   message,
		Data:      data,
	}

	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.maxSize
	if l.count < l.maxSize {
		l.count++
	}
}

// Convenience methods for different log levels

func (l *Logger) Debug(category Category, message string, data map[string]any) {
	l.Log(LevelDebug, category, message, data)
}

func (l *Logger) Info(category Category, message string, data map[string]any) {
	l.Log(LevelInfo, category, message, data)
}

func (l *Logger) Warn(category Category, message string, data map[string]any) {
	l.Log(LevelWarn, category, message, data)
}

func (l *Logger) Error(category Category, message string, data map[string]any) {
	l.Log(LevelError, category, message, data)
}

// GetEntries returns log entries, newest first.
// If limit is 0, returns all entries.
// If minLevel is specified, filters entries below that level.
func (l *Logger) GetEntries(limit int, minLevel *Level, category *Category) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.count == 0 {
		return []Entry{}
	}

	// Collect entries in reverse chronological order
	result := make([]Entry, 0, l.count)

	for i := 0; i < l.count; i++ {
		// Start from the most recent entry (head - 1) and go backwards
		idx := (l.head - 1 - i + l.maxSize) % l.maxSize
		entry := l.entries[idx]

		// Apply filters
		if minLevel != nil && entry.Level < *minLevel {
			continue
		}
		if category != nil && entry.Category != *category {
			continue
		}

		result = append(result, entry)

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

// Clear removes all log entries.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.head = 0
	l.count = 0
}

// Stats returns logging statistics.
type Stats struct {
	TotalEntries int   `json:"total_entries"`
	MaxEntries   int   `json:"max_entries"`
	MinLevel     Level `json:"min_level"`
}

func (l *Logger) Stats() Stats {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return Stats{
		TotalEntries: l.count,
		MaxEntries:   l.maxSize,
		MinLevel:     l.minLevel,
	}
}

// Package-level convenience functions using the global logger

func Debug(category Category, message string, data map[string]any) {
	Get().Debug(category, message, data)
}

func Info(category Category, message string, data map[string]any) {
	Get().Info(category, message, data)
}

func Warn(category Category, message string, data map[string]any) {
	Get().Warn(category, message, data)
}

func Error(category Category, message string, data map[string]any) {
	Get().Error(category, message, data)
}

// Debugf logs a formatted debug message.
func Debugf(category Category, format string, args ...any) {
	Get().Debug(category, fmt.Sprintf(format, args...), nil)
}

// Infof logs a formatted info message.
func Infof(category Category, format string, args ...any) {
	Get().Info(category, fmt.Sprintf(format, args...), nil)
}

// Warnf logs a formatted warning message.
func Warnf(category Category, format string, args ...any) {
	Get().Warn(category, fmt.Sprintf(format, args...), nil)
}

// Errorf logs a formatted error message.
func Errorf(category Category, format string, args ...any) {
	Get().Error(category, fmt.Sprintf(format, args...), nil)
}
