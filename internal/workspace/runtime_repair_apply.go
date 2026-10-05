package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

const (
	RepairSuccessTitle = "Runtime repair applied."
	RepairSuccessBody  = "Conflicting runtime artifacts were quarantined and Atlas-owned files were written."
	RepairNoopTitle    = "Runtime is healthy."
	RepairNoopBody     = "No repair actions were required."
	RepairStaleMessage = "runtime drift changed; review again"
)

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

// ApplyRuntimeRepair recomputes the plan, compares it to the reviewed signature,
// and mutates only when the reviewed plan still matches.
func ApplyRuntimeRepair(root, expectedSignature string, nowFn func() time.Time) (RuntimeRepairResult, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return RuntimeRepairResult{}, fmt.Errorf("runtime repair: workspace root is required")
	}

	now := time.Now().UTC()
	if nowFn != nil {
		now = nowFn().UTC()
	}

	files, err := DiscoverFiles(root)
	if err != nil {
		return RuntimeRepairResult{}, err
	}
	atlas := EvaluateAtlasStatus(root, files)
	health := EvaluateRuntimeHealth(root, atlas, files)
	plan := BuildRuntimeRepairPlan(root, health)

	result := RuntimeRepairResult{Plan: plan, Blockers: plan.Blockers, Blocked: plan.Blocked}
	if plan.Blocked {
		return result, fmt.Errorf("runtime repair blocked: %s", strings.Join(plan.Blockers, "; "))
	}

	currentSig := plan.Signature()
	if expectedSignature == "" {
		return result, fmt.Errorf("runtime repair: reviewed plan signature is required")
	}
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

	var backupItems []config.ConflictBackup
	for _, target := range plan.Targets {
		if target.Backup {
			backupItems = append(backupItems, config.ConflictBackup{
				Rel:    target.Path,
				Action: target.Action,
				Reason: target.Reason,
			})
		}
	}

	backupDir, manifest, err := config.BackupConflicts(root, backupItems, now)
	if err != nil {
		return result, fmt.Errorf("runtime repair: %w", err)
	}
	result.BackupDir = backupDir

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
		if err := removeConflict(root, target.Path); err != nil {
			return result, err
		}
		result.Quarantined = append(result.Quarantined, target.Path)
		result.Actions = append(result.Actions, target.Action+" "+target.Path)
	}

	doc := health.Document
	for _, target := range plan.Targets {
		switch target.Action {
		case RepairActionCreate, RepairActionReplace:
			if err := writeAtlasRuntimeFile(root, target, doc); err != nil {
				return result, err
			}
			if target.Action == RepairActionCreate {
				result.Created = append(result.Created, target.Path)
			} else {
				result.Replaced = append(result.Replaced, target.Path)
			}
			result.Actions = append(result.Actions, target.Action+" "+target.Path)
		}
	}

	if backupDir != "" {
		if err := config.WriteBackupManifest(root, backupDir, manifest); err != nil {
			return result, err
		}
	}

	if err := updateRepairState(root, health, now, result.Actions); err != nil {
		return result, err
	}

	result.MessageTitle = RepairSuccessTitle
	result.MessageBody = RepairSuccessBody
	return result, nil
}

func writeAtlasRuntimeFile(root string, target RuntimeRepairTarget, doc config.ProjectDocument) error {
	if err := configValidateWrite(target.Path); err != nil {
		return err
	}
	full := filepath.Join(root, filepath.FromSlash(target.Path))
	var existing []byte
	// Never merge non-Atlas/unmarked content into ATLAS:USER on replace/create.
	content, err := renderRepairFile(target.Path, doc, existing)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("runtime repair: create parent for %s: %w", target.Path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return fmt.Errorf("runtime repair: write %s: %w", target.Path, err)
	}
	return nil
}

func renderRepairFile(rel string, doc config.ProjectDocument, existing []byte) (string, error) {
	switch rel {
	case config.FileAgentsMD:
		return config.RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), existing), nil
	case config.FileCursorAtlasMDC:
		return config.RenderCursorAtlasMDC(doc.Project.Name), nil
	case config.FileOpenCodeAtlas:
		return config.RenderOpenCodeAtlas(doc.Project.Name), nil
	default:
		return "", fmt.Errorf("runtime repair: unsupported write path %q", rel)
	}
}

func configValidateWrite(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	switch clean {
	case config.FileAgentsMD, config.FileCursorAtlasMDC, config.FileOpenCodeAtlas:
		return nil
	default:
		return fmt.Errorf("runtime repair: refused write path %q", rel)
	}
}

func removeConflict(root, rel string) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("runtime repair: stat %s: %w", rel, err)
	}
	if info.IsDir() {
		if err := os.RemoveAll(full); err != nil {
			return fmt.Errorf("runtime repair: quarantine %s: %w", rel, err)
		}
		return nil
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("runtime repair: quarantine %s: %w", rel, err)
	}
	return nil
}

func updateRepairState(root string, health RuntimeHealth, now time.Time, actions []string) error {
	stamp := now.Format(time.RFC3339)
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

	path := filepath.Join(root, config.FileState)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("runtime repair: create state parent: %w", err)
	}
	data, err := yaml.Marshal(&state)
	if err != nil {
		return fmt.Errorf("runtime repair: marshal state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("runtime repair: write state: %w", err)
	}
	return nil
}
