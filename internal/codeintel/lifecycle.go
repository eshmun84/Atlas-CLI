package codeintel

import (
	"fmt"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// DecideRefreshMode selects the mutating refresh mode from read-only evidence.
// forceFull forces RefreshModeFull when a build would run.
func DecideRefreshMode(graphPresent bool, forceFull bool) RefreshMode {
	if !graphPresent {
		if forceFull {
			return RefreshModeFull
		}
		return RefreshModeInitial
	}
	if forceFull {
		return RefreshModeFull
	}
	return RefreshModeIncremental
}

// EnrichSnapshot adds Atlas-owned freshness from metadata + source fingerprint.
// Read-only: never writes metadata, never creates Atlas Home, never builds.
func EnrichSnapshot(project Project, st ProjectStatus) Snapshot {
	snap := Snapshot{
		Applicable:      true,
		Provider:        st.Provider,
		State:           st.State,
		Version:         st.Version,
		Executable:      st.Executable,
		Message:         st.Message,
		StorageDir:      st.StorageDir,
		GraphPresent:    st.GraphPresent,
		MetadataPresent: st.MetadataPresent,
		GraphDBPath:     st.GraphDBPath,
		MetadataPath:    st.MetadataPath,
	}

	switch st.State {
	case StateUnavailable, StateIncompatible, StateError, StateMissing:
		return snap
	}

	if !st.GraphPresent {
		snap.Freshness = FreshnessMissing
		snap.FreshnessReason = "Atlas-owned graph.db not present"
		return snap
	}

	meta, present, metaErr := LoadMetadata(st.MetadataPath)
	snap.MetadataPresent = present
	if metaErr != nil {
		snap.MetadataValid = false
		snap.Freshness = FreshnessError
		snap.FreshnessReason = metaErr.Error()
		snap.Message = metaErr.Error()
		return snap
	}
	if !present {
		snap.MetadataValid = false
		snap.Freshness = FreshnessStale
		snap.FreshnessReason = "metadata.json missing"
		return snap
	}
	snap.MetadataValid = true
	snap.RefreshedAt = meta.RefreshedAt
	snap.RefreshMode = meta.RefreshMode

	fp, err := ComputeSourceFingerprint(project.Root)
	if err != nil {
		snap.Freshness = FreshnessError
		snap.FreshnessReason = err.Error()
		return snap
	}
	if subtleEqual(meta.SourceFingerprint, fp.Value) {
		snap.Freshness = FreshnessReady
		snap.FreshnessReason = "source fingerprint matches metadata"
		return snap
	}
	snap.Freshness = FreshnessStale
	snap.FreshnessReason = "source fingerprint changed since last refresh"
	return snap
}

// ResolveProject fills HomePath / ID when missing using read-only helpers.
// Does not create Atlas Home.
func ResolveProject(root, projectName string) (Project, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Project{}, fmt.Errorf("codeintel: project root is required")
	}
	homePath, err := home.Resolve()
	if err != nil {
		homePath = ""
	}
	id, err := home.ProjectID(root, projectName)
	if err != nil {
		return Project{}, err
	}
	return Project{Root: root, ID: id, HomePath: homePath}, nil
}

func graphExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func subtleEqual(a, b string) bool {
	return strings.TrimSpace(a) != "" && a == b
}
