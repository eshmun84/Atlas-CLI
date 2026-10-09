package home

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// Project-scoped Atlas Home layout under $ATLAS_HOME/projects/<project-id>/.
const (
	DirProjects = "projects"

	ProjectDirState       = "state"
	ProjectDirContext     = "context"
	ProjectDirMemory      = "memory"
	ProjectDirBackups     = "backups"
	ProjectDirDiagnostics = "diagnostics"
	ProjectDirLocal       = "local"

	FileProjectIdentity = "local/identity.yaml"
	FileProjectLocal    = "state/local.yaml"
)

// ProjectLayoutDirectories are created under each Home project root by mutating flows.
var ProjectLayoutDirectories = []string{
	ProjectDirState,
	ProjectDirContext,
	ProjectDirMemory,
	ProjectDirBackups,
	ProjectDirDiagnostics,
	ProjectDirLocal,
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// ProjectIdentity is machine-local metadata for one Home project.
// Absolute paths are allowed here; they must never be written into portable project config.
type ProjectIdentity struct {
	SchemaVersion   int    `yaml:"schema_version"`
	ProjectID       string `yaml:"project_id"`
	ProjectName     string `yaml:"project_name,omitempty"`
	RootFingerprint string `yaml:"root_fingerprint"`
	// RootPath is machine-local diagnostic metadata only (Atlas Home, not portable).
	RootPath  string `yaml:"root_path,omitempty"`
	CreatedAt string `yaml:"created_at,omitempty"`
	UpdatedAt string `yaml:"updated_at,omitempty"`
}

// ProjectLocalState holds machine-local runtime events for one Home project.
// Transitional: portable .atlas/state.yaml may still carry compatibility mirrors.
type ProjectLocalState struct {
	SchemaVersion int `yaml:"schema_version"`

	ContextEconomyUpdatedAt   string `yaml:"context_economy_updated_at,omitempty"`
	ContextEconomyFingerprint string `yaml:"context_economy_fingerprint,omitempty"`

	RuntimeRepairedAt        string   `yaml:"runtime_repaired_at,omitempty"`
	LastRuntimeRepairActions []string `yaml:"last_runtime_repair_actions,omitempty"`
}

const projectIdentitySchema = 1
const projectLocalSchema = 1

// CanonicalRoot returns the cleaned absolute project root used for identity.
func CanonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", fmt.Errorf("atlas home: project root: %w", err)
	}
	return filepath.Clean(abs), nil
}

// RootFingerprint returns sha256 hex of the canonical absolute project root.
func RootFingerprint(root string) (string, error) {
	abs, err := CanonicalRoot(root)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:]), nil
}

// ProjectID returns a deterministic Home project identity from the canonical root.
// Format: <sanitized-basename>-<sha256(absRoot)[:16]>.
// Same name + different root always yields a different ID. Name alone is never sufficient.
func ProjectID(root, projectName string) (string, error) {
	abs, err := CanonicalRoot(root)
	if err != nil {
		return "", err
	}
	fp, err := RootFingerprint(abs)
	if err != nil {
		return "", err
	}
	hash := fp[:16]

	base := strings.TrimSpace(projectName)
	if base == "" {
		base = filepath.Base(abs)
	}
	base = strings.ToLower(base)
	base = nonID.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "project"
	}
	if len(base) > 40 {
		base = base[:40]
		base = strings.Trim(base, "-")
	}
	return base + "-" + hash, nil
}

// ProjectsRoot returns $ATLAS_HOME/projects.
func ProjectsRoot(homePath string) string {
	return filepath.Join(homePath, DirProjects)
}

// ProjectRoot returns $ATLAS_HOME/projects/<project-id>.
func ProjectRoot(homePath, projectID string) string {
	return filepath.Join(ProjectsRoot(homePath), projectID)
}

// ProjectContextDir returns $ATLAS_HOME/projects/<project-id>/context.
func ProjectContextDir(homePath, projectID string) string {
	return filepath.Join(ProjectRoot(homePath, projectID), ProjectDirContext)
}

