package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

const (
	RepairSuccessTitle = "Runtime repair applied."
	RepairSuccessBody  = "Atlas-owned runtime files were restored. Developer-owned runtime surfaces were left untouched."
	RepairNoopTitle    = "Runtime is healthy."
	RepairNoopBody     = "No repair actions were required."
	RepairStaleMessage = "runtime drift changed; review again"
)

// repairAfterWriteHook is an optional test seam invoked after each successful
// Atlas-owned write during Apply. Production code leaves it nil.
var repairAfterWriteHook func(rel string) error

// removeAttemptBackup removes a Home-relative attempt backup after successful
// baseline restore. Overridable in tests via export_test only.
var removeAttemptBackup = home.RemoveContainedRel

// RuntimeRepairResult is the outcome of an explicit Apply.
type RuntimeRepairResult struct {
	Noop         bool
	Blocked      bool
	Stale        bool
	Blockers     []string
	BackupDir    string
	Created      []string
	Replaced     []string
	Quarantined  []string
	Actions      []string
	Plan         RuntimeRepairPlan
	MessageTitle string
	MessageBody  string
}

type repairMutationSnapshot struct {
	files         []fsafety.FileSnapshot
	homePath      string
	projectID     string
	projectHome   home.ProjectLayoutSnapshot
	footprint     fsafety.TransactionFootprint
	attemptBackup string // Home-relative backup dir created by this attempt
}

