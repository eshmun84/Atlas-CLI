package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

const BackupManifestSchemaVersion = 1

// BackupManifest records files Atlas backed up before replacement.
type BackupManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	CreatedAt     string                `json:"created_at"`
	Reason        string                `json:"reason"`
	Entries       []BackupManifestEntry `json:"entries"`
}

// BackupManifestEntry is one backed-up path.
type BackupManifestEntry struct {
	OriginalPath string `json:"original_path"`
	BackupPath   string `json:"backup_path"`
	Kind         string `json:"kind"`
	Action       string `json:"action"`
	Reason       string `json:"reason,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	Result       string `json:"result,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
}

// ConflictBackup is one path copied into quarantine before repair writes.
type ConflictBackup struct {
	Rel    string
	Action string
	Reason string
}

// BackupExistingTargets copies existing runtime targets into
// $ATLAS_HOME/projects/<project-id>/backups/<timestamp>/.
// When no targets exist, it returns an empty backup dir and does not create a timestamp folder.
// If backup fails, no target files should be written by the caller.
// Footprint records files/dirs created under Home for identity-aware rollback.
func BackupExistingTargets(root, homePath, projectID string, targets []string, now time.Time) (backupDir string, manifest BackupManifest, fp fsafety.TransactionFootprint, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "", BackupManifest{}, fp, fmt.Errorf("backup: workspace root is required")
	}
	if strings.TrimSpace(homePath) == "" || strings.TrimSpace(projectID) == "" {
		return "", BackupManifest{}, fp, fmt.Errorf("backup: Atlas Home project identity is required")
	}

	manifest = BackupManifest{
		SchemaVersion: BackupManifestSchemaVersion,
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Reason:        "init apply runtime materialization",
		Entries:       []BackupManifestEntry{},
	}

	type pending struct {
		rel  string
		full string
	}
	var toBackup []pending
	for _, rel := range targets {
		rel = filepath.ToSlash(filepath.Clean(rel))
		full, joinErr := safeJoinRuntime(root, rel)
		if joinErr != nil {
			return "", BackupManifest{}, fp, joinErr
		}
		info, statErr := os.Lstat(full)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return "", BackupManifest{}, fp, fmt.Errorf("backup: stat %s: %w", rel, statErr)
		}
		if info.IsDir() {
			return "", BackupManifest{}, fp, fmt.Errorf("backup: %s exists as a directory; expected a file", rel)
		}
		toBackup = append(toBackup, pending{rel: rel, full: full})
	}
	if len(toBackup) == 0 {
		return "", manifest, fp, nil
	}

	layoutDirs, err := home.EnsureProjectLayoutCreated(homePath, projectID)
	fp.MergeDirs(layoutDirs)
	if err != nil {
		return "", BackupManifest{}, fp, err
	}
	stamp := mcp.NewBackupStamp(now)
	backupRel := filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirBackups, stamp))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, backupRel, home.DirPermHome)
	fp.MergeDirs(dirs)
	if err != nil {
		return "", BackupManifest{}, fp, fmt.Errorf("backup: create %s: %w", backupRel, err)
	}

	for _, item := range toBackup {
		destRel := filepath.ToSlash(filepath.Join(backupRel, item.rel))
		sum, werr := copyFileContainedTracked(homePath, item.full, destRel, &fp)
		if werr != nil {
			return "", BackupManifest{}, fp, fmt.Errorf("backup: copy %s: %w", item.rel, werr)
		}
		manifest.Entries = append(manifest.Entries, BackupManifestEntry{
			OriginalPath: item.rel,
			BackupPath:   destRel,
			Kind:         "file",
			Action:       "replaced",
			Reason:       "existing runtime target",
			Timestamp:    manifest.CreatedAt,
			Result:       "ok",
			SHA256:       sum,
		})
	}

	if err := writeBackupManifestContainedTracked(homePath, backupRel, manifest, &fp); err != nil {
		return "", BackupManifest{}, fp, err
	}
	return backupRel, manifest, fp, nil
}

