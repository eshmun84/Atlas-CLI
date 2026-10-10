package openspec

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/sdd"
)

// archiveNamePattern matches OpenSpec archive folder naming: YYYY-MM-DD-<change-id>
var archiveNamePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)$`)

func enginePresence(issues []string) sdd.Presence {
	return sdd.Presence{
		Engine:  sdd.EngineID(engineID),
		Present: true,
		Label:   engineLabel,
		Issues:  issues,
	}
}

func emptyPresence() sdd.Presence {
	return sdd.Presence{
		Engine:  sdd.EngineID(engineID),
		Present: false,
		Label:   engineLabel,
	}
}

func baseChange(projectID, changeID, location string, archived bool) sdd.ChangeRef {
	lifecycle := sdd.LifecycleActive
	if archived {
		lifecycle = sdd.LifecycleArchived
	}
	return sdd.ChangeRef{
		ProjectID:    projectID,
		Engine:       sdd.EngineID(engineID),
		ChangeID:     changeID,
		Lifecycle:    lifecycle,
		Location:     location,
		Archived:     archived,
		Verification: sdd.VerificationUnknown,
	}
}

// mapActiveLifecycle derives lifecycle from optional artifacts fail-closed.
//
// ready = required artifacts present + required tasks complete → looks ready
// for verification. This never maps to verified and never implies
// VerificationPass.
func mapActiveLifecycle(hasProposal, hasTasks, tasksComplete, hasAmbiguity bool) sdd.LifecycleState {
	if hasAmbiguity {
		return sdd.LifecycleUnknown
	}
	if !hasProposal && !hasTasks {
		// Directory exists but no recognizable change artifacts.
		return sdd.LifecycleIncomplete
	}
	if hasProposal && hasTasks && tasksComplete {
		// Ready for verification — not verified.
		return sdd.LifecycleReady
	}
	return sdd.LifecycleActive
}

func parseArchiveName(name string) (changeID string, archivedAt *time.Time) {
	name = strings.TrimSpace(name)
	m := archiveNamePattern.FindStringSubmatch(name)
	if m == nil {
		return name, nil
	}
	t, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return m[2], nil
	}
	utc := t.UTC()
	return m[2], &utc
}

func locationActive(changeID string) string {
	return filepath.ToSlash(filepath.Join(changesDir, changeID))
}

func locationArchived(dirName string) string {
	return filepath.ToSlash(filepath.Join(archiveDir, dirName))
}

func normalizeChangeID(name string) string {
	return strings.TrimSpace(name)
}

// validChangeID refuses path separators and traversal tokens in change IDs.
func validChangeID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return false
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) || strings.Contains(id, "..") {
		return false
	}
	if strings.Contains(id, string(filepath.Separator)) {
		return false
	}
	return true
}