// ApplyRuntimeRepair recomputes the plan, compares it to the reviewed signature,
// and mutates only when the reviewed plan still matches.
// Mutation is transactional: any error after the first workspace/Home project
// change restores Atlas-owned targets and repair state to the pre-Apply baseline.
// Developer/external surfaces are never mutated.
func ApplyRuntimeRepair(root, expectedSignature string, nowFn func() time.Time) (RuntimeRepairResult, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return RuntimeRepairResult{}, fmt.Errorf("runtime repair: workspace root is required")
	}

	now := time.Now().UTC()
	if nowFn != nil {
		now = nowFn().UTC()
	}
	if expectedSignature == "" {
		return RuntimeRepairResult{}, fmt.Errorf("runtime repair: reviewed plan signature is required")
	}

	homePath, err := home.Resolve()
	if err != nil {
		return RuntimeRepairResult{}, fmt.Errorf("runtime repair: resolve Atlas Home: %w", err)
	}

	lockSet, err := mutatelock.Acquire(mutatelock.Options{HomePath: homePath, Workspace: root})
	if err != nil {
		return RuntimeRepairResult{}, fmt.Errorf("runtime repair: %w", err)
	}
	defer func() { _ = lockSet.Release() }()

	// ---------- Locked: recompute plan → validate signature → snapshot → mutate ----------
	files, err := project.DiscoverFiles(root)
	if err != nil {
		return RuntimeRepairResult{}, err
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	health := EvaluateHealth(root, atlas, files)
	plan := BuildRuntimeRepairPlan(root, health)

	result := RuntimeRepairResult{Plan: plan, Blockers: plan.Blockers, Blocked: plan.Blocked}
	if plan.Blocked {
		return result, fmt.Errorf("runtime repair blocked: %s", strings.Join(plan.Blockers, "; "))
	}

	currentSig := plan.Signature()
	if expectedSignature != currentSig {
		result.Stale = true
		result.MessageTitle = RepairStaleMessage
		result.MessageBody = RepairStaleMessage
		return result, nil
	}

	if !plan.NeedsApply() {
		result.Noop = true
		result.MessageTitle = RepairNoopTitle
		result.MessageBody = RepairNoopBody
		return result, nil
	}

	// Preflight: render Atlas-owned write payloads from locked observations.
	doc := health.Document
	type pendingWrite struct {
		target  RuntimeRepairTarget
		content string
	}
	var writes []pendingWrite
	for _, target := range plan.Targets {
		switch target.Action {
		case RepairActionCreate, RepairActionReplace:
			if target.Kind == RepairKindHome {
				continue
			}
			if err := configValidateWrite(target.Path); err != nil {
				return result, err
			}
			var existing []byte
			if target.Path == config.FileAgentsMD {
				full, joinErr := fsafety.ContainedJoin(root, target.Path)
				if joinErr == nil {
					if data, readErr := os.ReadFile(full); readErr == nil {
						existing = data
					}
				}
			}
			content, renderErr := renderRepairFile(target.Path, doc, existing, homePath)
			if renderErr != nil {
				return result, renderErr
			}
			writes = append(writes, pendingWrite{target: target, content: content})
		}
	}

	// EnsureAndMirror is an idempotent Atlas Home precondition (global Home
	// assets). Project Apply rollback restores project-scoped repair targets
	// and state; it does not undo global Home mirror refresh.
	homeResult, err := home.EnsureAndMirror(now)
	if err != nil {
		return result, fmt.Errorf("runtime repair: %w", err)
	}
	homePath = homeResult.HomePath

	projectName := health.Document.Project.Name
	if projectName == "" {
		projectName = health.State.ProjectName
	}
	projectID, err := home.ProjectID(root, projectName)
	if err != nil {
		return result, fmt.Errorf("runtime repair: project id: %w", err)
	}
	// Fail closed on corrupt/unreadable local state before any project mutation.
	if _, _, err := home.LoadProjectLocalState(homeResult.HomePath, projectID); err != nil {
		return result, fmt.Errorf("runtime repair: %w", err)
	}

	// Snapshot BEFORE any project-Home mutation (layout/identity/local state).
	snap, err := captureRepairMutationSnapshot(root, homeResult.HomePath, projectID, plan)
	if err != nil {
		return result, fmt.Errorf("runtime repair: snapshot: %w", err)
	}
	rollback := func(cause error) (RuntimeRepairResult, error) {
		if restoreErr := restoreRepairMutationSnapshot(root, snap); restoreErr != nil {
			return RuntimeRepairResult{}, fmt.Errorf("runtime repair: %v (rollback failed: %v)", cause, restoreErr)
		}
		// Attempt-backup stamps are outside layout footprint teardown (SAFE_TO_KEEP
		// RemoveContainedRel). Remove stamp before empty createdDirs cleanup.
		if snap.attemptBackup != "" {
			if cleanErr := removeAttemptBackup(homeResult.HomePath, snap.attemptBackup); cleanErr != nil {
				return RuntimeRepairResult{}, fmt.Errorf("runtime repair: %w (restored project repair baseline; transactional backup cleanup failed: %v)", cause, cleanErr)
			}
		}
		if err := fsafety.RestoreCreatedDirs(homeResult.HomePath, snap.projectHome.Footprint); err != nil {
			return RuntimeRepairResult{}, fmt.Errorf("runtime repair: %v (rollback failed: %v)", cause, err)
		}
		return RuntimeRepairResult{}, fmt.Errorf("runtime repair: %w (rolled back project repair baseline)", cause)
	}

	layoutDirs, err := home.EnsureProjectLayoutCreated(homeResult.HomePath, projectID)
	snap.projectHome.Footprint.MergeDirs(layoutDirs)
	if err != nil {
		return rollback(err)
	}
	idWrote, idDirs, idErr := home.WriteProjectIdentityTracked(homeResult.HomePath, projectID, projectName, root, now)
	snap.projectHome.Footprint.MergeDirs(idDirs)
	if idErr != nil {
		return rollback(idErr)
	}
	snap.projectHome.Footprint.AddFile(idWrote)

	var backupItems []config.ConflictBackup
	for _, target := range plan.Targets {
		if target.Backup && target.Kind != RepairKindHome {
			backupItems = append(backupItems, config.ConflictBackup{
				Rel:    target.Path,
				Action: target.Action,
				Reason: target.Reason,
			})
		}
	}

	// Attempt-backup stamp is discarded via RemoveContainedRel after successful
	// baseline restore. On BackupConflicts failure, merge partial footprint so
	// rollback can tear down dirs Atlas already created.
	backupDir, manifest, backupFP, err := config.BackupConflicts(root, homeResult.HomePath, projectID, backupItems, now)
	if err != nil {
		snap.projectHome.Footprint.MergeFootprint(backupFP)
		return rollback(err)
	}
	result.BackupDir = backupDir
	snap.attemptBackup = backupDir

	for i := range manifest.Entries {
		manifest.Entries[i].Result = "quarantined"
		if manifest.Entries[i].Action == RepairActionReplace {
			manifest.Entries[i].Result = "replaced"
		}
	}

	for _, target := range plan.Targets {
		if target.Action != RepairActionQuarantine {
			continue
		}
		baseline := fileSnapshotByRel(snap.files, target.Path)
		if err := removeConflict(root, target.Path, baseline, &snap.footprint); err != nil {
			return rollback(err)
		}
		result.Quarantined = append(result.Quarantined, target.Path)
		result.Actions = append(result.Actions, target.Action+" "+target.Path)
	}

	for _, w := range writes {
		wrote, dirs, werr := fsafety.AtomicWriteContainedTracked(root, w.target.Path, []byte(w.content), 0o644, 0o755, ".atlas-write-*.tmp")
		snap.footprint.MergeDirs(dirs)
		if werr != nil {
			return rollback(fmt.Errorf("write %s: %w", w.target.Path, werr))
		}
		snap.footprint.AddFile(wrote)
		if repairAfterWriteHook != nil {
			if hookErr := repairAfterWriteHook(w.target.Path); hookErr != nil {
				return rollback(hookErr)
			}
		}
		if w.target.Action == RepairActionCreate {
			result.Created = append(result.Created, w.target.Path)
		} else {
			result.Replaced = append(result.Replaced, w.target.Path)
		}
		result.Actions = append(result.Actions, w.target.Action+" "+w.target.Path)
	}
	for _, target := range plan.Targets {
		if target.Kind == RepairKindHome && (target.Action == RepairActionCreate || target.Action == RepairActionReplace) {
			if target.Action == RepairActionCreate {
				result.Created = append(result.Created, target.Path)
			} else {
				result.Replaced = append(result.Replaced, target.Path)
			}
			result.Actions = append(result.Actions, target.Action+" "+target.Path)
		}
	}

	if backupDir != "" {
		if err := config.WriteBackupManifest(homeResult.HomePath, backupDir, manifest); err != nil {
			return rollback(err)
		}
	}

	if err := updateRepairState(root, homeResult.HomePath, projectID, health, now, result.Actions, &snap); err != nil {
		return rollback(err)
	}

	result.MessageTitle = RepairSuccessTitle
	result.MessageBody = RepairSuccessBody
	return result, nil
}

