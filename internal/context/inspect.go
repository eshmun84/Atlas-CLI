package context

import (
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
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
// Canonical/legacy index and capsule are inspected via symlink-safe Home reads.
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

	indexRel, capsuleRel, resolveErr := resolveContextRels(homePath, id)
	snap.IndexPath = filepath.Join(homePath, filepath.FromSlash(indexRel))
	snap.CapsulePath = filepath.Join(homePath, filepath.FromSlash(capsuleRel))
	snap.ProjectDir = filepath.Dir(snap.IndexPath)

	if resolveErr != nil {
		snap.State = StatusUnreadable
		snap.Message = "context index unreadable"
		return snap
	}

	indexPresent, indexErr := InspectContextLeaf(homePath, indexRel)
	if indexErr != nil {
		snap.State = StatusUnreadable
		snap.Message = "context index unreadable"
		return snap
	}
	snap.IndexPresent = indexPresent

	capsulePresent, capsuleErr := InspectContextLeaf(homePath, capsuleRel)
	if capsuleErr != nil {
		// External/symlink/non-regular capsule is never healthy.
		snap.State = StatusUnreadable
		snap.Message = "context capsule unreadable"
		return snap
	}
	snap.CapsulePresent = capsulePresent

	if !snap.IndexPresent {
		snap.State = StatusMissing
		snap.Message = "context index missing under Atlas Home"
		return snap
	}

	idx, present, loadErr := LoadIndexAt(homePath, id)
	if loadErr != nil {
		snap.State = StatusUnreadable
		snap.Message = "context index unreadable"
		return snap
	}
	if !present {
		snap.State = StatusMissing
		snap.Message = "context index missing under Atlas Home"
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
	return home.RelHomePath(homePath, full)
}
