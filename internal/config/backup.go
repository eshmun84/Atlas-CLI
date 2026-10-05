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
	SHA256       string `json:"sha256,omitempty"`
}

// BackupExistingTargets copies existing runtime targets into .atlas/backups/<timestamp>/.
// When no targets exist, it returns an empty backup dir and does not create a timestamp folder.
// If backup fails, no target files should be written by the caller.
func BackupExistingTargets(root string, targets []string, now time.Time) (backupRelDir string, manifest BackupManifest, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "", BackupManifest{}, fmt.Errorf("backup: workspace root is required")
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

	stamp := now.UTC().Format("20060102T150405Z")
	backupRelDir = filepath.ToSlash(filepath.Join(DirBackups, stamp))
	backupAbs, err := safeJoinAtlas(root, backupRelDir)
	if err != nil {
		return "", BackupManifest{}, err
	}
	if err := os.MkdirAll(backupAbs, 0o755); err != nil {
		return "", BackupManifest{}, fmt.Errorf("backup: create %s: %w", backupRelDir, err)
	}

	for _, item := range toBackup {
		destRel := filepath.ToSlash(filepath.Join(backupRelDir, item.rel))
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
			SHA256:       sum,
		})
	}

	manifestPath := filepath.Join(backupAbs, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", BackupManifest{}, fmt.Errorf("backup: marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return "", BackupManifest{}, fmt.Errorf("backup: write manifest: %w", err)
	}
	return backupRelDir, manifest, nil
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