// ProjectBackupsDir returns $ATLAS_HOME/projects/<project-id>/backups.
func ProjectBackupsDir(homePath, projectID string) string {
	return filepath.Join(ProjectRoot(homePath, projectID), ProjectDirBackups)
}

// ProjectIdentityPath returns the local identity metadata path.
func ProjectIdentityPath(homePath, projectID string) string {
	return filepath.Join(ProjectRoot(homePath, projectID), filepath.FromSlash(FileProjectIdentity))
}

// ProjectLocalStatePath returns the machine-local project state path.
func ProjectLocalStatePath(homePath, projectID string) string {
	return filepath.Join(ProjectRoot(homePath, projectID), filepath.FromSlash(FileProjectLocal))
}

// InspectProjectDataPresence reports whether Home already holds data for this project ID.
// Read-only: never creates directories. Detection is by project ID, never by name alone.
//
// Semantics:
//   - missing project root → false, nil
//   - real empty project dir → false, nil
//   - non-empty project dir → true, nil
//   - unexpected stat/read error → false, error
//   - symlink / unsafe containment → false, error
func InspectProjectDataPresence(homePath, projectID string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return false, nil
	}
	rel := filepath.ToSlash(filepath.Join(DirProjects, projectID))
	root, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return false, err
	}
	info, err := lstatFn(root)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("atlas home: project root is a symlink: %s", projectID)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("atlas home: project root is not a directory: %s", projectID)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

// ProjectDataPresent is a best-effort read-only helper for Status/review UIs.
// On inspection uncertainty it returns false (unknown treated as absent for display).
// Mutating flows MUST use InspectProjectDataPresence and fail closed on error.
func ProjectDataPresent(homePath, projectID string) bool {
	present, err := InspectProjectDataPresence(homePath, projectID)
	if err != nil {
		return false
	}
	return present
}

// InspectProject returns a read-only snapshot of one Home project directory.
// Presence flags use Home-contained Lstat/read-dir only; symlink/external
// targets are never followed or counted as present.
func InspectProject(homePath, projectID string) ProjectStatus {
	status := ProjectStatus{
		HomePath:        homePath,
		ProjectID:       projectID,
		Root:            ProjectRoot(homePath, projectID),
		IntegrityErrors: []string{},
	}
	present, err := InspectProjectDataPresence(homePath, projectID)
	if err != nil {
		status.IntegrityErrors = append(status.IntegrityErrors, err.Error())
		return status
	}
	if !present {
		return status
	}
	status.Present = true

	ctxRel := filepath.ToSlash(filepath.Join(DirProjects, projectID, ProjectDirContext))
	if ok, ierr := containedDirNonEmpty(homePath, ctxRel); ierr != nil {
		status.IntegrityErrors = append(status.IntegrityErrors, "context: "+ierr.Error())
	} else {
		status.ContextPresent = ok
	}

	bakRel := filepath.ToSlash(filepath.Join(DirProjects, projectID, ProjectDirBackups))
	if ok, ierr := containedDirExists(homePath, bakRel); ierr != nil {
		status.IntegrityErrors = append(status.IntegrityErrors, "backups: "+ierr.Error())
	} else {
		status.BackupsPresent = ok
	}

	if ok, ierr := containedRegularPresent(homePath, ProjectLocalStateRelPath(projectID)); ierr != nil {
		status.IntegrityErrors = append(status.IntegrityErrors, "local state: "+ierr.Error())
	} else {
		status.LocalStatePresent = ok
	}

	if ok, ierr := containedRegularPresent(homePath, ProjectIdentityRelPath(projectID)); ierr != nil {
		status.IntegrityErrors = append(status.IntegrityErrors, "identity: "+ierr.Error())
	} else {
		status.IdentityPresent = ok
		if ok {
			id, loaded, loadErr := LoadProjectIdentity(homePath, projectID)
			if loadErr != nil {
				status.IdentityPresent = false
				status.IntegrityErrors = append(status.IntegrityErrors, "identity: "+loadErr.Error())
			} else if loaded {
				status.Identity = id
			}
		}
	}
	return status
}

