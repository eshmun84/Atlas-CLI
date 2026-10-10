package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

const (
	ApplySuccessTitle = "Atlas configuration and runtime initialized."
	ApplySuccessBody  = "Runtime files materialized."
	// ConfigureApplySuccess is the short notice title; prefer FormatConfigureNotice for full copy.
	ConfigureApplySuccess = ConfigureApplySuccessTitle
)

// afterSkillsReconcileHook is an optional test seam invoked after successful
// skill projection reconcile during ApplyConfig / PersistConfigure.
// Production code leaves it nil.
var afterSkillsReconcileHook func() error

// Paths Apply must never create, modify, backup, replace, or delete.
var forbiddenApplyRelPaths = []string{
	".agents",
	".claude",
	"CLAUDE.md",
	"GEMINI.md",
	"README.md",
	".gitignore",
	FileMemoryDB,
	FileCapsule,
}

// ApplyInput is the in-memory init state used to persist Atlas config and runtime files.
type ApplyInput struct {
	Root            string
	Draft           ConfigDraft
	MCP             MCPDraft
	Now             func() time.Time
	AcceptHomeReset bool
}

// ApplyResult lists Atlas-owned and runtime paths created by Apply.
type ApplyResult struct {
	Directories  []string
	Files        []string
	RuntimeFiles []string
	BackupDir    string
	HomePath     string
	HomeCreated  bool
	ProjectID    string
	HomeReset    bool
}

