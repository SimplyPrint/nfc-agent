package updater

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected Version
	}{
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
		{"1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
		{"1.0.0", Version{Major: 1, Minor: 0, Patch: 0}},
		{"v2.0.0-beta", Version{Major: 2, Minor: 0, Patch: 0, Prerelease: "beta"}},
		{"1.0.0-rc1", Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "rc1"}},
		{"dev", Version{Prerelease: "dev"}},
		{"dev-abc1234", Version{Prerelease: "dev", Metadata: "abc1234"}},
		{"dev-abc1234-dirty", Version{Prerelease: "dev", Metadata: "abc1234-dirty"}},
		{"v0.1.0", Version{Major: 0, Minor: 1, Patch: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v := ParseVersion(tt.input)
			if v.Major != tt.expected.Major || v.Minor != tt.expected.Minor ||
				v.Patch != tt.expected.Patch || v.Prerelease != tt.expected.Prerelease ||
				v.Metadata != tt.expected.Metadata {
				t.Errorf("ParseVersion(%q) = %+v, want %+v", tt.input, v, tt.expected)
			}
		})
	}
}

func TestVersionIsDev(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"dev", true},
		{"dev-abc1234", true},
		{"1.0.0", false},
		{"v1.2.3", false},
		{"1.0.0-beta", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v := ParseVersion(tt.input)
			if v.IsDev() != tt.expected {
				t.Errorf("ParseVersion(%q).IsDev() = %v, want %v", tt.input, v.IsDev(), tt.expected)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.0.0", "1.0.0-beta", 1},
		{"1.0.0-beta", "1.0.0", -1},
		{"dev", "1.0.0", -1},
		{"1.0.0", "dev", 1},
		{"dev", "dev", 0},
		{"dev-abc", "dev-xyz", 0},
		{"v1.0.0", "1.0.0", 0},
		{"0.9.0", "1.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			v1 := ParseVersion(tt.v1)
			v2 := ParseVersion(tt.v2)
			result := v1.Compare(v2)
			if result != tt.expected {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestVersionIsOlderThan(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.1", "1.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"dev", "1.0.0", true},          // Note: dev is technically "older" but updater.go doesn't show updates for dev
		{"dev-abc1234", "1.0.0", true},  // Note: same as above
		{"1.0.0", "dev", false},
		{"0.9.0", "1.0.0", true},
		{"2.0.0", "1.9.9", false},
	}

	for _, tt := range tests {
		t.Run(tt.current+" < "+tt.latest, func(t *testing.T) {
			current := ParseVersion(tt.current)
			latest := ParseVersion(tt.latest)
			result := current.IsOlderThan(latest)
			if result != tt.expected {
				t.Errorf("IsOlderThan(%q, %q) = %v, want %v", tt.current, tt.latest, result, tt.expected)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"1.0.0-beta", "1.0.0-beta"},
		{"dev", "dev"},
		{"dev-abc1234", "dev-abc1234"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v := ParseVersion(tt.input)
			result := v.String()
			if result != tt.expected {
				t.Errorf("ParseVersion(%q).String() = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
