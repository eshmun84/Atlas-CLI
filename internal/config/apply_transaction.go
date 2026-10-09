package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// initMutationSnapshot captures project files/dirs and global Home mirror state
// so a later Init failure can restore the pre-Apply baseline.
type initMutationSnapshot struct {
	files           []fsafety.FileSnapshot
	dirsExisted     map[string]bool
	homePath        string
	projectID       string
	homeDataExisted bool
	homeStaging     home.ProjectResetStaging
	ownershipRaw    []byte
	ownershipExists bool
	ownershipMode   os.FileMode
	mirror          home.MirrorSnapshot
	footprint       fsafety.TransactionFootprint
	homeFootprint   fsafety.TransactionFootprint
}

// Atlas workspace directory candidates that Init may create. Tracked for
// baseline dir existence (not removal authority — see footprint.Dirs).
var initDirCandidates = []string{
	".atlas/contracts",
	".atlas",
	".cursor/agents",
	".cursor/rules",
	".cursor",
	".opencode/agents",
	".opencode",
	"docs/atlas",
	"docs",
}

func captureInitMutationSnapshot(root, homePath, projectID string, targets []string, atlasRels []string) (initMutationSnapshot, error) {
	snap := initMutationSnapshot{
		homePath:    homePath,
		projectID:   projectID,
		dirsExisted: map[string]bool{},
	}
	if homePath != "" && projectID != "" {
		present, err := home.InspectProjectDataPresence(homePath, projectID)
		if err != nil {
			return snap, fmt.Errorf("home project data: %w", err)
		}
		snap.homeDataExisted = present
		exists, raw, mode, ownErr := captureOwnershipBaseline(homePath, projectID)
		if ownErr != nil {
			return snap, fmt.Errorf("ownership snapshot: %w", ownErr)
		}
		snap.ownershipExists = exists
		snap.ownershipRaw = raw
		snap.ownershipMode = mode
	}
	for _, rel := range initDirCandidates {
		info, err := fsafety.LstatContained(root, rel)
		switch {
		case os.IsNotExist(err):
			snap.dirsExisted[rel] = false
		case err != nil:
			return snap, fmt.Errorf("workspace dir %s: %w", rel, err)
		case !info.IsDir():
			return snap, fmt.Errorf("workspace dir %s: not a directory", rel)
		default:
			snap.dirsExisted[rel] = true
		}
	}
	mirror, err := home.CaptureMirrorSnapshot(homePath)
	if err != nil {
		return snap, err
	}
	snap.mirror = mirror

	seen := map[string]struct{}{}
	add := func(rel string) error {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "" || rel == "." {
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
	for _, rel := range atlasRels {
		if err := add(rel); err != nil {
			return snap, err
		}
	}
	for _, rel := range targets {
		if err := add(rel); err != nil {
			return snap, err
		}
	}
	for _, rel := range []string{".cursor/mcp.json", "opencode.json", FileProjectDocsREADME} {
		if err := add(rel); err != nil {
			return snap, err
		}
	}
	return snap, nil
}

func restoreInitFiles(root string, snap initMutationSnapshot) error {
	var errs []string

	// A. Files first (stable Rel order).
	files := append([]fsafety.FileSnapshot(nil), snap.files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	for _, fs := range files {
		wrote := snap.footprint.FileByRel(fs.Rel)
		if err := fsafety.RestoreFileSnapshot(root, fs, wrote, snap.footprint.DeletedByRel(fs.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fs.Rel, err))
		}
	}

	// B. Created workspace dirs deepest-first (child conflict blocks parents).
	if err := fsafety.RestoreCreatedDirs(root, snap.footprint); err != nil {
		errs = append(errs, err.Error())
	}

	if snap.homeStaging.Active {
		if err := home.RestoreProjectReset(snap.homeStaging, snap.homeFootprint); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if snap.homePath != "" && snap.projectID != "" {
		if err := restoreOwnershipBaseline(snap.homePath, snap.projectID, snap.ownershipExists, snap.ownershipRaw, snap.ownershipMode, &snap.homeFootprint); err != nil {
			errs = append(errs, err.Error())
		}
		// Footprint-scoped teardown of project Home created when baseline had no data.
		if !snap.homeDataExisted && !snap.homeStaging.Active {
			if err := teardownHomeProjectFootprint(snap.homePath, snap.projectID, snap.homeFootprint); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if err := home.RestoreMirrorSnapshot(snap.mirror); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("init rollback: %s", strings.Join(errs, "; "))
}

func teardownHomeProjectFootprint(homePath, projectID string, fp fsafety.TransactionFootprint) error {
	var errs []string
	// Files under projects/<id>/ first.
	prefix := filepath.ToSlash(filepath.Join("projects", projectID)) + "/"
	files := fp.SortedFiles()
	for _, w := range files {
		if w.Rel != strings.TrimSuffix(prefix, "/") && !strings.HasPrefix(w.Rel, prefix) {
			continue
		}
		snap := fsafety.FileSnapshot{Rel: w.Rel, Exists: false}
		ww := w
		if err := fsafety.RestoreFileSnapshot(homePath, snap, &ww, fp.DeletedByRel(w.Rel)); err != nil {
			errs = append(errs, err.Error())
		}
	}
	dirFP := fsafety.TransactionFootprint{}
	for _, d := range fp.Dirs {
		if d.Rel == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(d.Rel, prefix) || d.Rel == filepath.ToSlash(filepath.Join("projects", projectID)) {
			dirFP.AddDir(d)
		}
	}
	if err := fsafety.RestoreCreatedDirs(homePath, dirFP); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("home project footprint teardown: %s", strings.Join(errs, "; "))
}

// configureMutationSnapshot captures config/MCP/docs/ownership for Configure Apply rollback.
type configureMutationSnapshot struct {
	files           []fsafety.FileSnapshot
	homePath        string
	projectID       string
	ownershipRaw    []byte
	ownershipExists bool
	ownershipMode   os.FileMode
	footprint       fsafety.TransactionFootprint
}

func captureConfigureMutationSnapshot(root, homePath, projectID string) (configureMutationSnapshot, error) {
	snap := configureMutationSnapshot{
		homePath:  homePath,
		projectID: projectID,
	}
	if homePath != "" && projectID != "" {
		exists, raw, mode, err := captureOwnershipBaseline(homePath, projectID)
		if err != nil {
			return snap, fmt.Errorf("ownership snapshot: %w", err)
		}
		snap.ownershipExists = exists
		snap.ownershipRaw = raw
		snap.ownershipMode = mode
	}
	for _, rel := range []string{FileConfig, ".cursor/mcp.json", "opencode.json", FileProjectDocsREADME} {
		fs, err := fsafety.CaptureFileSnapshot(root, rel)
		if err != nil {
			return snap, err
		}
		snap.files = append(snap.files, fs)
	}
	return snap, nil
}

func restoreConfigureFiles(root string, snap configureMutationSnapshot) error {
	var errs []string
	files := append([]fsafety.FileSnapshot(nil), snap.files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	for _, fs := range files {
		wrote := snap.footprint.FileByRel(fs.Rel)
		if err := fsafety.RestoreFileSnapshot(root, fs, wrote, snap.footprint.DeletedByRel(fs.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fs.Rel, err))
		}
	}
	if err := fsafety.RestoreCreatedDirs(root, snap.footprint); err != nil {
		errs = append(errs, err.Error())
	}
	if snap.homePath != "" && snap.projectID != "" {
		if err := restoreOwnershipBaseline(snap.homePath, snap.projectID, snap.ownershipExists, snap.ownershipRaw, snap.ownershipMode, &snap.footprint); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("configure rollback: %s", strings.Join(errs, "; "))
}

func captureOwnershipBaseline(homePath, projectID string) (exists bool, raw []byte, mode os.FileMode, err error) {
	if homePath == "" || projectID == "" {
		return false, nil, 0, nil
	}
	rel := mcp.OwnershipRelPath(projectID)
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, 0, nil
		}
		return false, nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return false, nil, 0, fmt.Errorf("not a regular file: %s", rel)
	}
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		return false, nil, 0, err
	}
	return true, append([]byte(nil), data...), info.Mode().Perm(), nil
}

func restoreOwnershipBaseline(homePath, projectID string, exists bool, raw []byte, mode os.FileMode, fp *fsafety.TransactionFootprint) error {
	if homePath == "" || projectID == "" {
		return nil
	}
	rel := mcp.OwnershipRelPath(projectID)
	baseline := fsafety.FileSnapshot{
		Rel:     rel,
		Exists:  exists,
		Data:    append([]byte(nil), raw...),
		Mode:    mode,
		WasFile: true,
	}
	wrote := (*fsafety.WrittenFile)(nil)
	var deleted *fsafety.DeletedFile
	if fp != nil {
		wrote = fp.FileByRel(rel)
		deleted = fp.DeletedByRel(rel)
	}
	if err := fsafety.RestoreFileSnapshot(homePath, baseline, wrote, deleted); err != nil {
		return fmt.Errorf("ownership restore: %w", err)
	}
	return nil
}

// recordOwnershipWriteFootprint captures post-write identity for ownership.yaml.
func recordOwnershipWriteFootprint(fp *fsafety.TransactionFootprint, homePath, projectID string, created bool) error {
	if fp == nil || homePath == "" || projectID == "" {
		return nil
	}
	rel := mcp.OwnershipRelPath(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		return err
	}
	fp.AddFile(fsafety.WrittenFile{
		Rel:      rel,
		Created:  created,
		Expected: append([]byte(nil), data...),
		Mode:     info.Mode().Perm(),
		Info:     info,
	})
	return nil
}

func recordTrackedWrite(fp *fsafety.TransactionFootprint, root, rel string, data []byte, perm os.FileMode) error {
	w, dirs, err := fsafety.AtomicWriteContainedTracked(root, rel, data, perm, 0o755, ".atlas-write-*.tmp")
	// Record successful sub-mutations even when the write itself fails.
	fp.MergeDirs(dirs)
	if err != nil {
		return err
	}
	fp.AddFile(w)
	return nil
}

func recordTrackedMkdir(fp *fsafety.TransactionFootprint, root, rel string, perm os.FileMode) error {
	dirs, err := fsafety.SafeMkdirAllCreated(root, rel, perm)
	fp.MergeDirs(dirs)
	return err
}

// PreflightMCPProjections runs a dry-run MCP reconcile to surface blockers before
// irreversible Init mutations.
func PreflightMCPProjections(root string, doc ProjectDocument, draftMCP MCPDraft) error {
	root = strings.TrimSpace(root)
	if root == "" || root == "." {
		return fmt.Errorf("mcp preflight: workspace root is required")
	}
	desired, err := draftMCP.DesiredState()
	if err != nil {
		return fmt.Errorf("mcp preflight: desired state: %w", err)
	}
	homePath, err := home.Resolve()
	if err != nil {
		return fmt.Errorf("mcp preflight: atlas home: %w", err)
	}
	projectID, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		return fmt.Errorf("mcp preflight: project id: %w", err)
	}
	ownership, err := mcp.LoadOwnership(homePath, projectID)
	if err != nil {
		return err
	}
	res, err := mcp.Reconcile(mcp.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Desired:    desired,
		Adapters:   SelectedMCPAdapters(doc.Adapters.Selected),
		Projectors: MCPProjectors(),
		Ownership:  ownership,
		DryRun:     true,
	})
	if err != nil {
		return fmt.Errorf("mcp preflight: %w", err)
	}
	if res.Blocked {
		return fmt.Errorf("mcp preflight blocked: %s", strings.Join(res.Errors, "; "))
	}
	return nil
}

// ensureProjectDocsScaffoldTracked writes the docs scaffold when selected and records footprint.
func ensureProjectDocsScaffoldTracked(fp *fsafety.TransactionFootprint, root string, selected bool) (created, skipped []string, err error) {
	if !selected {
		return nil, nil, nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, nil, fmt.Errorf("project docs: workspace root is required")
	}
	rel := FileProjectDocsREADME
	full, joinErr := fsafety.ContainedJoin(root, rel)
	if joinErr != nil {
		return nil, nil, fmt.Errorf("project docs: %w", joinErr)
	}
	if info, statErr := os.Lstat(full); statErr == nil {
		if info.IsDir() {
			return nil, nil, fmt.Errorf("project docs: %s exists as a directory", rel)
		}
		return nil, []string{rel}, nil
	} else if !os.IsNotExist(statErr) {
		return nil, nil, fmt.Errorf("project docs: stat %s: %w", rel, statErr)
	}
	if err := recordTrackedWrite(fp, root, rel, []byte(ProjectDocsScaffoldContent), 0o644); err != nil {
		return nil, nil, fmt.Errorf("project docs: write %s: %w", rel, err)
	}
	return []string{rel}, nil, nil
}

// preflightDocsScaffold verifies docs/atlas/README.md can be created safely when selected.
func preflightDocsScaffold(root string, selected bool) error {
	if !selected {
		return nil
	}
	rel := FileProjectDocsREADME
	full, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return fmt.Errorf("project docs preflight: %w", err)
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("project docs preflight: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("project docs preflight: %s exists as a directory", rel)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("project docs preflight: %s is a symlink", rel)
	}
	return nil
}