// ApplyConfig persists Atlas-owned configuration under .atlas/ and materializes
// allowlisted runtime entrypoints. It never modifies Git and never stores secrets.
//
// Phase A (read-only): validate, preflight MCP/docs, human Home-reset gate, render,
// and snapshot. Zero mutation until Phase B.
// Phase B (transaction): every error restores workspace files and staged Home project data.
func ApplyConfig(in ApplyInput) (ApplyResult, error) {
	root := filepath.Clean(strings.TrimSpace(in.Root))
	if root == "" || root == "." {
		return ApplyResult{}, fmt.Errorf("apply config: workspace root is required")
	}

	// Pure input prepare (no protected-state decisions).
	doc := BuildProjectDocument(in.Draft, in.MCP)
	if err := ValidateProjectDocument(doc); err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}
	now := time.Now().UTC()
	if in.Now != nil {
		now = in.Now().UTC()
	}
	targets := RuntimeTargets(doc)

	homePath, err := home.Resolve()
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}

	lockSet, err := mutatelock.Acquire(mutatelock.Options{HomePath: homePath, Workspace: root})
	if err != nil {
		return ApplyResult{HomePath: homePath}, fmt.Errorf("apply config: %w", err)
	}
	defer func() { _ = lockSet.Release() }()

	// ---------- Locked Phase A: read protected state → decide → snapshot ----------
	projectID, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: project id: %w", err)
	}

	needsHomeReset, inspectErr := home.InspectProjectDataPresence(homePath, projectID)
	if inspectErr != nil {
		// Fail closed: never assume "no data" when inspection is uncertain.
		return ApplyResult{HomePath: homePath, ProjectID: projectID},
			fmt.Errorf("apply config: inspect home project data: %w", inspectErr)
	}
	if needsHomeReset && !in.AcceptHomeReset {
		// Human gate from Home state observed under lock, before mutation.
		return ApplyResult{HomePath: homePath, ProjectID: projectID},
			fmt.Errorf("apply config: Atlas Home project data exists for this project; explicit reset acceptance is required")
	}

	if err := validateRuntimeTargets(root, targets); err != nil {
		return ApplyResult{}, err
	}
	if err := PreflightMCPProjections(root, doc, in.MCP); err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}
	if err := preflightDocsScaffold(root, doc.Project.DocsScaffold); err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}

	atlasRels := []string{
		FileConfig, FileLocal, FileAssetsLock, FileAgentRegistry, FileRuntimeManifest, FileSkillRegistry, FileState,
	}
	if DependsOnSDDOpenSpecContract(doc) {
		atlasRels = append(atlasRels, FileSDDOpenSpecContract)
	}

	// Pre-render write payloads from locked filesystem observations.
	// Skill registry is rendered after EnsureAndMirror so Home digests match.
	registry := RenderAgentRegistry(doc.Project.Name, doc.Adapters.Selected, homePath)
	manifest, err := RenderRuntimeManifestYAML(doc.Project.Name, doc.Adapters.Selected)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: render %s: %w", FileRuntimeManifest, err)
	}
	lockDoc, err := BuildAssetsLockDocumentFor(homePath, doc, version.Version)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: build assets lock: %w", err)
	}
	type atlasWrite struct {
		rel  string
		data []byte
	}
	var atlasFiles []atlasWrite
	for _, item := range []struct {
		rel  string
		data any
	}{
		{rel: FileConfig, data: doc},
		{rel: FileLocal, data: BuildLocalDocument(in.Draft)},
		{rel: FileAssetsLock, data: lockDoc},
	} {
		payload, marshalErr := marshalYAML(item.data)
		if marshalErr != nil {
			return ApplyResult{}, fmt.Errorf("apply config: marshal %s: %w", item.rel, marshalErr)
		}
		atlasFiles = append(atlasFiles, atlasWrite{rel: item.rel, data: payload})
	}
	atlasFiles = append(atlasFiles,
		atlasWrite{rel: FileAgentRegistry, data: []byte(registry)},
		atlasWrite{rel: FileRuntimeManifest, data: []byte(manifest)},
	)
	if DependsOnSDDOpenSpecContract(doc) {
		contract, renderErr := RenderSDDOpenSpecContract()
		if renderErr != nil {
			return ApplyResult{}, fmt.Errorf("apply config: render %s: %w", FileSDDOpenSpecContract, renderErr)
		}
		atlasFiles = append(atlasFiles, atlasWrite{rel: FileSDDOpenSpecContract, data: []byte(contract)})
	}
	state := BuildStateDocument(in.Draft, now.Format(time.RFC3339), true)
	stateData, err := marshalYAML(state)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: marshal %s: %w", FileState, err)
	}

	type runtimeWrite struct {
		rel     string
		content string
	}
	var runtimeFiles []runtimeWrite
	for _, rel := range targets {
		full, joinErr := safeJoinRuntime(root, rel)
		if joinErr != nil {
			return ApplyResult{}, joinErr
		}
		var existing []byte
		if data, readErr := os.ReadFile(full); readErr == nil {
			existing = data
		} else if !os.IsNotExist(readErr) {
			return ApplyResult{}, fmt.Errorf("apply config: read %s: %w", rel, readErr)
		}
		content, renderErr := renderRuntimeFile(rel, doc, existing)
		if renderErr != nil {
			return ApplyResult{}, fmt.Errorf("apply config: %w", renderErr)
		}
		runtimeFiles = append(runtimeFiles, runtimeWrite{rel: rel, content: content})
	}
	for _, file := range atlasFiles {
		if _, joinErr := safeJoinAtlas(root, file.rel); joinErr != nil {
			return ApplyResult{}, joinErr
		}
	}
	if _, err := safeJoinAtlas(root, FileState); err != nil {
		return ApplyResult{}, err
	}

	baseline, err := captureInitMutationSnapshot(root, homePath, projectID, targets, atlasRels)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: snapshot: %w", err)
	}

	// ---------- Phase B: mutation transaction ----------
	rollback := func(cause error) (ApplyResult, error) {
		if restoreErr := restoreInitFiles(root, baseline); restoreErr != nil {
			return ApplyResult{}, fmt.Errorf("apply config: %v (rollback failed: %v)", cause, restoreErr)
		}
		// Restores project workspace tree + project Home staging + global Home mirror
		// snapshot captured before EnsureAndMirror.
		return ApplyResult{}, fmt.Errorf("apply config: %w (rolled back project workspace and Atlas Home mirror baseline)", cause)
	}

	homeResult, err := home.EnsureAndMirror(now)
	// Merge partial Home footprint even when EnsureAndMirror fails mid-mutation.
	baseline.mirror.Footprint = homeResult.Footprint
	baseline.homeFootprint.MergeFootprint(homeResult.Footprint)
	if err != nil {
		return rollback(err)
	}
	result := ApplyResult{
		HomePath:    homeResult.HomePath,
		HomeCreated: homeResult.Created,
		ProjectID:   projectID,
	}

	if needsHomeReset {
		stamp := mcp.NewBackupStamp(now)
		staging, stageErr := home.StageProjectReset(homeResult.HomePath, projectID, stamp)
		if stageErr != nil {
			return rollback(stageErr)
		}
		baseline.homeStaging = staging
		result.HomeReset = staging.Active
	}

	layoutDirs, err := home.EnsureProjectLayoutCreated(homeResult.HomePath, projectID)
	baseline.homeFootprint.MergeDirs(layoutDirs)
	if err != nil {
		return rollback(err)
	}
	idWrote, idDirs, idErr := home.WriteProjectIdentityTracked(homeResult.HomePath, projectID, doc.Project.Name, root, now)
	baseline.homeFootprint.MergeDirs(idDirs)
	if idErr != nil {
		return rollback(idErr)
	}
	baseline.homeFootprint.AddFile(idWrote)

	if err := recordTrackedMkdir(&baseline.footprint, root, DirAtlas, 0o755); err != nil {
		return rollback(fmt.Errorf("create %s: %w", DirAtlas, err))
	}
	result.Directories = append(result.Directories, DirAtlas)

	backupDir, _, backupFP, err := BackupExistingTargets(root, homeResult.HomePath, projectID, targets, now)
	baseline.homeFootprint.MergeFootprint(backupFP)
	if err != nil {
		return rollback(err)
	}
	result.BackupDir = backupDir

	for _, file := range atlasFiles {
		if err := recordTrackedWrite(&baseline.footprint, root, file.rel, file.data, 0o644); err != nil {
			return rollback(fmt.Errorf("write %s: %w", file.rel, err))
		}
		result.Files = append(result.Files, file.rel)
	}
	for _, file := range runtimeFiles {
		if err := recordTrackedWrite(&baseline.footprint, root, file.rel, []byte(file.content), 0o644); err != nil {
			return rollback(fmt.Errorf("write %s: %w", file.rel, err))
		}
		result.RuntimeFiles = append(result.RuntimeFiles, file.rel)
	}

	if _, err := ReconcileMCPProjections(root, doc, in.MCP); err != nil {
		return rollback(fmt.Errorf("mcp materialization failed: %w", err))
	}
	if err := recordOwnershipWriteFootprint(&baseline.homeFootprint, homeResult.HomePath, projectID, !baseline.ownershipExists); err != nil {
		return rollback(fmt.Errorf("ownership footprint: %w", err))
	}
	skillRegistry, skillRegErr := RenderSkillRegistry(doc, homeResult.HomePath)
	if skillRegErr != nil {
		return rollback(fmt.Errorf("render %s: %w", FileSkillRegistry, skillRegErr))
	}
	if err := recordTrackedWrite(&baseline.footprint, root, FileSkillRegistry, []byte(skillRegistry), 0o644); err != nil {
		return rollback(fmt.Errorf("write %s: %w", FileSkillRegistry, err))
	}
	result.Files = append(result.Files, FileSkillRegistry)
	if skillRes, err := ReconcileSkillProjections(root, doc, result.HomeReset); err != nil {
		return rollback(fmt.Errorf("skills materialization failed: %w", err))
	} else if skillRes.Blocked {
		return rollback(fmt.Errorf("skills materialization blocked: %s", strings.Join(skillRes.Errors, "; ")))
	} else {
		baseline.skillsMutation = skillRes.Mutation
		// Fold skills Home dir footprint into Init Home footprint for teardown.
		baseline.homeFootprint.MergeFootprint(skillRes.Mutation.HomeFootprint)
		if afterSkillsReconcileHook != nil {
			if hookErr := afterSkillsReconcileHook(); hookErr != nil {
				return rollback(hookErr)
			}
		}
	}

	if err := recordTrackedWrite(&baseline.footprint, root, FileState, stateData, 0o644); err != nil {
		return rollback(fmt.Errorf("write %s: %w", FileState, err))
	}
	result.Files = append(result.Files, FileState)

	if created, _, docsErr := ensureProjectDocsScaffoldTracked(&baseline.footprint, root, doc.Project.DocsScaffold); docsErr != nil {
		return rollback(docsErr)
	} else {
		result.Files = append(result.Files, created...)
	}

	if err := home.CommitProjectReset(baseline.homeStaging); err != nil {
		return rollback(err)
	}
	return result, nil
}

