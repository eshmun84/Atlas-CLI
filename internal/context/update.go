package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"gopkg.in/yaml.v3"
)

// Update action kinds.
const (
	UpdateActionCreate  = "create"
	UpdateActionReplace = "replace"
)

// UpdateTarget is one planned Context Economy write under Atlas Home.
type UpdateTarget struct {
	Path   string
	Action string
	Kind   string
	Reason string
}

// UpdatePlan is a read-only preview for the TUI Review → Apply flow.
type UpdatePlan struct {
	Blocked    bool
	Blockers   []string
	Warnings   []string
	ProjectID  string
	HomePath   string
	Objective  string
	Targets    []UpdateTarget
	Creates    []string
	Replaces   []string
	NeedsWrite bool
	Current    StatusSnapshot
}

// NeedsApply reports whether Apply would mutate Atlas Home / project state.
func (p UpdatePlan) NeedsApply() bool {
	return !p.Blocked && p.NeedsWrite && len(p.Targets) > 0
}

// Signature returns a stable hash of planned actions.
func (p UpdatePlan) Signature() string {
	targets := append([]UpdateTarget(nil), p.Targets...)
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Path != targets[j].Path {
			return targets[i].Path < targets[j].Path
		}
		return targets[i].Action < targets[j].Action
	})
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\n", p.ProjectID, p.HomePath, p.Objective)
	for _, t := range targets {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\n", t.Action, t.Path, t.Kind, t.Reason)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// UpdateResult is the outcome of an explicit Apply.
type UpdateResult struct {
	Noop         bool
	Blocked      bool
	Stale        bool
	Blockers     []string
	Created      []string
	Replaced     []string
	Actions      []string
	Plan         UpdatePlan
	Index        IndexDocument
	CapsulePath  string
	PackPath     string
	MessageTitle string
	MessageBody  string
}

const (
	UpdateSuccessTitle = "Context Economy updated."
	UpdateSuccessBody  = "Index, capsule, and pack were written under Atlas Home."
	UpdateNoopTitle    = "Context Economy already fresh."
	UpdateNoopBody     = "No context update was required."
	UpdateStaleMessage = "context plan changed; review again"
)

// BuildUpdatePlan previews creating/updating index, capsule, and a pack.
// Read-only: does not write files.
func BuildUpdatePlan(root string, initialized bool, state config.StateDocument, objective string) UpdatePlan {
	plan := UpdatePlan{
		Blockers:  []string{},
		Warnings:  []string{},
		Targets:   []UpdateTarget{},
		Creates:   []string{},
		Replaces:  []string{},
		Objective: strings.TrimSpace(objective),
	}
	if plan.Objective == "" {
		plan.Objective = DefaultPackObjective
	}
	if !initialized {
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "project is not initialized")
		return plan
	}

	current := InspectFromState(root, initialized, state)
	plan.Current = current
	homePath, err := ResolveHome()
	if err != nil {
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "Atlas Home unresolved: "+err.Error())
		return plan
	}
	plan.HomePath = homePath

	name := state.ProjectName
	id, err := ProjectID(root, name)
	if err != nil {
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "project id unresolved: "+err.Error())
		return plan
	}
	plan.ProjectID = id

	indexPath := IndexPath(homePath, id)
	capsulePath := CapsulePath(homePath, id)
	packID := packIDFor(plan.Objective)
	packPath := PackPath(homePath, id, packID)

	addTarget := func(path, action, kind, reason string) {
		for _, existing := range plan.Targets {
			if existing.Path == path && existing.Action == action {
				return
			}
		}
		plan.Targets = append(plan.Targets, UpdateTarget{
			Path: path, Action: action, Kind: kind, Reason: reason,
		})
		switch action {
		case UpdateActionCreate:
			plan.Creates = append(plan.Creates, path)
		case UpdateActionReplace:
			plan.Replaces = append(plan.Replaces, path)
		}
	}

	switch current.State {
	case StatusMissing, StatusNA:
		addTarget(indexPath, UpdateActionCreate, "index", "context index missing")
		addTarget(capsulePath, UpdateActionCreate, "capsule", "context capsule missing")
		addTarget(packPath, UpdateActionCreate, "pack", "context pack missing")
		plan.NeedsWrite = true
	case StatusUnreadable:
		addTarget(indexPath, UpdateActionReplace, "index", "context index unreadable; rewrite")
		addTarget(capsulePath, UpdateActionReplace, "capsule", "rewrite capsule with index")
		addTarget(packPath, UpdateActionReplace, "pack", "rewrite pack with index")
		plan.NeedsWrite = true
	case StatusStale:
		addTarget(indexPath, UpdateActionReplace, "index", "context index stale")
		addTarget(capsulePath, UpdateActionReplace, "capsule", "refresh capsule from new index")
		addTarget(packPath, UpdateActionReplace, "pack", "refresh pack from new index")
		plan.NeedsWrite = true
	case StatusPresent:
		// Fresh: still allow explicit refresh of pack for the requested objective.
		if _, err := os.Stat(packPath); os.IsNotExist(err) {
			addTarget(packPath, UpdateActionCreate, "pack", "requested pack missing")
			plan.NeedsWrite = true
		} else {
			plan.Warnings = append(plan.Warnings, "index/capsule fresh; Apply refreshes pack for the current objective")
			addTarget(indexPath, UpdateActionReplace, "index", "explicit refresh requested")
			addTarget(capsulePath, UpdateActionReplace, "capsule", "explicit refresh requested")
			addTarget(packPath, UpdateActionReplace, "pack", "explicit pack refresh for objective")
			plan.NeedsWrite = true
		}
	default:
		addTarget(indexPath, UpdateActionCreate, "index", "initialize context index")
		addTarget(capsulePath, UpdateActionCreate, "capsule", "initialize context capsule")
		addTarget(packPath, UpdateActionCreate, "pack", "initialize context pack")
		plan.NeedsWrite = true
	}

	return plan
}