func captureRepairMutationSnapshot(root, homePath, projectID string, plan RuntimeRepairPlan) (repairMutationSnapshot, error) {
	snap := repairMutationSnapshot{homePath: homePath, projectID: projectID}
	projectHome, err := home.CaptureProjectLayoutSnapshot(homePath, projectID)
	if err != nil {
		return snap, err
	}
	snap.projectHome = projectHome
	seen := map[string]struct{}{}
	add := func(rel string) error {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "" || rel == "." || rel == RepairHomePath {
			return nil
		}
		if _, ok := seen[rel]; ok {
			return nil
		}
		seen[rel] = struct{}{}
		fs, err := fsafety.CaptureFileSnapshot(root, rel)
		if err != nil {
			return err
		}
		snap.files = append(snap.files, fs)
		return nil
	}
	for _, target := range plan.Targets {
		if target.Kind == RepairKindHome {
			continue
		}
		if err := add(target.Path); err != nil {
			return snap, err
		}
	}
	if err := add(config.FileState); err != nil {
		return snap, err
	}
	return snap, nil
}

func restoreRepairMutationSnapshot(root string, snap repairMutationSnapshot) error {
	var errs []string
	files := append([]fsafety.FileSnapshot(nil), snap.files...)
	sortFileSnapshots(files)
	for _, fs := range files {
		wrote := snap.footprint.FileByRel(fs.Rel)
		if err := fsafety.RestoreFileSnapshot(root, fs, wrote, snap.footprint.DeletedByRel(fs.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fs.Rel, err))
		}
	}
	if err := fsafety.RestoreCreatedDirs(root, snap.footprint); err != nil {
		errs = append(errs, err.Error())
	}
	// Files + prior modes only; createdDirs removed after attempt-backup cleanup.
	if err := home.RestoreProjectLayoutSnapshotFiles(snap.projectHome); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("repair rollback: %s", strings.Join(errs, "; "))
}

func sortFileSnapshots(files []fsafety.FileSnapshot) {
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
}

func renderRepairFile(rel string, doc config.ProjectDocument, existing []byte, homePath string) (string, error) {
	switch rel {
	case config.FileAgentsMD:
		return config.RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), doc.Adapters.Selected, existing)
	case config.FileCursorAtlasMDC:
		return config.RenderCursorAtlasMDC(doc.Project.Name)
	case config.FileOpenCodeAtlas:
		return config.RenderOpenCodeAtlas(doc.Project.Name)
	case config.FileAgentRegistry:
		return config.RenderAgentRegistry(doc.Project.Name, doc.Adapters.Selected, homePath), nil
	case config.FileRuntimeManifest:
		return config.RenderRuntimeManifestYAML(doc.Project.Name, doc.Adapters.Selected)
	case config.FileAssetsLock:
		return config.RenderAssetsLockYAMLFor(homePath, doc)
	case config.FileSDDOpenSpecContract:
		return config.RenderSDDOpenSpecContract()
	default:
		if config.IsAtlasAgentRuntimePath(rel) {
			return config.RenderAtlasAgent(filepath.Base(rel))
		}
		return "", fmt.Errorf("runtime repair: unsupported write path %q", rel)
	}
}