// PersistConfigureResult is the outcome of an explicit Configure Apply.
type PersistConfigureResult struct {
	Impact ConfigureImpact
	Notice string
}

// PersistConfigure writes the current draft/MCP state to .atlas/config.yaml
// and reconciles Atlas-owned MCP projections when MCP/adapters change.
// It does not rematerialize general runtime assets (AGENTS.md, rules, agents)
// and does not store credentials.
// Optional project docs scaffold may be created once when selected and missing.
// Config, MCP projections/ownership, and docs are one transaction: any Apply error
// restores the pre-Apply observable baseline.
func PersistConfigure(in ApplyInput) (PersistConfigureResult, error) {
	root := filepath.Clean(strings.TrimSpace(in.Root))
	if root == "" || root == "." {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: workspace root is required")
	}

	// Pure input prepare (draft → document); do not load on-disk previous yet.
	doc := BuildProjectDocument(in.Draft, in.MCP)
	if err := ValidateProjectDocument(doc); err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: %w", err)
	}
	data, err := marshalYAML(doc)
	if err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: marshal %s: %w", FileConfig, err)
	}

	homePath, err := home.Resolve()
	if err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: %w", err)
	}

	lockSet, err := mutatelock.Acquire(mutatelock.Options{HomePath: homePath, Workspace: root})
	if err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: %w", err)
	}
	defer func() { _ = lockSet.Release() }()

	// ---------- Locked: load current config → decide → snapshot → mutate ----------
	var previous ProjectDocument
	prevFull, prevJoinErr := safeJoinAtlas(root, FileConfig)
	if prevJoinErr != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: %w", prevJoinErr)
	}
	if prevInfo, prevStatErr := os.Lstat(prevFull); prevStatErr == nil {
		// Existing config must load successfully; never treat corrupt as empty.
		if prevInfo.Mode()&os.ModeSymlink != 0 || !prevInfo.Mode().IsRegular() {
			return PersistConfigureResult{}, fmt.Errorf("persist configure: existing %s is not a safe regular file", FileConfig)
		}
		prev, loadErr := LoadProjectDocumentAt(root)
		if loadErr != nil {
			return PersistConfigureResult{}, fmt.Errorf("persist configure: existing config unreadable: %w", loadErr)
		}
		previous = prev
	} else if !os.IsNotExist(prevStatErr) {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: inspect existing config: %w", prevStatErr)
	}

	if err := preflightDocsScaffold(root, doc.Project.DocsScaffold); err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: %w", err)
	}
	projectID, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: project id: %w", err)
	}

	baseline, err := captureConfigureMutationSnapshot(root, homePath, projectID)
	if err != nil {
		return PersistConfigureResult{}, fmt.Errorf("persist configure: snapshot: %w", err)
	}

	mcpChanged := MCPChanged(previous, doc)
	adaptersChanged := JoinChips(previous.Adapters.Selected) != JoinChips(doc.Adapters.Selected)
	skillsChanged := !skillPinsEqual(previous.SkillPins(), doc.SkillPins())
	impact := AnalyzeConfigureImpact(previous, doc, mcpChanged)
	needMCP := mcpChanged || adaptersChanged || mcpPreferencePresent(doc) || mcpPreferencePresent(previous)
	needSkills := adaptersChanged || skillsChanged

	// ---------- Phase B: mutation transaction ----------
	rollback := func(cause error) (PersistConfigureResult, error) {
		if restoreErr := restoreConfigureFiles(root, baseline); restoreErr != nil {
			return PersistConfigureResult{}, fmt.Errorf("persist configure: %v (rollback failed: %v)", cause, restoreErr)
		}
		return PersistConfigureResult{}, fmt.Errorf("persist configure: %w (rolled back to baseline)", cause)
	}

	if err := recordTrackedWrite(&baseline.footprint, root, FileConfig, data, 0o644); err != nil {
		return rollback(fmt.Errorf("write %s: %w", FileConfig, err))
	}

	// Configure Apply reconciles Atlas-owned MCP and Skills projections only.
	if needMCP {
		if _, err := ReconcileMCPProjections(root, doc, in.MCP); err != nil {
			return rollback(fmt.Errorf("mcp projection: %w", err))
		}
		if err := recordOwnershipWriteFootprint(&baseline.footprint, homePath, projectID, !baseline.ownershipExists); err != nil {
			return rollback(fmt.Errorf("ownership footprint: %w", err))
		}
		impact.MCPProjected = true
		impact.MCPPreferenceOnly = false
	}
	if needSkills {
		skillReg, regErr := RenderSkillRegistry(doc, homePath)
		if regErr != nil {
			return rollback(fmt.Errorf("skills registry: %w", regErr))
		}
		if err := recordTrackedWrite(&baseline.footprint, root, FileSkillRegistry, []byte(skillReg), 0o644); err != nil {
			return rollback(fmt.Errorf("write %s: %w", FileSkillRegistry, err))
		}
		if skillRes, err := ReconcileSkillProjections(root, doc, false); err != nil {
			return rollback(fmt.Errorf("skills projection: %w", err))
		} else if skillRes.Blocked {
			return rollback(fmt.Errorf("skills projection blocked: %s", strings.Join(skillRes.Errors, "; ")))
		} else {
			baseline.skillsMutation = skillRes.Mutation
			if afterSkillsReconcileHook != nil {
				if hookErr := afterSkillsReconcileHook(); hookErr != nil {
					return rollback(hookErr)
				}
			}
		}
	}

	created, skipped, docsErr := ensureProjectDocsScaffoldTracked(&baseline.footprint, root, doc.Project.DocsScaffold)
	if docsErr != nil {
		return rollback(docsErr)
	}
	impact.DocsCreated = created
	impact.DocsSkipped = skipped
	impact.DocsScaffoldSelected = doc.Project.DocsScaffold
	impact.Lines = impact.NoticeLines()

	return PersistConfigureResult{
		Impact: impact,
		Notice: FormatConfigureNotice(impact),
	}, nil
}

