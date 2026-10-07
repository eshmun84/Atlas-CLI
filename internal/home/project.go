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

// ProjectBackupDir returns $ATLAS_HOME/projects/<project-id>/backups/<stamp>.
func ProjectBackupDir(homePath, projectID, stamp string) string {
	return filepath.Join(ProjectBackupsDir(homePath, projectID), stamp)
}

// ProjectIdentityPath returns the local identity metadata path.
func ProjectIdentityPath(homePath, projectID string) string {
	return filepath.Join(ProjectRoot(homePath, projectID), filepath.FromSlash(FileProjectIdentity))
}

// ProjectLocalStatePath returns the machine-local project state path.
func ProjectLocalStatePath(homePath, projectID string) string {
	return filepath.Join(ProjectRoot(homePath, projectID), filepath.FromSlash(FileProjectLocal))
}

// ProjectDataPresent reports whether Home already holds data for this project ID.
// Read-only: never creates directories. Detection is by project ID, never by name alone.
func ProjectDataPresent(homePath, projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return false
	}
	root := ProjectRoot(homePath, projectID)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// InspectProject returns a read-only snapshot of one Home project directory.
func InspectProject(homePath, projectID string) ProjectStatus {
	status := ProjectStatus{
		HomePath:  homePath,
		ProjectID: projectID,
		Root:      ProjectRoot(homePath, projectID),
	}
	if !ProjectDataPresent(homePath, projectID) {
		return status
	}
	status.Present = true
	status.ContextPresent = dirNonEmpty(ProjectContextDir(homePath, projectID))
	status.BackupsPresent = dirExists(ProjectBackupsDir(homePath, projectID))
	status.LocalStatePresent = Exists(ProjectLocalStatePath(homePath, projectID))
	status.IdentityPresent = Exists(ProjectIdentityPath(homePath, projectID))
	if id, ok, err := LoadProjectIdentity(homePath, projectID); err == nil && ok {
		status.Identity = id
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
}

// EnsureProjectLayout creates the project-scoped Home directories. Mutating.
func EnsureProjectLayout(homePath, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("atlas home: project layout requires home path and project id")
	}
	if err := os.MkdirAll(ProjectRoot(homePath, projectID), 0o755); err != nil {
		return fmt.Errorf("atlas home: create project root: %w", err)
	}
	for _, dir := range ProjectLayoutDirectories {
		path := filepath.Join(ProjectRoot(homePath, projectID), dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("atlas home: create project %s: %w", dir, err)
		}
	}
	return nil
}

// WriteProjectIdentity persists machine-local identity metadata under Atlas Home.
func WriteProjectIdentity(homePath, projectID, projectName, root string, now time.Time) error {
	abs, err := CanonicalRoot(root)
	if err != nil {
		return err
	}
	fp, err := RootFingerprint(abs)
	if err != nil {
		return err
	}
	existing, present, _ := LoadProjectIdentity(homePath, projectID)
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
	path := ProjectIdentityPath(homePath, projectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("atlas home: create identity dir: %w", err)
	}
	return writeYAML(path, doc)
}

// LoadProjectIdentity reads local identity metadata when present.
func LoadProjectIdentity(homePath, projectID string) (ProjectIdentity, bool, error) {
	path := ProjectIdentityPath(homePath, projectID)
	data, err := os.ReadFile(path)
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

// LoadProjectLocalState reads machine-local project state when present.
func LoadProjectLocalState(homePath, projectID string) (ProjectLocalState, bool, error) {
	path := ProjectLocalStatePath(homePath, projectID)
	data, err := os.ReadFile(path)
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
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = projectLocalSchema
	}
	path := ProjectLocalStatePath(homePath, projectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("atlas home: create local state dir: %w", err)
	}
	return writeYAML(path, doc)
}

// ResetProject deletes only $ATLAS_HOME/projects/<project-id>/.
// It never deletes by project name alone and never touches other project IDs,
// Atlas Home global assets, the product repository, or Git metadata.
func ResetProject(homePath, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("atlas home: reset requires home path and project id")
	}
	if strings.Contains(projectID, "..") || strings.ContainsAny(projectID, `/\`) {
		return fmt.Errorf("atlas home: refused unsafe project id %q", projectID)
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

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func dirNonEmpty(path string) bool {
	if !dirExists(path) {
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func writeYAML(path string, doc any) error {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return fmt.Errorf("atlas home: marshal yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("atlas home: marshal yaml: %w", err)
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		return fmt.Errorf("atlas home: write %s: %w", path, err)
	}
	return nil
}
