package codeintel

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// RefreshOptions configures an explicit mutating Code Intelligence refresh.
type RefreshOptions struct {
	Root        string
	ProjectName string
	ProjectID   string
	HomePath    string
	ForceFull   bool
	Now         func() time.Time
}

// RefreshOutcome is the result of Service.Refresh.
type RefreshOutcome struct {
	Noop         bool
	Blocked      bool
	Blockers     []string
	Mode         RefreshMode
	State        State
	Freshness    Freshness
	Fingerprint  string
	Metadata     Metadata
	GraphDBPath  string
	MetadataPath string
	StorageDir   string
	Warnings     []string
	Message      string
	Containment  ContainmentReport
	Provider     ProviderID
	Version      string
}

// Refresh applies Atlas lifecycle policy then optionally mutates via Provider.Refresh.
func (s *Service) Refresh(ctx context.Context, opts RefreshOptions) (RefreshOutcome, error) {
	out := RefreshOutcome{
		Blockers: []string{},
		Warnings: []string{},
	}
	if s == nil {
		out.Blocked = true
		out.Blockers = append(out.Blockers, "codeintel service unavailable")
		return out, fmt.Errorf("codeintel: service unavailable")
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		out.Blocked = true
		out.Blockers = append(out.Blockers, "project root is required")
		return out, fmt.Errorf("codeintel: project root is required")
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	started := nowFn().UTC()

	homePath := strings.TrimSpace(opts.HomePath)
	if homePath == "" {
		var err error
		homePath, err = home.Resolve()
		if err != nil {
			out.Blocked = true
			out.Blockers = append(out.Blockers, "Atlas Home unresolved: "+err.Error())
			return out, err
		}
	}
	projectID := strings.TrimSpace(opts.ProjectID)
	if projectID == "" {
		var err error
		projectID, err = home.ProjectID(root, opts.ProjectName)
		if err != nil {
			out.Blocked = true
			out.Blockers = append(out.Blockers, err.Error())
			return out, err
		}
	}
	project := Project{Root: root, ID: projectID, HomePath: homePath}
	out.StorageDir = ProviderStorageDir(homePath, projectID, s.DefaultID())
	out.GraphDBPath = GraphDBPath(homePath, projectID, s.DefaultID())
	out.MetadataPath = MetadataPath(homePath, projectID, s.DefaultID())

	id := s.DefaultID()
	cap, err := s.Probe(ctx, id)
	out.Provider = cap.Provider
	out.Version = cap.Version
	out.State = cap.State
	if err != nil && cap.State == "" {
		out.State = StateError
		out.Message = err.Error()
		return out, err
	}
	switch cap.State {
	case StateUnavailable, StateMissing:
		out.State = StateUnavailable
		out.Message = cap.Message
		out.Blocked = true
		out.Blockers = append(out.Blockers, "provider unavailable")
		return out, nil
	case StateIncompatible:
		out.State = StateIncompatible
		out.Message = cap.Message
		out.Blocked = true
		out.Blockers = append(out.Blockers, "provider incompatible")
		return out, nil
	case StateError:
		out.Blocked = true
		out.Blockers = append(out.Blockers, cap.Message)
		out.Message = cap.Message
		return out, fmt.Errorf("codeintel: provider error: %s", cap.Message)
	}

	fp, err := ComputeSourceFingerprint(root)
	if err != nil {
		out.State = StateError
		out.Message = err.Error()
		return out, err
	}
	out.Fingerprint = fp.Value

	graphPresent := graphExists(out.GraphDBPath)
	meta, metaPresent, metaErr := LoadMetadata(out.MetadataPath)
	if metaErr == nil && metaPresent && graphPresent && !opts.ForceFull &&
		subtleEqual(meta.SourceFingerprint, fp.Value) {
		out.Noop = true
		out.Mode = RefreshModeNoop
		out.Freshness = FreshnessReady
		out.Metadata = meta
		out.Message = "graph already fresh; no refresh required"
		return out, nil
	}

	mode := DecideRefreshMode(graphPresent, opts.ForceFull)
	out.Mode = mode
	providerMode := mode
	if mode == RefreshModeInitial {
		providerMode = RefreshModeIncremental // CodeGraph default build is incremental/full-enough for empty DB
	}

	p, err := s.provider(id)
	if err != nil {
		out.Blocked = true
		out.Blockers = append(out.Blockers, err.Error())
		return out, err
	}

	if err := os.MkdirAll(out.StorageDir, 0o755); err != nil {
		out.State = StateError
		out.Message = err.Error()
		return out, fmt.Errorf("codeintel: create storage: %w", err)
	}

	beforeSide := snapshotCodegraphSideEffects(root)
	prevMetaPresent := metaPresent

	result, refreshErr := p.Refresh(ctx, RefreshRequest{
		Root:   root,
		DBPath: out.GraphDBPath,
		Mode:   providerMode,
	})
	if refreshErr != nil {
		out.State = StateError
		out.Message = refreshErr.Error()
		out.Containment = containCodegraphSideEffects(root, beforeSide, started)
		out.Warnings = append(out.Warnings, out.Containment.Warnings...)
		// Preserve previous metadata.
		if prevMetaPresent {
			out.Metadata = meta
		}
		return out, refreshErr
	}

	if !graphExists(out.GraphDBPath) {
		out.State = StateError
		out.Message = "provider refresh succeeded but graph.db is missing"
		out.Containment = containCodegraphSideEffects(root, beforeSide, started)
		out.Warnings = append(out.Warnings, out.Containment.Warnings...)
		return out, fmt.Errorf("codeintel: %s", out.Message)
	}

	// Post-refresh fingerprint (source may be unchanged; recompute for metadata).
	fpAfter, err := ComputeSourceFingerprint(root)
	if err != nil {
		out.State = StateError
		out.Message = err.Error()
		out.Containment = containCodegraphSideEffects(root, beforeSide, started)
		out.Warnings = append(out.Warnings, out.Containment.Warnings...)
		return out, err
	}

	out.Containment = containCodegraphSideEffects(root, beforeSide, started)
	out.Warnings = append(out.Warnings, out.Containment.Warnings...)

	rootIdentity, _ := home.RootFingerprint(root)
	newMeta := Metadata{
		SchemaVersion:       MetadataSchemaVersion,
		Provider:            string(cap.Provider),
		ProviderVersion:     cap.Version,
		ProjectID:           projectID,
		ProjectRootIdentity: rootIdentity,
		GraphDBPath:         out.GraphDBPath,
		RefreshedAt:         started.Format(time.RFC3339),
		RefreshMode:         string(mode),
		SourceFingerprint:   fpAfter.Value,
		NodesTotal:          result.NodesTotal,
		FilesTotal:          result.FilesTotal,
	}
	if err := WriteMetadataAtomic(out.MetadataPath, newMeta); err != nil {
		out.State = StateError
		out.Message = err.Error()
		if prevMetaPresent {
			out.Metadata = meta
		}
		return out, err
	}

	out.Metadata = newMeta
	out.Fingerprint = fpAfter.Value
	out.Freshness = FreshnessReady
	out.State = StateAvailable
	out.Message = fmt.Sprintf("refresh %s complete", mode)
	if result.Message != "" {
		out.Message = result.Message
	}
	_ = project
	return out, nil
}