func validateRuntimeTargets(root string, targets []string) error {
	for _, rel := range targets {
		full, err := safeJoinRuntime(root, rel)
		if err != nil {
			return err
		}
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("apply config: stat %s: %w", rel, err)
		}
		if info.IsDir() {
			return fmt.Errorf("apply config: %s exists as a directory; expected a file", rel)
		}
	}
	return nil
}

func marshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isAllowedRuntimePath(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	switch clean {
	case FileAgentsMD, FileCursorAtlasMDC, FileOpenCodeAtlas:
		return true
	default:
		return IsAtlasAgentRuntimePath(clean)
	}
}

func assertAllowedAtlasPath(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("apply config: refused path %q", rel)
	}
	if clean != DirAtlas && !strings.HasPrefix(clean, DirAtlas+"/") {
		return fmt.Errorf("apply config: refused non-atlas path %q", rel)
	}
	for _, forbidden := range forbiddenApplyRelPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("apply config: refused forbidden path %q", rel)
		}
	}
	return nil
}

func assertAllowedRuntimePath(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("apply config: refused path %q", rel)
	}
	if !isAllowedRuntimePath(clean) {
		return fmt.Errorf("apply config: refused runtime path %q", rel)
	}
	for _, forbidden := range forbiddenApplyRelPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("apply config: refused forbidden path %q", rel)
		}
	}
	return nil
}

func safeJoinAtlas(root, rel string) (string, error) {
	if err := assertAllowedAtlasPath(rel); err != nil {
		return "", err
	}
	return safeJoinRoot(root, rel)
}

func safeJoinRuntime(root, rel string) (string, error) {
	if err := assertAllowedRuntimePath(rel); err != nil {
		return "", err
	}
	return safeJoinRoot(root, rel)
}

func assertAllowedConflictPath(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("backup: refused path %q", rel)
	}
	allowed := []string{
		FileAgentsMD,
		FileAgentRegistry,
		FileSkillRegistry,
		FileRuntimeManifest,
		FileAssetsLock,
		FileSDDOpenSpecContract,
		"AGENT.md",
		"CLAUDE.md",
		"GEMINI.md",
		".cursor",
		".opencode",
		".agents",
		".claude",
		".codex",
	}
	for _, prefix := range allowed {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return nil
		}
	}
	return fmt.Errorf("backup: refused conflict path %q", rel)
}

func safeJoinRoot(root, rel string) (string, error) {
	full, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return "", fmt.Errorf("apply config: %w", err)
	}
	return full, nil
}
