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
func BackupExistingTargets(root, homePath, projectID string, targets []string, now time.Time) (backupDir string, manifest BackupManifest, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "", BackupManifest{}, fmt.Errorf("backup: workspace root is required")
	}
	if strings.TrimSpace(homePath) == "" || strings.TrimSpace(projectID) == "" {
		return "", BackupManifest{}, fmt.Errorf("backup: Atlas Home project identity is required")
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
			return "", BackupManifest{}, joinErr
		}
		info, statErr := os.Lstat(full)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return "", BackupManifest{}, fmt.Errorf("backup: stat %s: %w", rel, statErr)
		}
		if info.IsDir() {
			return "", BackupManifest{}, fmt.Errorf("backup: %s exists as a directory; expected a file", rel)
		}
		toBackup = append(toBackup, pending{rel: rel, full: full})
	}
	if len(toBackup) == 0 {
		return "", manifest, nil
	}

	if err := home.EnsureProjectLayout(homePath, projectID); err != nil {
		return "", BackupManifest{}, err
	}
	stamp := now.UTC().Format("20060102T150405Z")
	backupAbs := home.ProjectBackupDir(homePath, projectID, stamp)
	backupRel := home.RelHomePath(homePath, backupAbs)
	if err := os.MkdirAll(backupAbs, 0o755); err != nil {
		return "", BackupManifest{}, fmt.Errorf("backup: create %s: %w", backupRel, err)
	}

	for _, item := range toBackup {
		destRel := filepath.ToSlash(filepath.Join(backupRel, item.rel))
		destAbs := filepath.Join(backupAbs, filepath.FromSlash(item.rel))
		if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
			return "", BackupManifest{}, fmt.Errorf("backup: create parent for %s: %w", destRel, err)
		}
		sum, err := copyFileWithSHA(item.full, destAbs)
		if err != nil {
			return "", BackupManifest{}, fmt.Errorf("backup: copy %s: %w", item.rel, err)
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

	if err := writeBackupManifest(backupAbs, manifest); err != nil {
		return "", BackupManifest{}, err
	}
	return backupRel, manifest, nil
}

// BackupConflicts copies files or directories into
// $ATLAS_HOME/projects/<project-id>/backups/<timestamp>/.
// Directories are copied recursively. Missing paths are skipped.
// Transitional project-local .atlas/backups/ may still exist for older installs.
func BackupConflicts(root, homePath, projectID string, items []ConflictBackup, now time.Time) (backupDir string, manifest BackupManifest, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "", BackupManifest{}, fmt.Errorf("backup: workspace root is required")
	}
	if strings.TrimSpace(homePath) == "" || strings.TrimSpace(projectID) == "" {
		return "", BackupManifest{}, fmt.Errorf("backup: Atlas Home project identity is required")
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
			return "", BackupManifest{}, err
		}
		full, joinErr := safeJoinRoot(root, rel)
		if joinErr != nil {
			return "", BackupManifest{}, joinErr
		}
		info, statErr := os.Lstat(full)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return "", BackupManifest{}, fmt.Errorf("backup: stat %s: %w", rel, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", BackupManifest{}, fmt.Errorf("backup: refused symlink %s", rel)
		}
		seen[rel] = struct{}{}
		toBackup = append(toBackup, pending{item: item, rel: rel, full: full, dir: info.IsDir()})
	}
	if len(toBackup) == 0 {
		return "", manifest, nil
	}

	if err := home.EnsureProjectLayout(homePath, projectID); err != nil {
		return "", BackupManifest{}, err
	}
	stamp := now.Format("20060102T150405Z")
	backupAbs := home.ProjectBackupDir(homePath, projectID, stamp)
	backupRel := home.RelHomePath(homePath, backupAbs)
	if err := os.MkdirAll(backupAbs, 0o755); err != nil {
		return "", BackupManifest{}, fmt.Errorf("backup: create %s: %w", backupRel, err)
	}

	for _, item := range toBackup {
		destRel := filepath.ToSlash(filepath.Join(backupRel, item.rel))
		destAbs := filepath.Join(backupAbs, filepath.FromSlash(item.rel))
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
			if err := os.MkdirAll(destAbs, 0o755); err != nil {
				return "", BackupManifest{}, fmt.Errorf("backup: create %s: %w", destRel, err)
			}
			sum, err = copyDirWithSHA(item.full, destAbs)
			if err != nil {
				return "", BackupManifest{}, fmt.Errorf("backup: copy dir %s: %w", item.rel, err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
				return "", BackupManifest{}, fmt.Errorf("backup: create parent for %s: %w", destRel, err)
			}
			sum, err = copyFileWithSHA(item.full, destAbs)
			if err != nil {
				return "", BackupManifest{}, fmt.Errorf("backup: copy %s: %w", item.rel, err)
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

	if err := writeBackupManifest(backupAbs, manifest); err != nil {
		return "", BackupManifest{}, err
	}
	return backupRel, manifest, nil
}

// WriteBackupManifest writes manifest.json into an absolute Home backup directory.
// backupDir may be Home-relative (projects/<id>/backups/<stamp>) or absolute.
func WriteBackupManifest(homePath, backupDir string, manifest BackupManifest) error {
	backupAbs := backupDir
	if !filepath.IsAbs(backupDir) {
		backupAbs = filepath.Join(homePath, filepath.FromSlash(backupDir))
	}
	return writeBackupManifest(backupAbs, manifest)
}

func writeBackupManifest(backupAbs string, manifest BackupManifest) error {
	manifestPath := filepath.Join(backupAbs, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("backup: write manifest: %w", err)
	}
	return nil
}

func copyDirWithSHA(src, dst string) (string, error) {
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
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		sum, err := copyFileWithSHA(path, target)
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

func copyFileWithSHA(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer out.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
