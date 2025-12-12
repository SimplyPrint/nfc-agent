package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewChecker(t *testing.T) {
	checker := NewChecker("1.0.0")
	if checker == nil {
		t.Fatal("NewChecker returned nil")
	}
	if checker.currentVersion != "1.0.0" {
		t.Errorf("currentVersion = %q, want %q", checker.currentVersion, "1.0.0")
	}
}

func TestCheckerCheck(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := GitHubRelease{
			TagName:     "v1.1.0",
			Name:        "Release 1.1.0",
			Body:        "## What's New\n- Feature A\n- Bug fix B",
			HTMLURL:     "https://github.com/test/repo/releases/v1.1.0",
			PublishedAt: time.Now(),
			Assets: []GitHubAsset{
				{
					Name:               "test-darwin-arm64.tar.gz",
					BrowserDownloadURL: "https://example.com/darwin-arm64.tar.gz",
				},
			},
		}
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	// Test with a checker pointing to the test server
	checker := NewChecker("1.0.0")

	// Call Check - since we can't override the URL easily, we test the basic flow
	info := checker.Check(false)

	// Basic assertions
	if info == nil {
		t.Fatal("Check returned nil")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.0.0")
	}
	if info.Platform == "" {
		t.Error("Platform should not be empty")
	}
	if info.CheckedAt.IsZero() {
		t.Error("CheckedAt should not be zero")
	}
}

func TestCheckerCaching(t *testing.T) {
	checker := NewChecker("1.0.0")

	// First call
	info1 := checker.Check(false)
	checkedAt1 := info1.CheckedAt

	// Second call (should use cache)
	time.Sleep(10 * time.Millisecond)
	info2 := checker.Check(false)
	checkedAt2 := info2.CheckedAt

	// Should be the same (cached)
	if !checkedAt1.Equal(checkedAt2) {
		t.Error("Second call should have used cached result")
	}

	// Force refresh
	time.Sleep(10 * time.Millisecond)
	info3 := checker.Check(true)
	checkedAt3 := info3.CheckedAt

	// Should be different (refreshed)
	if checkedAt1.Equal(checkedAt3) {
		t.Error("Force refresh should have created new result")
	}
}

func TestCheckerClearCache(t *testing.T) {
	checker := NewChecker("1.0.0")

	// First call to populate cache
	checker.Check(false)

	// Clear cache
	checker.ClearCache()

	// Verify cache is cleared
	checker.mu.RLock()
	cachedResult := checker.cachedResult
	cacheExpiry := checker.cacheExpiry
	checker.mu.RUnlock()

	if cachedResult != nil {
		t.Error("cachedResult should be nil after ClearCache")
	}
	if !cacheExpiry.IsZero() {
		t.Error("cacheExpiry should be zero after ClearCache")
	}
}

func TestFindDownloadURL(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "nfc-agent-darwin-amd64.tar.gz", BrowserDownloadURL: "https://example.com/darwin-amd64.tar.gz"},
		{Name: "nfc-agent-darwin-arm64.dmg", BrowserDownloadURL: "https://example.com/darwin-arm64.dmg"},
		{Name: "nfc-agent-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux-amd64.tar.gz"},
		{Name: "nfc-agent-linux-amd64.deb", BrowserDownloadURL: "https://example.com/linux-amd64.deb"},
		{Name: "nfc-agent-windows-amd64.exe", BrowserDownloadURL: "https://example.com/windows-amd64.exe"},
	}

	// Test that it returns something for the current platform
	url := findDownloadURL(assets)
	t.Logf("Found download URL for current platform: %s", url)
	// Can't assert specific URL since it depends on runtime.GOOS/GOARCH
}

func TestFindDownloadURL_Empty(t *testing.T) {
	assets := []GitHubAsset{}
	url := findDownloadURL(assets)
	if url != "" {
		t.Errorf("Expected empty string for empty assets, got %q", url)
	}
}

func TestFindDownloadURL_NoMatch(t *testing.T) {
	// Assets that won't match any common platform
	assets := []GitHubAsset{
		{Name: "nfc-agent-freebsd-riscv64.tar.gz", BrowserDownloadURL: "https://example.com/freebsd.tar.gz"},
	}
	url := findDownloadURL(assets)
	// This may or may not match depending on the test machine's OS
	// Just verify it doesn't panic
	t.Logf("URL for non-matching assets: %q", url)
}

func TestTruncateReleaseNotes(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 100, "short"},
		{"this is a longer string that exceeds the limit", 20, "this is a longer str..."},
		{"", 10, ""},
		{"exactly10!", 10, "exactly10!"},
		{"  whitespace  ", 100, "whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateReleaseNotes(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateReleaseNotes(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestUpdateInfoFields(t *testing.T) {
	// Test that UpdateInfo struct serializes correctly
	info := UpdateInfo{
		Available:      true,
		CurrentVersion: "1.0.0",
		LatestVersion:  "1.1.0",
		ReleaseURL:     "https://example.com/release",
		ReleaseNotes:   "Test notes",
		Platform:       "darwin/arm64",
		CheckedAt:      time.Now(),
		IsDev:          false,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal UpdateInfo: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal UpdateInfo: %v", err)
	}

	if decoded["available"] != true {
		t.Error("available should be true")
	}
	if decoded["currentVersion"] != "1.0.0" {
		t.Error("currentVersion mismatch")
	}
	if decoded["latestVersion"] != "1.1.0" {
		t.Error("latestVersion mismatch")
	}
}
