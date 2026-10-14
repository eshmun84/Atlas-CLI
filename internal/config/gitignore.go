package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

const (
	FileGitignore = ".gitignore"

	gitignoreBlockBegin = "# >>> Atlas local governance >>>"
	gitignoreBlockEnd   = "# <<< Atlas local governance <<<"
)

// AtlasLocalGovernanceGitignoreLines is the canonical managed ignore block body
// (without begin/end markers).
var AtlasLocalGovernanceGitignoreLines = []string{
	".atlas/",
	".cursor/",
	"AGENTS.md",
	"openspec/",
	"docs/atlas/",
}

// AtlasLocalGovernanceGitignoreBlock returns the full managed block including markers
// and a trailing newline.
func AtlasLocalGovernanceGitignoreBlock() string {
	var b strings.Builder
	b.WriteString(gitignoreBlockBegin)
	b.WriteByte('\n')
	for _, line := range AtlasLocalGovernanceGitignoreLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(gitignoreBlockEnd)
	b.WriteByte('\n')
	return b.String()
}

// GitignoreReconcileResult describes a managed .gitignore update.
type GitignoreReconcileResult struct {
	Changed   bool
	Created   bool
	BackupDir string
	Content   string
}

// NeedsLocalOnlyGitignore reports whether Apply should reconcile .gitignore.
func NeedsLocalOnlyGitignore(doc ProjectDocument) bool {
	return strings.TrimSpace(doc.SourceControl.GovernanceFiles) == GovernanceFilesLocalOnly
}

// PlanLocalOnlyGitignore returns the desired .gitignore bytes when local_only is set.
// Read-only: does not write. When current already matches, Changed is false.
func PlanLocalOnlyGitignore(root string, doc ProjectDocument) (GitignoreReconcileResult, error) {
	if !NeedsLocalOnlyGitignore(doc) {
		return GitignoreReconcileResult{}, nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return GitignoreReconcileResult{}, fmt.Errorf("gitignore: workspace root is required")
	}

	full, err := fsafety.ContainedJoin(root, FileGitignore)
	if err != nil {
		return GitignoreReconcileResult{}, err
	}
	existing, readErr := os.ReadFile(full)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return GitignoreReconcileResult{}, fmt.Errorf("gitignore: read: %w", readErr)
		}
		desired := AtlasLocalGovernanceGitignoreBlock()
		return GitignoreReconcileResult{Changed: true, Created: true, Content: desired}, nil
	}

	desired, err := mergeAtlasGitignoreBlock(string(existing))
	if err != nil {
		return GitignoreReconcileResult{}, err
	}
	if desired == string(existing) {
		return GitignoreReconcileResult{Changed: false, Content: desired}, nil
	}
	return GitignoreReconcileResult{Changed: true, Created: false, Content: desired}, nil
}

// ReconcileLocalOnlyGitignore upserts the Atlas managed block under local_only.
// Backs up an existing file only when a mutation will occur.
// workspaceFP tracks .gitignore writes; homeFP tracks Home-scoped backups.
func ReconcileLocalOnlyGitignore(root, homePath, projectID string, doc ProjectDocument, now time.Time) (GitignoreReconcileResult, fsafety.TransactionFootprint, fsafety.TransactionFootprint, error) {
	var workspaceFP, homeFP fsafety.TransactionFootprint
	plan, err := PlanLocalOnlyGitignore(root, doc)
	if err != nil {
		return GitignoreReconcileResult{}, workspaceFP, homeFP, err
	}
	if !plan.Changed {
		return plan, workspaceFP, homeFP, nil
	}

	if !plan.Created {
		backupDir, backupFP, backupErr := backupSingleProjectFile(root, homePath, projectID, FileGitignore, "init apply local_only gitignore", now)
		homeFP.MergeFootprint(backupFP)
		if backupErr != nil {
			return GitignoreReconcileResult{}, workspaceFP, homeFP, backupErr
		}
		plan.BackupDir = backupDir
	}

	wrote, dirs, writeErr := fsafety.AtomicWriteContainedTracked(root, FileGitignore, []byte(plan.Content), 0o644, 0o755, ".atlas-gitignore-*.tmp")
	workspaceFP.MergeDirs(dirs)
	if writeErr != nil {
		return GitignoreReconcileResult{}, workspaceFP, homeFP, fmt.Errorf("gitignore: write: %w", writeErr)
	}
	workspaceFP.AddFile(wrote)
	return plan, workspaceFP, homeFP, nil
}