// ProjectStatus is a read-only Home project snapshot.
type ProjectStatus struct {
	HomePath          string
	ProjectID         string
	Root              string
	Present           bool
	ContextPresent    bool
	BackupsPresent    bool
	LocalStatePresent bool
	IdentityPresent   bool
	Identity          ProjectIdentity
	// IntegrityErrors reports unsafe/symlink/non-regular project-local paths.
	IntegrityErrors []string
}

// EnsureProjectLayout creates the project-scoped Home directories. Mutating.
func EnsureProjectLayout(homePath, projectID string) error {
	_, err := EnsureProjectLayoutCreated(homePath, projectID)
	return err
}

// EnsureProjectLayoutCreated is EnsureProjectLayout plus CreatedDir footprints
// for directories this call newly created.
func EnsureProjectLayoutCreated(homePath, projectID string) ([]fsafety.CreatedDir, error) {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return nil, fmt.Errorf("atlas home: project layout requires home path and project id")
	}
	var created []fsafety.CreatedDir
	relRoot := filepath.ToSlash(filepath.Join(DirProjects, projectID))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, relRoot, DirPermHome)
	created = append(created, dirs...)
	if err != nil {
		return created, fmt.Errorf("atlas home: create project root: %w", err)
	}
	if err := chmodRealDir(ProjectRoot(homePath, projectID), DirPermHome); err != nil {
		return created, err
	}
	for _, dir := range ProjectLayoutDirectories {
		rel := filepath.ToSlash(filepath.Join(DirProjects, projectID, dir))
		dirs, err := fsafety.SafeMkdirAllCreated(homePath, rel, DirPermHome)
		created = append(created, dirs...)
		if err != nil {
			return created, fmt.Errorf("atlas home: create project %s: %w", dir, err)
		}
		if err := chmodRealDir(filepath.Join(homePath, filepath.FromSlash(rel)), DirPermHome); err != nil {
			return created, err
		}
	}
	// codegraph/ and mcp/ are created by their own writers; harden when present.
	for _, extra := range []string{"codegraph", "mcp"} {
		abs := filepath.Join(ProjectRoot(homePath, projectID), extra)
		info, err := lstatFn(abs)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return created, fmt.Errorf("atlas home: lstat project %s: %w", extra, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return created, fmt.Errorf("atlas home: refusing chmod through symlink projects/%s/%s", projectID, extra)
		}
		if !info.IsDir() {
			return created, fmt.Errorf("atlas home: project %s is not a directory", extra)
		}
		if err := chmodFn(abs, DirPermHome); err != nil {
			return created, fmt.Errorf("atlas home: chmod project %s: %w", extra, err)
		}
	}
	return created, nil
}

// WriteProjectIdentity persists machine-local identity metadata under Atlas Home.
func WriteProjectIdentity(homePath, projectID, projectName, root string, now time.Time) error {
	_, _, err := WriteProjectIdentityTracked(homePath, projectID, projectName, root, now)
	return err
}

// WriteProjectIdentityTracked is WriteProjectIdentity plus write footprints.
func WriteProjectIdentityTracked(homePath, projectID, projectName, root string, now time.Time) (fsafety.WrittenFile, []fsafety.CreatedDir, error) {
	abs, err := CanonicalRoot(root)
	if err != nil {
		return fsafety.WrittenFile{}, nil, err
	}
	fp, err := RootFingerprint(abs)
	if err != nil {
		return fsafety.WrittenFile{}, nil, err
	}
	existing, present, err := LoadProjectIdentity(homePath, projectID)
	if err != nil {
		return fsafety.WrittenFile{}, nil, err
	}
	createdAt := stamp(now)
	if present && existing.CreatedAt != "" {
		createdAt = existing.CreatedAt
	}
	doc := ProjectIdentity{
		SchemaVersion:   projectIdentitySchema,
		ProjectID:       projectID,
		ProjectName:     strings.TrimSpace(projectName),
		RootFingerprint: fp,
		RootPath:        abs,
		CreatedAt:       createdAt,
		UpdatedAt:       stamp(now),
	}
	rel := filepath.ToSlash(filepath.Join(DirProjects, projectID, FileProjectIdentity))
	return writeHomeYAMLTracked(homePath, rel, doc, 0o600)
}

