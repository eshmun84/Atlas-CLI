package context

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// StatusSnapshot is a read-only Context Economy report for Status/Doctor.
type StatusSnapshot struct {
	Applicable     bool
	Configured     bool
	ProjectID      string
	HomePath       string
	ProjectDir     string
	IndexPath      string
	CapsulePath    string
	IndexPresent   bool
	CapsulePresent bool
	State          string // n/a | missing | present | stale | unreadable
	IndexedAt      string
	Fingerprint    string
	Message        string
}

// InspectInput carries read-only project facts for inspection.
type InspectInput struct {
	Root           string
	Initialized    bool
	ProjectName    string
	StateProjectID string
	StateUpdatedAt string
}

// Inspect reports Context Economy state without creating or mutating files.
func Inspect(in InspectInput) StatusSnapshot {
	snap := StatusSnapshot{
		State: StatusNA,
	}
	if !in.Initialized {
		snap.Message = "project not initialized; Context Economy inactive"
		return snap
	}
	snap.Applicable = true
	snap.Configured = true

	homePath, err := ResolveHome()
	if err != nil || strings.TrimSpace(homePath) == "" {
		snap.State = StatusUnreadable
		snap.Message = "Atlas Home path unresolved"
		return snap
	}
	snap.HomePath = homePath

	name := strings.TrimSpace(in.ProjectName)
	id := strings.TrimSpace(in.StateProjectID)
	if id == "" {
		computed, idErr := ProjectID(in.Root, name)
		if idErr != nil {
			snap.State = StatusUnreadable
			snap.Message = "project id unresolved"
			return snap
		}
		id = computed
	}
	snap.ProjectID = id
	snap.ProjectDir = ProjectDir(homePath, id)
	snap.IndexPath = IndexPath(homePath, id)
	snap.CapsulePath = CapsulePath(homePath, id)

	indexInfo, indexErr := os.Stat(snap.IndexPath)
	capsuleInfo, capsuleErr := os.Stat(snap.CapsulePath)
	snap.IndexPresent = indexErr == nil && !indexInfo.IsDir()
	snap.CapsulePresent = capsuleErr == nil && !capsuleInfo.IsDir()

	if !snap.IndexPresent {
		snap.State = StatusMissing
		snap.Message = "context index missing under Atlas Home"
		return snap
	}

	idx, loadErr := LoadIndex(snap.IndexPath)
	if loadErr != nil {
		snap.State = StatusUnreadable
		snap.Message = "context index unreadable"
		return snap
	}
	snap.IndexedAt = idx.IndexedAt
	snap.Fingerprint = idx.Fingerprint

	if !snap.CapsulePresent {
		snap.State = StatusStale
		snap.Message = "index present but capsule missing"
		return snap
	}

	stale, staleErr := IsStale(in.Root, idx)
	if staleErr != nil {
		snap.State = StatusUnreadable
		snap.Message = "unable to evaluate freshness"
		return snap
	}
	if stale {
		snap.State = StatusStale
		snap.Message = DescribeStale(true)
		return snap
	}
	snap.State = StatusPresent
	snap.Message = "index and capsule present and fresh"
	if in.StateUpdatedAt != "" {
		snap.Message += " (state ref " + in.StateUpdatedAt + ")"
	}
	return snap
}

// InspectFromState builds InspectInput helpers from config state + root.
func InspectFromState(root string, initialized bool, state config.StateDocument) StatusSnapshot {
	name := state.ProjectName
	return Inspect(InspectInput{
		Root:           root,
		Initialized:    initialized,
		ProjectName:    name,
		StateProjectID: state.ContextEconomyProjectID,
		StateUpdatedAt: state.ContextEconomyUpdatedAt,
	})
}

// RelHomePath returns a display path under Atlas Home when possible.
func RelHomePath(homePath, full string) string {
	rel, err := filepath.Rel(homePath, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(rel)
}
