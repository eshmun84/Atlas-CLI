package codegraph

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExecutableName is the CodeGraph CLI binary discovered on PATH.
const ExecutableName = "codegraph"

// Compatibility baseline for the CodeGraph adapter (@optave/codegraph).
//
// Upstream CLI surface this adapter targets (verified against v3.x, e.g. 3.17.0):
//   - codegraph --version
//   - codegraph build [dir] --db <path>
//   - codegraph stats --db <path> --json
//   - codegraph query … --db <path> --json
//
// Flag support is command-specific, not universal:
//   - --db is used with build, stats, and query;
//   - --json is used with stats and query (not a general/common flag for all
//     CodeGraph subcommands; build does not use --json in this contract).
//
// The current documented release line is v3.x (e.g. upstream notes around v3.17).
// Slice 31 therefore accepts only major == SupportedMajor. Older majors are
// treated as incompatible because their CLI contracts were not verified.
// Newer majors are also incompatible until Atlas explicitly widens this gate.
//
// This policy lives only inside the CodeGraph adapter package.
const SupportedMajor = 3

// ProbedCapabilityVersion is the only capability Probe actually verifies at
// runtime in Slice 31 (via codegraph --version).
const ProbedCapabilityVersion = "version"

// V3Contract is the documented @optave/codegraph major-3 CLI surface this
// adapter is written against. It is NOT runtime-probed evidence and must not
// be copied into Capability.Capabilities.
// Entries "--db" / "--json" name flags that appear in that surface; they are
// not claimed as universal across every CodeGraph subcommand.
var V3Contract = []string{
	"build",  // codegraph build [dir] --db <path>
	"stats",  // codegraph stats --db <path> --json
	"query",  // codegraph query … --db <path> --json
	"--db",   // on build, stats, and query
	"--json", // on stats and query (not build)
}

var versionToken = regexp.MustCompile(`(?i)\bv?(\d+)\.(\d+)\.(\d+)\b`)

type semver struct {
	major, minor, patch int
}

// ParseVersion extracts a semantic version from CodeGraph --version output.
func ParseVersion(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("codegraph: empty version output")
	}
	m := versionToken.FindStringSubmatch(raw)
	if m == nil {
		return "", fmt.Errorf("codegraph: cannot parse version from %q", raw)
	}
	return fmt.Sprintf("%s.%s.%s", m[1], m[2], m[3]), nil
}

// IsCompatible reports whether version matches the adapter's supported major.
func IsCompatible(version string) bool {
	got, err := parseSemver(version)
	if err != nil {
		return false
	}
	return got.major == SupportedMajor
}

// CompatibilityMessage explains why a version is accepted or rejected.
func CompatibilityMessage(version string) string {
	got, err := parseSemver(version)
	if err != nil {
		return "unparseable CodeGraph version"
	}
	if got.major == SupportedMajor {
		return fmt.Sprintf("CodeGraph %s matches Atlas adapter major %d baseline", version, SupportedMajor)
	}
	return fmt.Sprintf("CodeGraph %s is outside Atlas adapter major %d baseline", version, SupportedMajor)
}

func parseSemver(v string) (semver, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return semver{}, fmt.Errorf("invalid semver %q", v)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, err
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, err
	}
	patchPart := parts[2]
	if i := strings.IndexAny(patchPart, "-+"); i >= 0 {
		patchPart = patchPart[:i]
	}
	patch, err := strconv.Atoi(patchPart)
	if err != nil {
		return semver{}, err
	}
	return semver{major: major, minor: minor, patch: patch}, nil
}