// ProjectIdentityRelPath returns projects/<id>/local/identity.yaml relative to Home.
func ProjectIdentityRelPath(projectID string) string {
	return filepath.ToSlash(filepath.Join(DirProjects, projectID, FileProjectIdentity))
}

// ProjectLocalStateRelPath returns projects/<id>/state/local.yaml relative to Home.
func ProjectLocalStateRelPath(projectID string) string {
	return filepath.ToSlash(filepath.Join(DirProjects, projectID, FileProjectLocal))
}

// LoadProjectIdentity reads local identity via symlink-safe contained Home read.
// Missing => false,nil. Symlink/unsafe leaf => error (external bytes never parsed).
func LoadProjectIdentity(homePath, projectID string) (ProjectIdentity, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return ProjectIdentity{}, false, nil
	}
	data, err := fsafety.ReadFileContained(homePath, ProjectIdentityRelPath(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectIdentity{}, false, nil
		}
		return ProjectIdentity{}, false, fmt.Errorf("atlas home: read identity: %w", err)
	}
	var doc ProjectIdentity
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return ProjectIdentity{}, false, fmt.Errorf("atlas home: parse identity: %w", err)
	}
	return doc, true, nil
}

// LoadProjectLocalState reads machine-local project state via symlink-safe
// contained Home read. Missing => false,nil. Symlink/unsafe => error.
func LoadProjectLocalState(homePath, projectID string) (ProjectLocalState, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return ProjectLocalState{}, false, nil
	}
	data, err := fsafety.ReadFileContained(homePath, ProjectLocalStateRelPath(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectLocalState{}, false, nil
		}
		return ProjectLocalState{}, false, fmt.Errorf("atlas home: read local state: %w", err)
	}
	var doc ProjectLocalState
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return ProjectLocalState{}, false, fmt.Errorf("atlas home: parse local state: %w", err)
	}
	return doc, true, nil
}

// WriteProjectLocalState persists machine-local runtime state under Atlas Home.
func WriteProjectLocalState(homePath, projectID string, doc ProjectLocalState) error {
	_, _, err := WriteProjectLocalStateTracked(homePath, projectID, doc)
	return err
}

// WriteProjectLocalStateTracked is WriteProjectLocalState plus write footprints.
func WriteProjectLocalStateTracked(homePath, projectID string, doc ProjectLocalState) (fsafety.WrittenFile, []fsafety.CreatedDir, error) {
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = projectLocalSchema
	}
	rel := filepath.ToSlash(filepath.Join(DirProjects, projectID, FileProjectLocal))
	return writeHomeYAMLTracked(homePath, rel, doc, 0o600)
}

// ProjectResetStaging holds a staged rename of projects/<id> for transactional Init reset.
type ProjectResetStaging struct {
	HomePath   string
	ProjectID  string
	StagingRel string // relative to Home, e.g. projects/.atlas-init-tx-<stamp>/<id>
	Active     bool
}

