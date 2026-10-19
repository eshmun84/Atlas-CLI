package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// AtlasLocalGovernanceGitignoreLinesFor returns managed ignore paths derived
// from Atlas ownership for the given document (adapters + skill pins).
//
// Ownership notes:
//   - Shared Atlas surfaces (`.atlas/`, AGENTS.md, openspec/, docs/atlas/) always.
//   - Cursor: coarse `.cursor/` — Git cannot safely separate Atlas vs user Cursor files.
//   - OpenCode: Atlas-owned path patterns only (no blanket `.opencode/`, no root
//     `opencode.json`). Managed MCP lives at `.opencode/opencode.json` (OpenCode
//     config layer that merges after root `opencode.json`), so local_only can hide
//     Atlas MCP without hiding user-owned root OpenCode settings.
func AtlasLocalGovernanceGitignoreLinesFor(doc ProjectDocument) []string {
	lines := []string{
		".atlas/",
		"AGENTS.md",
		"openspec/",
		"docs/atlas/",
	}
	hasCursor, hasOpenCode := false, false
	for _, a := range normalizeSelectedAdapters(doc.Adapters.Selected) {
		switch a {
		case "cursor":
			hasCursor = true
		case "opencode":
			hasOpenCode = true
		}
	}
	if hasCursor {
		lines = append(lines, ".cursor/")
	}
	if hasOpenCode {
		lines = append(lines,
			".opencode/atlas.md",
			".opencode/agents/atlas-*.md",
			".opencode/plugins/atlas-hooks/",
			".opencode/opencode.json",
			".opencode/commands/opsx-*",
			".opencode/skills/openspec-*/",
		)
		seenSkill := map[string]struct{}{}
		var skillLines []string
		for _, pin := range doc.SkillPins() {
			id := strings.TrimSpace(pin.ID)
			if id == "" || strings.HasPrefix(id, "openspec-") {
				continue
			}
			if _, ok := seenSkill[id]; ok {
				continue
			}
			seenSkill[id] = struct{}{}
			skillLines = append(skillLines, ".opencode/skills/"+id+"/")
		}
		sort.Strings(skillLines)
		lines = append(lines, skillLines...)
	}
	return lines
}

// AtlasLocalGovernanceGitignoreLines is the full dual-adapter ignore body used
// by callers without a project document (tests / fallback). Prefer
// AtlasLocalGovernanceGitignoreLinesFor with the live ProjectDocument.
var AtlasLocalGovernanceGitignoreLines = AtlasLocalGovernanceGitignoreLinesFor(ProjectDocument{
	Adapters: AdaptersPersist{Selected: []string{"cursor", "opencode"}},
})

// AtlasLocalGovernanceGitignoreBlockFor returns the managed block for doc.
func AtlasLocalGovernanceGitignoreBlockFor(doc ProjectDocument) string {
	var b strings.Builder
	b.WriteString(gitignoreBlockBegin)
	b.WriteByte('\n')
	for _, line := range AtlasLocalGovernanceGitignoreLinesFor(doc) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(gitignoreBlockEnd)
	b.WriteByte('\n')
	return b.String()
}

// AtlasLocalGovernanceGitignoreBlock returns the full dual-adapter managed block.
func AtlasLocalGovernanceGitignoreBlock() string {
	return AtlasLocalGovernanceGitignoreBlockFor(ProjectDocument{
		Adapters: AdaptersPersist{Selected: []string{"cursor", "opencode"}},
	})
}

// GitignoreReconcileResult describes a managed .gitignore update.
type GitignoreReconcileResult struct {
	Changed   bool
	Created   bool
	BackupDir string
	Content   string
}

// NeedsLocalOnlyGitignore reports whether local_only policy applies.
func NeedsLocalOnlyGitignore(doc ProjectDocument) bool {
	return strings.TrimSpace(doc.SourceControl.GovernanceFiles) == GovernanceFilesLocalOnly
}

// NeedsVersionedGitignoreCleanup reports whether versioned policy should remove
// the Atlas managed ignore block (governance surfaces remain visible to Git).
func NeedsVersionedGitignoreCleanup(doc ProjectDocument) bool {
	return strings.TrimSpace(doc.SourceControl.GovernanceFiles) == GovernanceFilesVersioned
}

// PlanGovernanceGitignore returns the desired .gitignore mutation for the
// governance_files policy. local_only upserts the managed block; versioned
// removes it while preserving project-owned content. Independent of Git hosting.
func PlanGovernanceGitignore(root string, doc ProjectDocument) (GitignoreReconcileResult, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return GitignoreReconcileResult{}, fmt.Errorf("gitignore: workspace root is required")
	}

	switch {
	case NeedsLocalOnlyGitignore(doc):
		return planLocalOnlyGitignore(root, doc)
	case NeedsVersionedGitignoreCleanup(doc):
		return planVersionedGitignoreCleanup(root)
	default:
		return GitignoreReconcileResult{}, nil
	}
}

// PlanLocalOnlyGitignore returns the desired .gitignore bytes when local_only is set.
// Prefer PlanGovernanceGitignore for callers that also handle versioned cleanup.
func PlanLocalOnlyGitignore(root string, doc ProjectDocument) (GitignoreReconcileResult, error) {
	if !NeedsLocalOnlyGitignore(doc) {
		return GitignoreReconcileResult{}, nil
	}
	return planLocalOnlyGitignore(root, doc)
}

