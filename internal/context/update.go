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
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
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

// contextAfterWriteHook is an optional test seam after each Home context write.
var contextAfterWriteHook func(rel string) error

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
		packPresent, packErr := InspectContextLeaf(homePath, ContextPackRel(id, packID))
		if packErr != nil || !packPresent {
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
// Writes are transactional: any error after the first mutation restores context files,
// Home local state, and portable .atlas/state.yaml to the pre-Apply baseline.
func ApplyUpdate(root, expectedSignature string, state config.StateDocument, objective string, nowFn func() time.Time) (UpdateResult, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return UpdateResult{}, fmt.Errorf("context update: workspace root is required")
	}
	now := time.Now().UTC()
	if nowFn != nil {
		now = nowFn().UTC()
	}
	if expectedSignature == "" {
		return UpdateResult{}, fmt.Errorf("context update: reviewed plan signature is required")
	}

	homePath, err := home.Resolve()
	if err != nil {
		return UpdateResult{}, fmt.Errorf("context update: %w", err)
	}

	lockSet, err := mutatelock.Acquire(mutatelock.Options{HomePath: homePath, Workspace: root})
	if err != nil {
		return UpdateResult{}, fmt.Errorf("context update: %w", err)
	}
	defer func() { _ = lockSet.Release() }()

	// ---------- Locked: recompute plan → validate signature → render → snapshot ----------
	initialized := state.Initialized
	plan := BuildUpdatePlan(root, initialized, state, objective)
	result := UpdateResult{Plan: plan, Blockers: plan.Blockers, Blocked: plan.Blocked}
	if plan.Blocked {
		return result, fmt.Errorf("context update blocked: %s", strings.Join(plan.Blockers, "; "))
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

	// Preflight: render all payloads from locked observations.
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
	stateData, err := renderPortableState(state, plan.ProjectID, idx, now)
	if err != nil {
		return result, err
	}

	writes := []contextWrite{
		{path: IndexPath(plan.HomePath, plan.ProjectID), data: indexYAML},
		{path: CapsulePath(plan.HomePath, plan.ProjectID), data: capsule},
		{path: PackPath(plan.HomePath, plan.ProjectID, pack.PackID), data: packYAML},
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

	// EnsureAndMirror is an idempotent global Home precondition (not undone here).
	if _, err := home.EnsureAndMirror(now); err != nil {
		return result, fmt.Errorf("context update: %w", err)
	}
	// Fail closed on corrupt/unreadable local state before any project mutation.
	if state.Initialized {
		if _, _, err := home.LoadProjectLocalState(plan.HomePath, plan.ProjectID); err != nil {
			return result, fmt.Errorf("context update: %w", err)
		}
	}

	// Snapshot BEFORE any project-Home mutation (layout/identity/local/context).
	snap, err := captureContextMutationSnapshot(root, plan.HomePath, plan.ProjectID, writes)
	if err != nil {
		return result, fmt.Errorf("context update: snapshot: %w", err)
	}
	rollback := func(cause error) (UpdateResult, error) {
		if restoreErr := restoreContextMutationSnapshot(root, snap); restoreErr != nil {
			return UpdateResult{}, fmt.Errorf("context update: %v (rollback failed: %v)", cause, restoreErr)
		}
		return UpdateResult{}, fmt.Errorf("context update: %w (rolled back context baseline)", cause)
	}

	layoutDirs, err := home.EnsureProjectLayoutCreated(plan.HomePath, plan.ProjectID)
	snap.projectHome.Footprint.MergeDirs(layoutDirs)
	if err != nil {
		return rollback(err)
	}
	idWrote, idDirs, idErr := home.WriteProjectIdentityTracked(plan.HomePath, plan.ProjectID, state.ProjectName, root, now)
	snap.projectHome.Footprint.MergeDirs(idDirs)
	if idErr != nil {
		return rollback(idErr)
	}
	snap.projectHome.Footprint.AddFile(idWrote)

	for _, w := range writes {
		rel, relErr := filepath.Rel(plan.HomePath, w.path)
		if relErr != nil {
			return rollback(fmt.Errorf("path %s: %w", w.path, relErr))
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "..") {
			return rollback(fmt.Errorf("path escapes Atlas Home: %s", w.path))
		}
		wrote, dirs, werr := fsafety.AtomicWriteContainedTracked(plan.HomePath, rel, []byte(w.data), 0o644, home.DirPermHome, ".atlas-write-*.tmp")
		snap.homeFootprint.MergeDirs(dirs)
		if werr != nil {
			return rollback(fmt.Errorf("write %s: %w", rel, werr))
		}
		snap.homeFootprint.AddFile(wrote)
		if contextAfterWriteHook != nil {
			if hookErr := contextAfterWriteHook(rel); hookErr != nil {
				return rollback(hookErr)
			}
		}
		if w.action == UpdateActionCreate {
			result.Created = append(result.Created, w.path)
		} else {
			result.Replaced = append(result.Replaced, w.path)
		}
		result.Actions = append(result.Actions, w.action+" "+w.path)
	}

	if state.Initialized {
		if err := writeContextStates(root, plan.HomePath, plan.ProjectID, idx, now, stateData, &snap); err != nil {
			return rollback(err)
		}
	}

	result.Index = idx
	result.CapsulePath = CapsulePath(plan.HomePath, plan.ProjectID)
	result.PackPath = PackPath(plan.HomePath, plan.ProjectID, pack.PackID)
	result.MessageTitle = UpdateSuccessTitle
	result.MessageBody = UpdateSuccessBody
	return result, nil
}

type contextWrite struct {
	path   string
	data   string
	action string
}

type contextMutationSnapshot struct {
	homePath      string
	projectID     string
	projectHome   home.ProjectLayoutSnapshot
	homeFiles     []fsafety.FileSnapshot
	portableState fsafety.FileSnapshot
	homeFootprint fsafety.TransactionFootprint
	rootFootprint fsafety.TransactionFootprint
}

// captureFileSnapshotFn is a test seam; production uses fsafety.CaptureFileSnapshot.
var captureFileSnapshotFn = fsafety.CaptureFileSnapshot

func captureContextMutationSnapshot(root, homePath, projectID string, writes []contextWrite) (contextMutationSnapshot, error) {
	snap := contextMutationSnapshot{homePath: homePath, projectID: projectID}
	projectHome, err := home.CaptureProjectLayoutSnapshot(homePath, projectID)
	if err != nil {
		return snap, err
	}
	snap.projectHome = projectHome
	for _, w := range writes {
		rel, err := filepath.Rel(homePath, w.path)
		if err != nil {
			return snap, err
		}
		rel = filepath.ToSlash(rel)
		fs, err := captureFileSnapshotFn(homePath, rel)
		if err != nil {
			return snap, fmt.Errorf("context file %s: %w", rel, err)
		}
		snap.homeFiles = append(snap.homeFiles, fs)
	}
	ps, err := captureFileSnapshotFn(root, config.FileState)
	if err != nil {
		return snap, fmt.Errorf("portable state: %w", err)
	}
	snap.portableState = ps
	return snap, nil
}

func restoreContextMutationSnapshot(root string, snap contextMutationSnapshot) error {
	var errs []string
	homeFiles := append([]fsafety.FileSnapshot(nil), snap.homeFiles...)
	sort.Slice(homeFiles, func(i, j int) bool { return homeFiles[i].Rel < homeFiles[j].Rel })
	for _, fs := range homeFiles {
		wrote := snap.homeFootprint.FileByRel(fs.Rel)
		if err := fsafety.RestoreFileSnapshot(snap.homePath, fs, wrote, snap.homeFootprint.DeletedByRel(fs.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fs.Rel, err))
		}
	}
	if err := fsafety.RestoreCreatedDirs(snap.homePath, snap.homeFootprint); err != nil {
		errs = append(errs, err.Error())
	}
	wrote := snap.rootFootprint.FileByRel(snap.portableState.Rel)
	if err := fsafety.RestoreFileSnapshot(root, snap.portableState, wrote, snap.rootFootprint.DeletedByRel(snap.portableState.Rel)); err != nil {
		errs = append(errs, fmt.Sprintf("portable state: %v", err))
	}
	if err := fsafety.RestoreCreatedDirs(root, snap.rootFootprint); err != nil {
		errs = append(errs, err.Error())
	}
	if err := home.RestoreProjectLayoutSnapshot(snap.projectHome); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("context rollback: %s", strings.Join(errs, "; "))
}

func renderPortableState(state config.StateDocument, projectID string, idx IndexDocument, now time.Time) ([]byte, error) {
	if !state.Initialized {
		return nil, nil
	}
	stamp := now.UTC().Format(time.RFC3339)
	state.ContextEconomyUpdatedAt = stamp
	state.ContextEconomyProjectID = projectID
	state.ContextEconomyHomeRel = HomeRelContext(projectID)
	state.ContextEconomyFingerprint = idx.Fingerprint
	if state.SchemaVersion == 0 {
		state.SchemaVersion = config.PersistSchemaVersion
	}
	return marshalYAML(state)
}

func writeContextStates(root, homePath, projectID string, idx IndexDocument, now time.Time, stateData []byte, snap *contextMutationSnapshot) error {
	stamp := now.UTC().Format(time.RFC3339)
	local, _, err := home.LoadProjectLocalState(homePath, projectID)
	if err != nil {
		return fmt.Errorf("context update: %w", err)
	}
	local.ContextEconomyUpdatedAt = stamp
	local.ContextEconomyFingerprint = idx.Fingerprint
	localWrote, localDirs, err := home.WriteProjectLocalStateTracked(homePath, projectID, local)
	if snap != nil {
		snap.projectHome.Footprint.MergeDirs(localDirs)
	}
	if err != nil {
		return fmt.Errorf("context update: %w", err)
	}
	if snap != nil {
		snap.projectHome.Footprint.AddFile(localWrote)
	}
	if len(stateData) == 0 {
		return nil
	}
	wrote, dirs, err := fsafety.AtomicWriteContainedTracked(root, config.FileState, stateData, 0o644, 0o755, ".atlas-write-*.tmp")
	if snap != nil {
		snap.rootFootprint.MergeDirs(dirs)
	}
	if err != nil {
		return fmt.Errorf("context update: write state: %w", err)
	}
	if snap != nil {
		snap.rootFootprint.AddFile(wrote)
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
