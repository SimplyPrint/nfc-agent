package updater

import (
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string // e.g., "dev", "beta", "rc1"
	Metadata   string // e.g., "abc1234" from "dev-abc1234"
}

// ParseVersion parses a version string like "v1.2.3", "1.2.3", "dev", "dev-abc1234"
func ParseVersion(s string) Version {
	// Remove leading 'v' if present
	s = strings.TrimPrefix(s, "v")

	// Handle dev versions
	if s == "dev" || strings.HasPrefix(s, "dev-") {
		parts := strings.SplitN(s, "-", 2)
		v := Version{Prerelease: "dev"}
		if len(parts) > 1 {
			v.Metadata = parts[1]
		}
		return v
	}

	// Parse semantic version: major.minor.patch[-prerelease][+metadata]
	var v Version
	re := regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([a-zA-Z0-9.-]+))?(?:\+([a-zA-Z0-9.-]+))?$`)
	matches := re.FindStringSubmatch(s)

	if matches == nil {
		// Invalid version, treat as unknown
		return Version{Prerelease: "unknown"}
	}

	v.Major, _ = strconv.Atoi(matches[1])
	if matches[2] != "" {
		v.Minor, _ = strconv.Atoi(matches[2])
	}
	if matches[3] != "" {
		v.Patch, _ = strconv.Atoi(matches[3])
	}
	v.Prerelease = matches[4]
	v.Metadata = matches[5]

	return v
}

// IsDev returns true if this is a development version
func (v Version) IsDev() bool {
	return v.Prerelease == "dev" || v.Prerelease == "unknown"
}

// Compare returns:
//
//	-1 if v < other
//	 0 if v == other
//	 1 if v > other
func (v Version) Compare(other Version) int {
	// Dev versions are always considered "older" than release versions
	// for update purposes (so users always see updates available)
	if v.IsDev() && !other.IsDev() {
		return -1
	}
	if !v.IsDev() && other.IsDev() {
		return 1
	}
	if v.IsDev() && other.IsDev() {
		return 0 // Can't compare dev versions
	}

	// Compare major.minor.patch
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	// Equal version numbers - compare prerelease
	// No prerelease > with prerelease (1.0.0 > 1.0.0-beta)
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}

	return 0
}

// IsOlderThan returns true if v is older than other (i.e., other is newer)
func (v Version) IsOlderThan(other Version) bool {
	return v.Compare(other) < 0
}

// String returns the version as a string
func (v Version) String() string {
	if v.IsDev() {
		if v.Metadata != "" {
			return "dev-" + v.Metadata
		}
		return "dev"
	}

	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	return s
}