func planLocalOnlyGitignore(root string, doc ProjectDocument) (GitignoreReconcileResult, error) {
	full, err := fsafety.ContainedJoin(root, FileGitignore)
	if err != nil {
		return GitignoreReconcileResult{}, err
	}
	existing, readErr := os.ReadFile(full)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return GitignoreReconcileResult{}, fmt.Errorf("gitignore: read: %w", readErr)
		}
		desired := AtlasLocalGovernanceGitignoreBlockFor(doc)
		return GitignoreReconcileResult{Changed: true, Created: true, Content: desired}, nil
	}

	desired, err := mergeAtlasGitignoreBlock(string(existing), doc)
	if err != nil {
		return GitignoreReconcileResult{}, err
	}
	if desired == string(existing) {
		return GitignoreReconcileResult{Changed: false, Content: desired}, nil
	}
	return GitignoreReconcileResult{Changed: true, Created: false, Content: desired}, nil
}

func planVersionedGitignoreCleanup(root string) (GitignoreReconcileResult, error) {
	full, err := fsafety.ContainedJoin(root, FileGitignore)
	if err != nil {
		return GitignoreReconcileResult{}, err
	}
	existing, readErr := os.ReadFile(full)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return GitignoreReconcileResult{}, nil
		}
		return GitignoreReconcileResult{}, fmt.Errorf("gitignore: read: %w", readErr)
	}
	desired, err := stripAtlasGitignoreBlocks(string(existing))
	if err != nil {
		return GitignoreReconcileResult{}, err
	}
	if desired == string(existing) {
		return GitignoreReconcileResult{Changed: false, Content: desired}, nil
	}
	return GitignoreReconcileResult{Changed: true, Created: false, Content: desired}, nil
}

// ReconcileLocalOnlyGitignore upserts the Atlas managed block under local_only.
// Prefer ReconcileGovernanceGitignore when versioned cleanup is also required.
func ReconcileLocalOnlyGitignore(root, homePath, projectID string, doc ProjectDocument, now time.Time) (GitignoreReconcileResult, fsafety.TransactionFootprint, fsafety.TransactionFootprint, error) {
	if !NeedsLocalOnlyGitignore(doc) {
		return GitignoreReconcileResult{}, fsafety.TransactionFootprint{}, fsafety.TransactionFootprint{}, nil
	}
	return reconcileGovernanceGitignore(root, homePath, projectID, doc, now, "local_only gitignore")
}

// ReconcileGovernanceGitignore applies local_only upsert or versioned block removal.
// Backs up an existing file only when a mutation will occur.
// workspaceFP tracks .gitignore writes; homeFP tracks Home-scoped backups.
func ReconcileGovernanceGitignore(root, homePath, projectID string, doc ProjectDocument, now time.Time) (GitignoreReconcileResult, fsafety.TransactionFootprint, fsafety.TransactionFootprint, error) {
	reason := "governance gitignore"
	switch {
	case NeedsLocalOnlyGitignore(doc):
		reason = "local_only gitignore"
	case NeedsVersionedGitignoreCleanup(doc):
		reason = "versioned gitignore cleanup"
	}
	return reconcileGovernanceGitignore(root, homePath, projectID, doc, now, reason)
}

func reconcileGovernanceGitignore(root, homePath, projectID string, doc ProjectDocument, now time.Time, reason string) (GitignoreReconcileResult, fsafety.TransactionFootprint, fsafety.TransactionFootprint, error) {
	var workspaceFP, homeFP fsafety.TransactionFootprint
	plan, err := PlanGovernanceGitignore(root, doc)
	if err != nil {
		return GitignoreReconcileResult{}, workspaceFP, homeFP, err
	}
	if !plan.Changed {
		return plan, workspaceFP, homeFP, nil
	}

	if !plan.Created {
		backupDir, backupFP, backupErr := backupSingleProjectFile(root, homePath, projectID, FileGitignore, reason, now)
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

// stripAtlasGitignoreBlocks removes all Atlas managed blocks, preserving
// project-owned content. Malformed markers fail closed.
func stripAtlasGitignoreBlocks(existing string) (string, error) {
	spans, err := findAtlasGitignoreSpans(existing)
	if err != nil {
		return "", err
	}
	if len(spans) == 0 {
		return existing, nil
	}
	var b strings.Builder
	cursor := 0
	for _, span := range spans {
		b.WriteString(existing[cursor:span.begin])
		cursor = span.end
	}
	b.WriteString(existing[cursor:])
	out := b.String()
	// Collapse excess blank lines left by block removal (cosmetic).
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return out, nil
}

// mergeAtlasGitignoreBlock consolidates zero-or-more Atlas managed blocks into
// exactly one canonical block while preserving project-owned content outside
// those blocks. Malformed markers fail closed without guessing.
func mergeAtlasGitignoreBlock(existing string, doc ProjectDocument) (string, error) {
	block := AtlasLocalGovernanceGitignoreBlockFor(doc)
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