// BackupConflicts copies files or directories into
// $ATLAS_HOME/projects/<project-id>/backups/<timestamp>/.
// Directories are copied recursively. Missing paths are skipped.
// Transitional project-local .atlas/backups/ may still exist for older installs.
func BackupConflicts(root, homePath, projectID string, items []ConflictBackup, now time.Time) (backupDir string, manifest BackupManifest, fp fsafety.TransactionFootprint, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "", BackupManifest{}, fp, fmt.Errorf("backup: workspace root is required")
	}
	if strings.TrimSpace(homePath) == "" || strings.TrimSpace(projectID) == "" {
		return "", BackupManifest{}, fp, fmt.Errorf("backup: Atlas Home project identity is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	manifest = BackupManifest{
		SchemaVersion: BackupManifestSchemaVersion,
		CreatedAt:     now.Format(time.RFC3339),
		Reason:        "runtime repair",
		Entries:       []BackupManifestEntry{},
	}

	type pending struct {
		item ConflictBackup
		rel  string
		full string
		dir  bool
	}
	var toBackup []pending
	seen := map[string]struct{}{}
	for _, item := range items {
		rel := filepath.ToSlash(filepath.Clean(item.Rel))
		if _, ok := seen[rel]; ok {
			continue
		}
		if err := assertAllowedConflictPath(rel); err != nil {
			return "", BackupManifest{}, fp, err
		}
		full, joinErr := safeJoinRoot(root, rel)
		if joinErr != nil {
			return "", BackupManifest{}, fp, joinErr
		}
		info, statErr := os.Lstat(full)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return "", BackupManifest{}, fp, fmt.Errorf("backup: stat %s: %w", rel, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", BackupManifest{}, fp, fmt.Errorf("backup: refused symlink %s", rel)
		}
		seen[rel] = struct{}{}
		toBackup = append(toBackup, pending{item: item, rel: rel, full: full, dir: info.IsDir()})
	}
	if len(toBackup) == 0 {
		return "", manifest, fp, nil
	}

	layoutDirs, err := home.EnsureProjectLayoutCreated(homePath, projectID)
	fp.MergeDirs(layoutDirs)
	if err != nil {
		return "", BackupManifest{}, fp, err
	}
	stamp := mcp.NewBackupStamp(now)
	backupRel := filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirBackups, stamp))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, backupRel, home.DirPermHome)
	fp.MergeDirs(dirs)
	if err != nil {
		return "", BackupManifest{}, fp, fmt.Errorf("backup: create %s: %w", backupRel, err)
	}

	for _, item := range toBackup {
		destRel := filepath.ToSlash(filepath.Join(backupRel, item.rel))
		action := item.item.Action
		if action == "" {
			action = "replaced"
		}
		reason := item.item.Reason
		if reason == "" {
			reason = "runtime conflict"
		}
		kind := "file"
		var sum string
		if item.dir {
			kind = "directory"
			dirs, mkErr := fsafety.SafeMkdirAllCreated(homePath, destRel, home.DirPermHome)
			fp.MergeDirs(dirs)
			if mkErr != nil {
				return "", BackupManifest{}, fp, fmt.Errorf("backup: create %s: %w", destRel, mkErr)
			}
			sum, err = copyDirContainedTracked(homePath, item.full, destRel, &fp)
			if err != nil {
				return "", BackupManifest{}, fp, fmt.Errorf("backup: copy dir %s: %w", item.rel, err)
			}
		} else {
			sum, err = copyFileContainedTracked(homePath, item.full, destRel, &fp)
			if err != nil {
				return "", BackupManifest{}, fp, fmt.Errorf("backup: copy %s: %w", item.rel, err)
			}
		}
		manifest.Entries = append(manifest.Entries, BackupManifestEntry{
			OriginalPath: item.rel,
			BackupPath:   destRel,
			Kind:         kind,
			Action:       action,
			Reason:       reason,
			Timestamp:    manifest.CreatedAt,
			Result:       "copied",
			SHA256:       sum,
		})
	}

	if err := writeBackupManifestContainedTracked(homePath, backupRel, manifest, &fp); err != nil {
		return "", BackupManifest{}, fp, err
	}
	return backupRel, manifest, fp, nil
}

// WriteBackupManifest writes manifest.json into an absolute Home backup directory.
// backupDir may be Home-relative (projects/<id>/backups/<stamp>) or absolute.
func WriteBackupManifest(homePath, backupDir string, manifest BackupManifest) error {
	_, err := WriteBackupManifestTracked(homePath, backupDir, manifest)
	return err
}

// WriteBackupManifestTracked is WriteBackupManifest plus write footprints.
func WriteBackupManifestTracked(homePath, backupDir string, manifest BackupManifest) (fsafety.TransactionFootprint, error) {
	var fp fsafety.TransactionFootprint
	backupRel := backupDir
	if filepath.IsAbs(backupDir) {
		rel, err := filepath.Rel(homePath, backupDir)
		if err != nil {
			return fp, fmt.Errorf("backup: manifest path: %w", err)
		}
		backupRel = filepath.ToSlash(rel)
	}
	if err := writeBackupManifestContainedTracked(homePath, backupRel, manifest, &fp); err != nil {
		return fp, err
	}
	return fp, nil
}

func writeBackupManifestContainedTracked(homePath, backupRel string, manifest BackupManifest, fp *fsafety.TransactionFootprint) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: marshal manifest: %w", err)
	}
	data = append(data, '\n')
	rel := filepath.ToSlash(filepath.Join(backupRel, "manifest.json"))
	w, dirs, err := fsafety.AtomicWriteContainedTracked(homePath, rel, data, 0o600, home.DirPermHome, ".atlas-backup-*.tmp")
	fp.MergeDirs(dirs)
	if err != nil {
		return fmt.Errorf("backup: write manifest: %w", err)
	}
	fp.AddFile(w)
	return nil
}

func copyDirContainedTracked(homePath, src, destRel string, fp *fsafety.TransactionFootprint) (string, error) {
	h := sha256.New()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refused symlink %s", rel)
		}
		targetRel := filepath.ToSlash(filepath.Join(destRel, rel))
		if info.IsDir() {
			dirs, mkErr := fsafety.SafeMkdirAllCreated(homePath, targetRel, home.DirPermHome)
			fp.MergeDirs(dirs)
			if mkErr != nil {
				return mkErr
			}
			return nil
		}
		sum, err := copyFileContainedTracked(homePath, path, targetRel, fp)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s:%s\n", filepath.ToSlash(rel), sum)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFileContainedTracked(homePath, src, destRel string, fp *fsafety.TransactionFootprint) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	data, err := io.ReadAll(in)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	w, dirs, err := fsafety.AtomicWriteContainedTracked(homePath, destRel, data, 0o600, home.DirPermHome, ".atlas-backup-*.tmp")
	fp.MergeDirs(dirs)
	if err != nil {
		return "", err
	}
	fp.AddFile(w)
	return hex.EncodeToString(sum[:]), nil
}
