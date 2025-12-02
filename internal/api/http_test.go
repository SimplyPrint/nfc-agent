package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleVersion(t *testing.T) {
	// Save original values
	origVersion := Version
	origBuildTime := BuildTime
	origGitCommit := GitCommit

	// Set test values
	Version = "1.2.3-test"
	BuildTime = "2024-01-15T10:30:00Z"
	GitCommit = "abc1234"

	// Restore after test
	defer func() {
		Version = origVersion
		BuildTime = origBuildTime
		GitCommit = origGitCommit
	}()

	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	w := httptest.NewRecorder()

	handleVersion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["version"] != "1.2.3-test" {
		t.Errorf("expected version '1.2.3-test', got '%s'", result["version"])
	}
	if result["buildTime"] != "2024-01-15T10:30:00Z" {
		t.Errorf("expected buildTime '2024-01-15T10:30:00Z', got '%s'", result["buildTime"])
	}
	if result["gitCommit"] != "abc1234" {
		t.Errorf("expected gitCommit 'abc1234', got '%s'", result["gitCommit"])
	}
}

func TestHandleVersion_MethodNotAllowed(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/version", nil)
			w := httptest.NewRecorder()

			handleVersion(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d for %s, got %d", http.StatusMethodNotAllowed, method, w.Code)
			}
		})
	}
}

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", result["status"])
	}

	// readerCount should be a number (even if 0 when no readers connected)
	if _, ok := result["readerCount"].(float64); !ok {
		t.Errorf("expected readerCount to be a number, got %T", result["readerCount"])
	}
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/health", nil)
			w := httptest.NewRecorder()

			handleHealth(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d for %s, got %d", http.StatusMethodNotAllowed, method, w.Code)
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		checkCORS      bool
	}{
		{"GET request", http.MethodGet, http.StatusOK, true},
		{"POST request", http.MethodPost, http.StatusOK, true},
		{"PUT request", http.MethodPut, http.StatusOK, true},
		{"DELETE request", http.MethodDelete, http.StatusOK, true},
		{"OPTIONS preflight", http.MethodOptions, http.StatusOK, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.checkCORS {
				if w.Header().Get("Access-Control-Allow-Origin") != "*" {
					t.Error("expected Access-Control-Allow-Origin header to be '*'")
				}
				if w.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
					t.Error("expected Access-Control-Allow-Methods header")
				}
				if w.Header().Get("Access-Control-Allow-Headers") != "Content-Type" {
					t.Error("expected Access-Control-Allow-Headers header")
				}
			}
		})
	}
}

func TestCORSMiddleware_PreflightResponse(t *testing.T) {
	handler := corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// This should not be called for OPTIONS
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Handler called"))
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// OPTIONS should return 200, not 201 from the inner handler
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d for OPTIONS, got %d", http.StatusOK, w.Code)
	}

	// Body should be empty for preflight
	if w.Body.Len() > 0 {
		t.Errorf("expected empty body for OPTIONS preflight, got %s", w.Body.String())
	}
}

func TestRespondJSON(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		data       interface{}
		expectJSON bool
	}{
		{
			name:       "simple map",
			status:     http.StatusOK,
			data:       map[string]string{"message": "hello"},
			expectJSON: true,
		},
		{
			name:       "created status",
			status:     http.StatusCreated,
			data:       map[string]string{"id": "123"},
			expectJSON: true,
		},
		{
			name:       "error response",
			status:     http.StatusBadRequest,
			data:       map[string]string{"error": "invalid input"},
			expectJSON: true,
		},
		{
			name:       "complex struct",
			status:     http.StatusOK,
			data:       map[string]interface{}{"count": 42, "items": []string{"a", "b"}},
			expectJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			respondJSON(w, tt.status, tt.data)

			if w.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, w.Code)
			}

			if w.Header().Get("Content-Type") != "application/json" {
				t.Error("expected Content-Type to be application/json")
			}

			if tt.expectJSON {
				var result interface{}
				if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode JSON response: %v", err)
				}
			}
		})
	}
}

