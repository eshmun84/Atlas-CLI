package codegraph

import "strings"

// VersionArgs returns argv for obtaining the CodeGraph CLI version.
// Official form: codegraph --version
// Note: -v is --verbose upstream, not version.
func VersionArgs() []string {
	return []string{"--version"}
}

// WithDB appends the official custom DB flag for graph-aware commands.
// Official form: -d / --db <path>
// Empty dbPath leaves args unchanged (caller must not invent alternate env vars).
func WithDB(args []string, dbPath string) []string {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return append([]string(nil), args...)
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args...)
	out = append(out, "--db", dbPath)
	return out
}

// WithJSON appends the official machine-readable JSON flag (-j / --json).
func WithJSON(args []string) []string {
	out := make([]string, 0, len(args)+1)
	out = append(out, args...)
	out = append(out, "--json")
	return out
}

// BuildArgs prepares a future explicit graph build against the Atlas-owned DB.
// Not executed by Probe/Status.
func BuildArgs(projectDir, dbPath string) []string {
	args := []string{"build"}
	if dir := strings.TrimSpace(projectDir); dir != "" {
		args = append(args, dir)
	}
	return WithDB(args, dbPath)
}

// StatsArgs prepares a future read-oriented stats invocation against the
// Atlas-owned DB. Not executed by Probe/Status in Slice 31.
func StatsArgs(dbPath string) []string {
	return WithJSON(WithDB([]string{"stats"}, dbPath))
}

// QueryArgs prepares a future symbol query against the Atlas-owned DB.
// Not executed by Probe/Status.
func QueryArgs(dbPath string, queryParts ...string) []string {
	args := []string{"query"}
	args = append(args, queryParts...)
	return WithJSON(WithDB(args, dbPath))
}
