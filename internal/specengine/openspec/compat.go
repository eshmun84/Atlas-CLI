package openspec

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// ExecutableName is the OpenSpec CLI binary name on PATH.
	ExecutableName = "openspec"

	// MinSupportedVersion is the Atlas compatibility floor (inclusive).
	MinSupportedVersion = "1.14.0"

	// MaxSupportedMajor excludes this major and above until reviewed.
	MaxSupportedMajor = 2
)

// ParsedVersion is a semantic version used for OpenSpec compatibility and pins.
// Pre holds prerelease identifiers without a leading '-'. Build metadata is not
// preserved (stripped on parse) and never used for exact pin matching.
type ParsedVersion struct {
	Major int
	Minor int
	Patch int
	Pre   string // e.g. "beta.1"; empty for stable releases
	Raw   string // trimmed original input
}

// String returns major.minor.patch[-pre] (no leading v; build omitted).
func (v ParsedVersion) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		return s + "-" + v.Pre
	}
	return s
}

// ParseVersion parses "1.14.2", "v1.14.2", or "1.14.2-beta.1".
// Build metadata (+...) is stripped. Prerelease is preserved for exact pins.
func ParseVersion(raw string) (ParsedVersion, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if s == "" {
		return ParsedVersion{}, fmt.Errorf("openspec: empty version")
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core := s
	pre := ""
	if i := strings.IndexByte(core, '-'); i >= 0 {
		pre = core[i+1:]
		core = core[:i]
		if pre == "" {
			return ParsedVersion{}, fmt.Errorf("openspec: invalid version %q", raw)
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return ParsedVersion{}, fmt.Errorf("openspec: invalid version %q", raw)
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil || maj < 0 {
		return ParsedVersion{}, fmt.Errorf("openspec: invalid version %q", raw)
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil || min < 0 {
		return ParsedVersion{}, fmt.Errorf("openspec: invalid version %q", raw)
	}
	patch := 0
	if len(parts) == 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil || patch < 0 {
			return ParsedVersion{}, fmt.Errorf("openspec: invalid version %q", raw)
		}
	}
	return ParsedVersion{
		Major: maj,
		Minor: min,
		Patch: patch,
		Pre:   pre,
		Raw:   strings.TrimSpace(raw),
	}, nil
}

// Compare returns -1 if a<b, 0 if numeric cores equal, 1 if a>b.
// Prerelease is ignored here; use ExactMatch for project pins.
func Compare(a, b ParsedVersion) int {
	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// AtlasCompatible reports whether v is inside Atlas's supported window:
// >= 1.14.0 and major < 2. Prerelease at the floor (e.g. 1.14.0-rc.1) is rejected.
func AtlasCompatible(v ParsedVersion) bool {
	min, err := ParseVersion(MinSupportedVersion)
	if err != nil {
		return false
	}
	if v.Major >= MaxSupportedMajor {
		return false
	}
	cmp := Compare(v, min)
	if cmp < 0 {
		return false
	}
	if cmp == 0 && v.Pre != "" {
		return false
	}
	return true
}

// ExactMatch reports whether installed equals the project pin, including prerelease.
// Stable and prerelease with the same numeric core are NOT equivalent.
func ExactMatch(installed, pin ParsedVersion) bool {
	return Compare(installed, pin) == 0 && installed.Pre == pin.Pre
}