func TestNewMux(t *testing.T) {
	mux := NewMux()

	// Test that routes are registered
	routes := []string{
		"/v1/readers",
		"/v1/version",
		"/v1/health",
		"/v1/supported-readers",
	}

	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// Should not be 404
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s not registered", route)
		}
	}
}

func TestNewMux_RootServesWebUI(t *testing.T) {
	mux := NewMux()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Root should serve the web UI (not 404)
	if w.Code == http.StatusNotFound {
		t.Error("root route should serve web UI, got 404")
	}
}

func TestHandleListReaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/readers", nil)
	w := httptest.NewRecorder()

	handleListReaders(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type to be application/json")
	}

	// Response should be valid JSON array (even if empty)
	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestHandleListReaders_MethodNotAllowed(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/readers", nil)
			w := httptest.NewRecorder()

			handleListReaders(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d for %s, got %d", http.StatusMethodNotAllowed, method, w.Code)
			}
		})
	}
}

func TestHandleSupportedReaders(t *testing.T) {
	mux := NewMux()

	req := httptest.NewRequest(http.MethodGet, "/v1/supported-readers", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have a "readers" key
	if _, ok := result["readers"]; !ok {
		t.Error("response should contain 'readers' key")
	}
}

func TestHandleReaderRoutes_InvalidPath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{"missing index", "/v1/readers/", http.StatusBadRequest},
		{"invalid index", "/v1/readers/abc/card", http.StatusBadRequest},
		{"negative index", "/v1/readers/-1/card", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handleReaderRoutes(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestHandleReaderRoutes_UnknownEndpoint(t *testing.T) {
	// First we need readers to be available, but since we can't mock core.ListReaders,
	// this test will return "no readers found" or "reader index out of range"
	req := httptest.NewRequest(http.MethodGet, "/v1/readers/0/unknown", nil)
	w := httptest.NewRecorder()

	handleReaderRoutes(w, req)

	// Should return an error (either not found or bad request depending on reader availability)
	if w.Code == http.StatusOK {
		t.Error("unknown endpoint should not return 200 OK")
	}
}

func TestVersionVariables(t *testing.T) {
	// Test that version variables are initialized
	if Version == "" {
		t.Error("Version should have a default value")
	}

	// Save and restore
	origVersion := Version
	origBuildTime := BuildTime
	origGitCommit := GitCommit

	defer func() {
		Version = origVersion
		BuildTime = origBuildTime
		GitCommit = origGitCommit
	}()

	// Test modification
	Version = "test-version"
	BuildTime = "test-time"
	GitCommit = "test-commit"

	if Version != "test-version" {
		t.Errorf("Version should be modifiable, got %s", Version)
	}
	if BuildTime != "test-time" {
		t.Errorf("BuildTime should be modifiable, got %s", BuildTime)
	}
	if GitCommit != "test-commit" {
		t.Errorf("GitCommit should be modifiable, got %s", GitCommit)
	}
}

func TestHandleVersion_ContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	w := httptest.NewRecorder()

	handleVersion(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestHandleHealth_ContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// Test with mock card request body
func TestHandleReaderCard_WriteRequest_InvalidJSON(t *testing.T) {
	// Create a mock request with invalid JSON
	body := bytes.NewBufferString("{invalid json}")
	req := httptest.NewRequest(http.MethodPost, "/v1/readers/0/card", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// This will fail because there are no readers, but we're testing the flow
	handleReaderRoutes(w, req)

	// Should return an error
	if w.Code == http.StatusOK {
		t.Error("invalid JSON should not return 200 OK")
	}
}

// Benchmark tests
func BenchmarkHandleVersion(b *testing.B) {
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
		w := httptest.NewRecorder()
		handleVersion(w, req)
	}
}

func BenchmarkHandleHealth(b *testing.B) {
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
		w := httptest.NewRecorder()
		handleHealth(w, req)
	}
}

func BenchmarkCORSMiddleware(b *testing.B) {
	handler := corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		handler(w, req)
	}
}

func BenchmarkRespondJSON(b *testing.B) {
	data := map[string]interface{}{
		"key":    "value",
		"number": 42,
		"array":  []string{"a", "b", "c"},
	}

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		respondJSON(w, http.StatusOK, data)
	}
}

func BenchmarkNewMux(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewMux()
	}
}
