package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// AtomicWriteContained writes data to root/rel via temp+rename after symlink-safe
// containment checks.
func AtomicWriteContained(root, rel string, data []byte, perm os.FileMode) error {
	if err := fsafety.AtomicWriteContainedPrefix(root, rel, data, perm, ".atlas-mcp-*.tmp"); err != nil {
		return fmt.Errorf("mcp write: %s", trimFsafetyPrefix(strings.TrimPrefix(err.Error(), "fsafety write: ")))
	}
	return nil
}

func atomicReplace(path string, data []byte, perm os.FileMode) error {
	if err := fsafety.AtomicReplace(path, data, perm, ".atlas-mcp-*.tmp"); err != nil {
		return fmt.Errorf("mcp write: %s", trimFsafetyPrefix(strings.TrimPrefix(err.Error(), "fsafety write: ")))
	}
	return nil
}

// DirMCPBackups is the Home-relative MCP backup directory name.
const DirMCPBackups = "backups"

// WriteHomeBackup stores a native MCP config backup under Atlas Home.
// Never writes *.atlas-backup (or any backup) into the project workspace.
func WriteHomeBackup(homePath, projectID, stamp, adapterID string, data []byte) (string, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	stamp = strings.TrimSpace(stamp)
	adapterID = strings.TrimSpace(adapterID)
	if homePath == "" || projectID == "" || stamp == "" || adapterID == "" {
		return "", fmt.Errorf("mcp backup: home path, project id, stamp, and adapter are required")
	}
	if strings.Contains(adapterID, "/") || strings.Contains(adapterID, `\`) || strings.Contains(adapterID, "..") {
		return "", fmt.Errorf("mcp backup: invalid adapter id")
	}
	// Contain Home writes: refuse symlink escape under Atlas Home project tree.
	rel := filepath.ToSlash(filepath.Join("projects", projectID, DirMCP, DirMCPBackups, stamp, adapterID+".json"))
	dirRel := filepath.ToSlash(filepath.Join("projects", projectID, DirMCP, DirMCPBackups, stamp))
	if err := fsafety.SafeMkdirAll(homePath, dirRel, 0o700); err != nil {
		return "", fmt.Errorf("mcp backup: mkdir: %w", err)
	}
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, 0o700, ".atlas-mcp-*.tmp"); err != nil {
		return "", fmt.Errorf("mcp backup: %s", trimFsafetyPrefix(err.Error()))
	}
	return filepath.Join(homePath, filepath.FromSlash(rel)), nil
}

var backupSeq atomic.Uint64

// NewBackupStamp returns a unique UTC stamp suitable for backup directory names.
// Nanosecond precision plus a process-local monotonic sequence avoids collisions
// when two backups are requested within the same second (or nanosecond).
func NewBackupStamp(now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	seq := backupSeq.Add(1)
	return fmt.Sprintf("%s-%06d", now.UTC().Format("20060102T150405.000000000Z"), seq)
}

// WorkspaceHasAtlasMCPBackup reports whether any Atlas MCP backup artifact
// exists under the project workspace (should always be false after Slice 32.1).
func WorkspaceHasAtlasMCPBackup(root string) (bool, string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if strings.HasSuffix(name, ".atlas-backup") || strings.Contains(name, ".atlas-mcp-") && strings.HasSuffix(name, ".tmp") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return fmt.Errorf("rel %s: %w", path, relErr)
			}
			found = filepath.ToSlash(rel)
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return false, "", err
	}
	return found != "", found, nil
}