// mergeAtlasGitignoreBlock consolidates zero-or-more Atlas managed blocks into
// exactly one canonical block while preserving project-owned content outside
// those blocks. Malformed markers fail closed without guessing.
func mergeAtlasGitignoreBlock(existing string) (string, error) {
	block := AtlasLocalGovernanceGitignoreBlock()
	spans, err := findAtlasGitignoreSpans(existing)
	if err != nil {
		return "", err
	}
	if len(spans) == 0 {
		out := existing
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		if out != "" && !strings.HasSuffix(out, "\n\n") {
			out += "\n"
		}
		return out + block, nil
	}

	var b strings.Builder
	cursor := 0
	inserted := false
	for _, span := range spans {
		prefix := existing[cursor:span.begin]
		if !inserted {
			if prefix != "" && !strings.HasSuffix(prefix, "\n") {
				prefix += "\n"
			}
			b.WriteString(prefix)
			b.WriteString(block)
			inserted = true
		} else {
			// Drop Atlas blocks; keep only project-owned content between them.
			b.WriteString(prefix)
		}
		cursor = span.end
	}
	b.WriteString(existing[cursor:])
	return b.String(), nil
}

type gitignoreSpan struct {
	begin int // inclusive index of begin marker
	end   int // exclusive index after end marker (+ optional trailing newline)
}

func findAtlasGitignoreSpans(existing string) ([]gitignoreSpan, error) {
	var spans []gitignoreSpan
	searchFrom := 0
	for {
		beginRel := strings.Index(existing[searchFrom:], gitignoreBlockBegin)
		endRel := strings.Index(existing[searchFrom:], gitignoreBlockEnd)
		switch {
		case beginRel < 0 && endRel < 0:
			return spans, nil
		case beginRel < 0 && endRel >= 0:
			return nil, fmt.Errorf("gitignore: malformed Atlas local governance markers: end without begin")
		case beginRel >= 0 && endRel < 0:
			return nil, fmt.Errorf("gitignore: malformed Atlas local governance markers: begin without end")
		case endRel < beginRel:
			return nil, fmt.Errorf("gitignore: malformed Atlas local governance markers: end before begin")
		}

		begin := searchFrom + beginRel
		endMarker := searchFrom + endRel
		// Nested / overlapping: another begin between this begin and its end.
		if nextBeginRel := strings.Index(existing[begin+len(gitignoreBlockBegin):endMarker], gitignoreBlockBegin); nextBeginRel >= 0 {
			return nil, fmt.Errorf("gitignore: malformed Atlas local governance markers: overlapping or nested blocks")
		}
		endIdx := endMarker + len(gitignoreBlockEnd)
		if endIdx < len(existing) && existing[endIdx] == '\n' {
			endIdx++
		}
		spans = append(spans, gitignoreSpan{begin: begin, end: endIdx})
		searchFrom = endIdx
	}
}

func backupSingleProjectFile(root, homePath, projectID, rel, reason string, now time.Time) (backupDir string, fp fsafety.TransactionFootprint, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return "", fp, fmt.Errorf("backup: workspace root is required")
	}
	if strings.TrimSpace(homePath) == "" || strings.TrimSpace(projectID) == "" {
		return "", fp, fmt.Errorf("backup: Atlas Home project identity is required")
	}
	full, joinErr := fsafety.ContainedJoin(root, rel)
	if joinErr != nil {
		return "", fp, joinErr
	}
	info, statErr := os.Lstat(full)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", fp, nil
		}
		return "", fp, fmt.Errorf("backup: lstat %s: %w", rel, statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fp, fmt.Errorf("backup: %s is not a regular file", rel)
	}

	stamp := mcp.NewBackupStamp(now)
	backupRel := filepath.ToSlash(filepath.Join("projects", projectID, home.ProjectDirBackups, stamp))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, backupRel, home.DirPermHome)
	fp.MergeDirs(dirs)
	if err != nil {
		return "", fp, fmt.Errorf("backup: create dir: %w", err)
	}
	backupDir = filepath.Join(homePath, filepath.FromSlash(backupRel))

	data, err := os.ReadFile(full)
	if err != nil {
		return "", fp, fmt.Errorf("backup: read %s: %w", rel, err)
	}
	destRel := filepath.ToSlash(filepath.Join(backupRel, filepath.Base(rel)))
	wrote, wdirs, werr := fsafety.AtomicWriteContainedTracked(homePath, destRel, data, 0o644, home.DirPermHome, ".atlas-backup-*.tmp")
	fp.MergeDirs(wdirs)
	if werr != nil {
		return "", fp, fmt.Errorf("backup: write: %w", werr)
	}
	fp.AddFile(wrote)

	manifest := BackupManifest{
		SchemaVersion: BackupManifestSchemaVersion,
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Reason:        reason,
		Entries: []BackupManifestEntry{{
			OriginalPath: rel,
			BackupPath:   filepath.ToSlash(filepath.Join(stamp, filepath.Base(rel))),
			Kind:         "file",
			Action:       "backup",
			Reason:       reason,
			Timestamp:    now.UTC().Format(time.RFC3339),
			Result:       "ok",
			SHA256:       sha256HexBytes(data),
		}},
	}
	if err := WriteBackupManifest(homePath, backupDir, manifest); err != nil {
		return "", fp, err
	}
	return backupDir, fp, nil
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