// ApplyUpdate recomputes the plan, checks signature, and writes Atlas Home context.
// Also updates minimal .atlas/state.yaml metadata. Never writes large context into the repo.
func ApplyUpdate(root, expectedSignature string, state config.StateDocument, objective string, nowFn func() time.Time) (UpdateResult, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return UpdateResult{}, fmt.Errorf("context update: workspace root is required")
	}
	now := time.Now().UTC()
	if nowFn != nil {
		now = nowFn().UTC()
	}

	initialized := state.Initialized
	plan := BuildUpdatePlan(root, initialized, state, objective)
	result := UpdateResult{Plan: plan, Blockers: plan.Blockers, Blocked: plan.Blocked}
	if plan.Blocked {
		return result, fmt.Errorf("context update blocked: %s", strings.Join(plan.Blockers, "; "))
	}
	if expectedSignature == "" {
		return result, fmt.Errorf("context update: reviewed plan signature is required")
	}
	if expectedSignature != plan.Signature() {
		result.Stale = true
		result.MessageTitle = UpdateStaleMessage
		result.MessageBody = UpdateStaleMessage
		return result, nil
	}
	if !plan.NeedsApply() {
		result.Noop = true
		result.MessageTitle = UpdateNoopTitle
		result.MessageBody = UpdateNoopBody
		return result, nil
	}

	if _, err := home.EnsureAndMirror(now); err != nil {
		return result, fmt.Errorf("context update: %w", err)
	}
	// Ensure context layout exists even if older homes predate the directory.
	if err := os.MkdirAll(ProjectsRoot(plan.HomePath), 0o755); err != nil {
		return result, fmt.Errorf("context update: create projects root: %w", err)
	}

	idx, err := BuildIndex(root, state.ProjectName, now)
	if err != nil {
		return result, fmt.Errorf("context update: build index: %w", err)
	}
	idx.ProjectID = plan.ProjectID
	indexYAML, err := RenderIndexYAML(idx)
	if err != nil {
		return result, err
	}
	capsule := RenderCapsule(idx)
	pack := BuildPack(idx, plan.Objective, now)
	packYAML, err := RenderPackYAML(pack)
	if err != nil {
		return result, err
	}

	writes := []struct {
		path   string
		data   string
		kind   string
		action string
	}{
		{IndexPath(plan.HomePath, plan.ProjectID), indexYAML, "index", ""},
		{CapsulePath(plan.HomePath, plan.ProjectID), capsule, "capsule", ""},
		{PackPath(plan.HomePath, plan.ProjectID, pack.PackID), packYAML, "pack", ""},
	}
	for i := range writes {
		for _, t := range plan.Targets {
			if t.Path == writes[i].path {
				writes[i].action = t.Action
				break
			}
		}
		if writes[i].action == "" {
			if _, statErr := os.Stat(writes[i].path); os.IsNotExist(statErr) {
				writes[i].action = UpdateActionCreate
			} else {
				writes[i].action = UpdateActionReplace
			}
		}
	}

	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
			return result, fmt.Errorf("context update: create parent for %s: %w", w.path, err)
		}
		if err := os.WriteFile(w.path, []byte(w.data), 0o644); err != nil {
			return result, fmt.Errorf("context update: write %s: %w", w.path, err)
		}
		if w.action == UpdateActionCreate {
			result.Created = append(result.Created, w.path)
		} else {
			result.Replaced = append(result.Replaced, w.path)
		}
		result.Actions = append(result.Actions, w.action+" "+w.path)
	}

	if err := updateProjectState(root, state, plan.ProjectID, idx, now); err != nil {
		return result, err
	}

	result.Index = idx
	result.CapsulePath = CapsulePath(plan.HomePath, plan.ProjectID)
	result.PackPath = PackPath(plan.HomePath, plan.ProjectID, pack.PackID)
	result.MessageTitle = UpdateSuccessTitle
	result.MessageBody = UpdateSuccessBody
	return result, nil
}

func updateProjectState(root string, state config.StateDocument, projectID string, idx IndexDocument, now time.Time) error {
	statePath := filepath.Join(root, filepath.FromSlash(config.FileState))
	if !state.Initialized {
		// Do not create state for uninitialized projects.
		return nil
	}
	state.ContextEconomyUpdatedAt = now.UTC().Format(time.RFC3339)
	state.ContextEconomyProjectID = projectID
	state.ContextEconomyHomeRel = filepath.ToSlash(filepath.Join(DirContextProjects, projectID))
	state.ContextEconomyFingerprint = idx.Fingerprint
	if state.SchemaVersion == 0 {
		state.SchemaVersion = config.PersistSchemaVersion
	}
	data, err := marshalYAML(state)
	if err != nil {
		return fmt.Errorf("context update: marshal state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return fmt.Errorf("context update: create .atlas: %w", err)
	}
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		return fmt.Errorf("context update: write state: %w", err)
	}
	return nil
}

func marshalYAML(v any) ([]byte, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
