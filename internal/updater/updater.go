package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// GitHubAPIURL is the endpoint for fetching the latest release
	GitHubAPIURL = "https://api.github.com/repos/SimplyPrint/nfc-agent/releases/latest"
	// CacheDuration defines how long to cache update check results
	CacheDuration = 30 * time.Minute
	// RequestTimeout is the timeout for GitHub API requests
	RequestTimeout = 10 * time.Second
	// UserAgent identifies this client to GitHub
	UserAgent = "nfc-agent-updater"
	// MaxReleaseNotesLength is the maximum length of release notes to return
	MaxReleaseNotesLength = 500
)

// GitHubRelease represents the GitHub API response for a release
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a release asset (download file)
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Available      bool       `json:"available"`
	CurrentVersion string     `json:"currentVersion"`
	LatestVersion  string     `json:"latestVersion,omitempty"`
	ReleaseURL     string     `json:"releaseUrl,omitempty"`
	ReleaseNotes   string     `json:"releaseNotes,omitempty"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
	DownloadURL    string     `json:"downloadUrl,omitempty"`
	Platform       string     `json:"platform"`
	CheckedAt      time.Time  `json:"checkedAt"`
	Error          string     `json:"error,omitempty"`
	IsDev          bool       `json:"isDev"`
}

// Checker handles update checking with caching
type Checker struct {
	currentVersion string
	httpClient     *http.Client

	mu           sync.RWMutex
	cachedResult *UpdateInfo
	cacheExpiry  time.Time
}

// NewChecker creates a new update checker
func NewChecker(currentVersion string) *Checker {
	return &Checker{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout: RequestTimeout,
		},
	}
}

// Check checks for updates, using cache if available
func (c *Checker) Check(forceRefresh bool) *UpdateInfo {
	c.mu.RLock()
	if !forceRefresh && c.cachedResult != nil && time.Now().Before(c.cacheExpiry) {
		result := *c.cachedResult
		c.mu.RUnlock()
		return &result
	}
	c.mu.RUnlock()

	// Perform actual check
	result := c.checkGitHub()

	// Update cache
	c.mu.Lock()
	c.cachedResult = result
	c.cacheExpiry = time.Now().Add(CacheDuration)
	c.mu.Unlock()

	return result
}

// checkGitHub fetches the latest release from GitHub
func (c *Checker) checkGitHub() *UpdateInfo {
	info := &UpdateInfo{
		CurrentVersion: c.currentVersion,
		Platform:       runtime.GOOS + "/" + runtime.GOARCH,
		CheckedAt:      time.Now(),
		IsDev:          ParseVersion(c.currentVersion).IsDev(),
	}

	req, err := http.NewRequest(http.MethodGet, GitHubAPIURL, nil)
	if err != nil {
		info.Error = fmt.Sprintf("failed to create request: %v", err)
		return info
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		info.Error = fmt.Sprintf("failed to fetch release info: %v", err)
		return info
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		// Rate limited
		info.Error = "rate limited by GitHub API, try again later"
		return info
	}

	if resp.StatusCode == http.StatusNotFound {
		info.Error = "no releases found"
		return info
	}

	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("GitHub API returned status %d", resp.StatusCode)
		return info
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		info.Error = fmt.Sprintf("failed to parse release info: %v", err)
		return info
	}

	// Parse versions
	currentVer := ParseVersion(c.currentVersion)
	latestVer := ParseVersion(release.TagName)

	info.LatestVersion = release.TagName
	info.ReleaseURL = release.HTMLURL
	info.ReleaseNotes = truncateReleaseNotes(release.Body, MaxReleaseNotesLength)
	info.PublishedAt = &release.PublishedAt

	// Check if update is available
	// Dev builds should NOT show update available (they're typically ahead of releases)
	if currentVer.IsDev() {
		info.Available = false
	} else {
		info.Available = currentVer.IsOlderThan(latestVer)
	}

	// Find appropriate download for this platform
	info.DownloadURL = findDownloadURL(release.Assets)

	return info
}

// findDownloadURL finds the appropriate asset for the current platform
func findDownloadURL(assets []GitHubAsset) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go arch names to common naming conventions
	archNames := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64"},
		"arm64": {"arm64", "aarch64"},
		"386":   {"386", "i386", "x86"},
	}

	// Build list of acceptable arch patterns
	archPatterns := archNames[arch]
	if archPatterns == nil {
		archPatterns = []string{arch}
	}

	// Define preferred extensions by OS
	preferredExtensions := map[string][]string{
		"darwin":  {".dmg", ".pkg", ".tar.gz", ".zip"},
		"windows": {".exe", ".msi", ".zip"},
		"linux":   {".deb", ".rpm", ".tar.gz", ".zip"},
	}

	extensions := preferredExtensions[os]
	if extensions == nil {
		extensions = []string{".tar.gz", ".zip"}
	}

	// Score assets by preference
	type scoredAsset struct {
		url   string
		score int
	}
	var candidates []scoredAsset

	for _, asset := range assets {
		name := strings.ToLower(asset.Name)

		// Check OS match
		osMatch := false
		switch os {
		case "darwin":
			osMatch = strings.Contains(name, "darwin") || strings.Contains(name, "macos") || strings.Contains(name, "mac")
		case "windows":
			osMatch = strings.Contains(name, "windows") || strings.Contains(name, "win")
		case "linux":
			osMatch = strings.Contains(name, "linux")
		}

		if !osMatch {
			continue
		}

		// Check arch match
		archMatch := false
		for _, pattern := range archPatterns {
			if strings.Contains(name, pattern) {
				archMatch = true
				break
			}
		}

		// For macOS universal binaries, also accept if no specific arch is mentioned
		if os == "darwin" && !archMatch && strings.Contains(name, "universal") {
			archMatch = true
		}

		if !archMatch {
			continue
		}

		// Score by extension preference
		score := len(extensions) // Default score (lowest)
		for i, ext := range extensions {
			if strings.HasSuffix(name, ext) {
				score = i
				break
			}
		}

		candidates = append(candidates, scoredAsset{
			url:   asset.BrowserDownloadURL,
			score: score,
		})
	}

	// Return the best match
	if len(candidates) == 0 {
		return ""
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score < best.score {
			best = c
		}
	}

	return best.url
}

// truncateReleaseNotes truncates release notes to maxLen characters
func truncateReleaseNotes(notes string, maxLen int) string {
	notes = strings.TrimSpace(notes)
	if len(notes) <= maxLen {
		return notes
	}
	return notes[:maxLen] + "..."
}

// ClearCache clears the cached update info
func (c *Checker) ClearCache() {
	c.mu.Lock()
	c.cachedResult = nil
	c.cacheExpiry = time.Time{}
	c.mu.Unlock()
}