// StageProjectReset renames projects/<id> aside instead of deleting it, so a later
// failure can restore the exact prior Home project tree.
func StageProjectReset(homePath, projectID, stamp string) (ProjectResetStaging, error) {
	projectID = strings.TrimSpace(projectID)
	stamp = strings.TrimSpace(stamp)
	if homePath == "" || projectID == "" || stamp == "" {
		return ProjectResetStaging{}, fmt.Errorf("atlas home: stage reset requires home path, project id, and stamp")
	}
	if strings.Contains(projectID, "..") || strings.ContainsAny(projectID, `/\`) {
		return ProjectResetStaging{}, fmt.Errorf("atlas home: refused unsafe project id %q", projectID)
	}
	if _, err := fsafety.ContainedJoin(homePath, filepath.ToSlash(filepath.Join(DirProjects, projectID))); err != nil {
		return ProjectResetStaging{}, fmt.Errorf("atlas home: stage reset: %w", err)
	}
	target := ProjectRoot(homePath, projectID)
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return ProjectResetStaging{HomePath: homePath, ProjectID: projectID}, nil
	} else if err != nil {
		return ProjectResetStaging{}, err
	}
	stagingRel := filepath.ToSlash(filepath.Join(DirProjects, ".atlas-init-tx-"+stamp, projectID))
	if err := fsafety.SafeMkdirAll(homePath, filepath.ToSlash(filepath.Join(DirProjects, ".atlas-init-tx-"+stamp)), 0o700); err != nil {
		return ProjectResetStaging{}, fmt.Errorf("atlas home: stage reset mkdir: %w", err)
	}
	stagingAbs := filepath.Join(homePath, filepath.FromSlash(stagingRel))
	if err := os.Rename(target, stagingAbs); err != nil {
		return ProjectResetStaging{}, fmt.Errorf("atlas home: stage reset rename: %w", err)
	}
	return ProjectResetStaging{
		HomePath:   homePath,
		ProjectID:  projectID,
		StagingRel: stagingRel,
		Active:     true,
	}, nil
}

// CommitProjectReset removes the staged prior project tree after a successful Apply.
//
// Authorization contract (OUTSIDE_TRANSACTION_BUT_EXPLICIT_PRODUCT_OPERATION):
//   - caller must have supplied AcceptHomeReset=true on that Apply;
//   - AcceptHomeReset is validated before StageProjectReset;
//   - staging must belong to that same Apply (st.Active from StageProjectReset);
//   - TUI is currently the human-facing source of AcceptHomeReset;
//   - Atlas Core does NOT infer, authenticate, or interpret human provenance.
//
// Not for transactional rollback of live trees.
func CommitProjectReset(st ProjectResetStaging) error {
	if !st.Active || st.StagingRel == "" {
		return nil
	}
	if _, err := fsafety.ContainedJoin(st.HomePath, st.StagingRel); err != nil {
		return fmt.Errorf("atlas home: commit reset: %w", err)
	}
	stagingAbs := filepath.Join(st.HomePath, filepath.FromSlash(st.StagingRel))
	if err := os.RemoveAll(stagingAbs); err != nil {
		return fmt.Errorf("atlas home: commit reset: %w", err)
	}
	// Best-effort remove empty staging parent.
	_ = os.Remove(filepath.Dir(stagingAbs))
	return nil
}

// RestoreProjectReset puts the staged prior project tree back after footprint
// teardown of the partially created live projects/<id> tree.
// Destructive cleanup of transaction-created directories uses RestoreCreatedDirs only.
// Does not independently Remove the live project root after identity conflict.
func RestoreProjectReset(st ProjectResetStaging, liveFootprint fsafety.TransactionFootprint) error {
	if !st.Active || st.StagingRel == "" {
		return nil
	}
	prefix := filepath.ToSlash(filepath.Join(DirProjects, st.ProjectID))
	var errs []string
	for _, w := range liveFootprint.SortedFiles() {
		if w.Rel != prefix && !strings.HasPrefix(w.Rel, prefix+"/") {
			continue
		}
		snap := fsafety.FileSnapshot{Rel: w.Rel, Exists: false}
		ww := w
		if err := fsafety.RestoreFileSnapshot(st.HomePath, snap, &ww, nil); err != nil {
			errs = append(errs, err.Error())
		}
	}
	dirFP := fsafety.TransactionFootprint{}
	for _, d := range liveFootprint.Dirs {
		if d.Rel == prefix || strings.HasPrefix(d.Rel, prefix+"/") {
			dirFP.AddDir(d)
		}
	}
	if err := fsafety.RestoreCreatedDirs(st.HomePath, dirFP); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("atlas home: restore reset: %s", strings.Join(errs, "; "))
	}
	target := ProjectRoot(st.HomePath, st.ProjectID)
	info, err := os.Lstat(target)
	if err == nil {
		// Diagnostic only — do not Remove; footprint is sole destructive authority.
		kind := "entry"
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		} else if info.IsDir() {
			kind = "directory"
		} else if info.Mode().IsRegular() {
			kind = "file"
		}
		return fmt.Errorf("atlas home: restore reset: rollback conflict: %s: live tree still present after footprint teardown (observed %s); staging not renamed; no destructive action taken", prefix, kind)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("atlas home: restore reset: lstat live tree: %w", err)
	}
	stagingAbs := filepath.Join(st.HomePath, filepath.FromSlash(st.StagingRel))
	if err := os.Rename(stagingAbs, target); err != nil {
		return fmt.Errorf("atlas home: restore reset rename: %w", err)
	}
	_ = os.Remove(filepath.Dir(stagingAbs))
	return nil
}

// ResetProject deletes only $ATLAS_HOME/projects/<project-id>/.
//
// Authorization contract (OUTSIDE_TRANSACTION_BUT_EXPLICIT_PRODUCT_OPERATION):
//   - no production product callers (cmd/internal Apply/Repair/Context/MCP);
//   - tests and smoke harness only;
//   - MUST NOT be reused by automatic Init/Configure/Repair/Context/MCP rollback.
//
// Prefer StageProjectReset for Init Apply transactions.
func ResetProject(homePath, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("atlas home: reset requires home path and project id")
	}
	if strings.Contains(projectID, "..") || strings.ContainsAny(projectID, `/\`) {
		return fmt.Errorf("atlas home: refused unsafe project id %q", projectID)
	}
	if _, err := fsafety.ContainedJoin(homePath, filepath.ToSlash(filepath.Join(DirProjects, projectID))); err != nil {
		return fmt.Errorf("atlas home: reset: %w", err)
	}
	target := ProjectRoot(homePath, projectID)
	homeAbs, err := filepath.Abs(homePath)
	if err != nil {
		return fmt.Errorf("atlas home: resolve home: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("atlas home: resolve project: %w", err)
	}
	projectsAbs := filepath.Join(homeAbs, DirProjects)
	sep := string(os.PathSeparator)
	if targetAbs != filepath.Join(projectsAbs, projectID) &&
		!strings.HasPrefix(targetAbs, projectsAbs+sep) {
		return fmt.Errorf("atlas home: refused reset outside projects root")
	}
	if filepath.Base(targetAbs) != projectID {
		return fmt.Errorf("atlas home: refused reset path mismatch")
	}
	if _, err := os.Stat(targetAbs); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(targetAbs); err != nil {
		return fmt.Errorf("atlas home: reset project %s: %w", projectID, err)
	}
	return nil
}

// RelHomePath returns a display path relative to Atlas Home when possible.
func RelHomePath(homePath, full string) string {
	rel, err := filepath.Rel(homePath, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(rel)
}

// containedDirExists reports a real contained directory under Home.
// Missing => false,nil. Symlink/non-dir/unsafe => false,error.
func containedDirExists(homePath, rel string) (bool, error) {
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("not a directory: %s", rel)
	}
	return true, nil
}

// containedDirNonEmpty reports a real contained non-empty directory under Home.
func containedDirNonEmpty(homePath, rel string) (bool, error) {
	ok, err := containedDirExists(homePath, rel)
	if err != nil || !ok {
		return false, err
	}
	full, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return false, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

// containedRegularPresent reports a real contained regular non-symlink file.
func containedRegularPresent(homePath, rel string) (bool, error) {
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("not a regular file: %s", rel)
	}
	return true, nil
}

func writeHomeYAML(homePath, rel string, doc any, perm os.FileMode) error {
	_, _, err := writeHomeYAMLTracked(homePath, rel, doc, perm)
	return err
}

func writeHomeYAMLTracked(homePath, rel string, doc any, perm os.FileMode) (fsafety.WrittenFile, []fsafety.CreatedDir, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return fsafety.WrittenFile{}, nil, fmt.Errorf("atlas home: marshal yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fsafety.WrittenFile{}, nil, fmt.Errorf("atlas home: marshal yaml: %w", err)
	}
	if perm == 0 {
		perm = 0o600
	}
	w, dirs, err := fsafety.AtomicWriteContainedTracked(homePath, rel, []byte(buf.String()), perm, DirPermHome, ".atlas-home-*.tmp")
	if err != nil {
		return fsafety.WrittenFile{}, dirs, fmt.Errorf("atlas home: write %s: %w", rel, err)
	}
	return w, dirs, nil
}