func configValidateWrite(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	switch clean {
	case config.FileAgentsMD, config.FileCursorAtlasMDC, config.FileOpenCodeAtlas,
		config.FileAgentRegistry, config.FileRuntimeManifest, config.FileAssetsLock,
		config.FileSDDOpenSpecContract, RepairHomePath:
		return nil
	default:
		if config.IsAtlasAgentRuntimePath(clean) {
			return nil
		}
		return fmt.Errorf("runtime repair: refused write path %q", rel)
	}
}

func fileSnapshotByRel(files []fsafety.FileSnapshot, rel string) fsafety.FileSnapshot {
	rel = filepath.ToSlash(filepath.Clean(rel))
	for _, fs := range files {
		if filepath.ToSlash(filepath.Clean(fs.Rel)) == rel {
			return fs
		}
	}
	return fsafety.FileSnapshot{Rel: rel, Exists: false}
}

func removeConflict(root, rel string, baseline fsafety.FileSnapshot, fp *fsafety.TransactionFootprint) error {
	full, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return fmt.Errorf("runtime repair: quarantine path %s: %w", rel, err)
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("runtime repair: stat %s: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("runtime repair: quarantine %s: rollback conflict: symlink where regular file expected (expected regular file; observed symlink); no destructive action taken", rel)
	}
	if info.IsDir() {
		// MUST_REPLACE: refuse recursive wipe of directory children.
		return fmt.Errorf("runtime repair: quarantine %s: rollback conflict: refusing recursive directory wipe (expected regular file; observed directory); no destructive action taken", rel)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("runtime repair: quarantine %s: rollback conflict: not a regular file (observed %s); no destructive action taken", rel, info.Mode())
	}
	if !baseline.Exists || !baseline.WasFile {
		return fmt.Errorf("runtime repair: quarantine %s: rollback conflict: no baseline file snapshot for transactional delete; no destructive action taken", rel)
	}
	deleted, err := fsafety.RemoveRegularFileTracked(root, baseline)
	if err != nil {
		return fmt.Errorf("runtime repair: quarantine %s: %w", rel, err)
	}
	if fp != nil {
		fp.AddDeleted(deleted)
	}
	return nil
}

func updateRepairState(root, homePath, projectID string, health Health, now time.Time, actions []string, snap *repairMutationSnapshot) error {
	stamp := now.Format(time.RFC3339)

	local, _, err := home.LoadProjectLocalState(homePath, projectID)
	if err != nil {
		return fmt.Errorf("runtime repair: %w", err)
	}
	local.RuntimeRepairedAt = stamp
	local.LastRuntimeRepairActions = append([]string{}, actions...)
	localWrote, localDirs, err := home.WriteProjectLocalStateTracked(homePath, projectID, local)
	if snap != nil {
		snap.projectHome.Footprint.MergeDirs(localDirs)
	}
	if err != nil {
		return fmt.Errorf("runtime repair: %w", err)
	}
	if snap != nil {
		snap.projectHome.Footprint.AddFile(localWrote)
	}

	state := health.State
	if !health.StateLoads {
		state = config.StateDocument{
			SchemaVersion: config.PersistSchemaVersion,
			Initialized:   true,
			ProjectName:   health.Document.Project.Name,
		}
	}
	state.Initialized = true
	state.RuntimeMaterialized = true
	if state.RuntimeMaterializedAt == "" {
		state.RuntimeMaterializedAt = stamp
	}
	// Transitional mirrors in portable state; prefer Home project-local state.
	state.RuntimeRepairedAt = stamp
	state.LastRuntimeRepairActions = append([]string{}, actions...)
	if state.AppliedAt == "" {
		state.AppliedAt = stamp
	}
	if state.AtlasVersion == "" {
		ver := version.Version
		if ver == "" {
			ver = "0.1.0"
		}
		state.AtlasVersion = ver
	}
	if strings.TrimSpace(state.ProjectName) == "" {
		state.ProjectName = health.Document.Project.Name
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = config.PersistSchemaVersion
	}

	data, err := yaml.Marshal(&state)
	if err != nil {
		return fmt.Errorf("runtime repair: marshal state: %w", err)
	}
	wrote, dirs, err := fsafety.AtomicWriteContainedTracked(root, config.FileState, data, 0o644, 0o755, ".atlas-write-*.tmp")
	if snap != nil {
		snap.footprint.MergeDirs(dirs)
	}
	if err != nil {
		return fmt.Errorf("runtime repair: write state: %w", err)
	}
	if snap != nil {
		snap.footprint.AddFile(wrote)
	}
	return nil
}
