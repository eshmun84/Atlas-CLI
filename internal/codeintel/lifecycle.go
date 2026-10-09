package codeintel

import (
	"strings"
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

	if strings.TrimSpace(st.GraphDBPath) != "" {
		if project.HomePath == "" {
			snap.GraphPresent = false
			snap.State = StateError
			snap.Freshness = FreshnessError
			snap.FreshnessReason = "Atlas Home path required for storage inspection"
			snap.Message = snap.FreshnessReason
			return snap
		}
		leaf, err := InspectStorageLeaf(project.HomePath, st.GraphDBPath)
		if err != nil {
			snap.GraphPresent = false
			snap.State = StateError
			snap.Freshness = FreshnessError
			snap.FreshnessReason = err.Error()
			snap.Message = err.Error()
			return snap
		}
		snap.GraphPresent = leaf.Present
	}

	if !snap.GraphPresent {
		snap.Freshness = FreshnessMissing
		snap.FreshnessReason = "Atlas-owned graph.db not present"
		return snap
	}

	meta, present, metaErr := LoadMetadata(project.HomePath, st.MetadataPath)
	snap.MetadataPresent = present
	if metaErr != nil {
		snap.MetadataValid = false
		snap.Freshness = FreshnessError
		snap.FreshnessReason = metaErr.Error()
		snap.Message = metaErr.Error()
		snap.State = StateError
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

func graphPresentSafe(homePath, path string) (bool, error) {
	leaf, err := InspectStorageLeaf(homePath, path)
	if err != nil {
		return false, err
	}
	return leaf.Present, nil
}

func subtleEqual(a, b string) bool {
	return strings.TrimSpace(a) != "" && a == b
}
