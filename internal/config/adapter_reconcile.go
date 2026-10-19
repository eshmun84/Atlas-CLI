package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// AtlasOwnedAdapterRuntimePaths returns Atlas-owned runtime entry + agent pack
// paths under one adapter surface. Full adapter reprojection also reconciles
// MCP, skills, and hooks via their projectors (orchestrated by PersistConfigure
// from Atlas canonical state — never from another adapter's files).
func AtlasOwnedAdapterRuntimePaths(adapter string) []string {
	var out []string
	switch adapter {
	case "cursor":
		out = append(out, FileCursorAtlasMDC)
	case "opencode":
		out = append(out, FileOpenCodeAtlas, FileOpenCodeMCP)
	default:
		return nil
	}
	out = append(out, AtlasAgentRuntimePaths([]string{adapter})...)
	sort.Strings(out)
	return out
}

// AdapterRuntimePathsToRemove returns Atlas-owned runtime paths present for
// adapters in previous but not in next. Unknown/user-owned files under the
// adapter tree are never listed.
func AdapterRuntimePathsToRemove(previous, next []string) []string {
	prev := map[string]bool{}
	for _, a := range normalizeSelectedAdapters(previous) {
		prev[a] = true
	}
	nextSet := map[string]bool{}
	for _, a := range normalizeSelectedAdapters(next) {
		nextSet[a] = true
	}
	var out []string
	seen := map[string]struct{}{}
	for adapter := range prev {
		if nextSet[adapter] {
			continue
		}
		for _, path := range AtlasOwnedAdapterRuntimePaths(adapter) {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// ConfigureRuntimeSnapshotPaths returns paths that must be snapshotted when
// Configure reconciles adapter runtime surfaces.
func ConfigureRuntimeSnapshotPaths(previous, next ProjectDocument) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(rel string) {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "" || rel == "." {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	add(FileAgentsMD)
	add(FileAgentRegistry)
	add(FileRuntimeManifest)
	add(FileAssetsLock)
	add(FileGitignore)
	// Legacy OpenCode root host file may be stripped of Atlas MCP keys on reconcile.
	add("opencode.json")
	for _, rel := range RuntimeTargets(next) {
		add(rel)
	}
	for _, rel := range AdapterRuntimePathsToRemove(previous.Adapters.Selected, next.Adapters.Selected) {
		add(rel)
	}
	for _, rel := range RuntimeTargets(previous) {
		add(rel)
	}
	for _, rel := range AtlasOwnedAdapterRuntimePaths("opencode") {
		add(rel)
	}
	sort.Strings(out)
	return out
}

// removeTrackedAtlasOwnedFile deletes an Atlas-owned regular file after
// verifying it is a safe regular file. Records deletion on the footprint.
// Missing paths are no-ops. Symlinks/dirs fail closed.
func removeTrackedAtlasOwnedFile(fp *fsafety.TransactionFootprint, root, rel string) error {
	if err := assertAllowedRuntimePath(rel); err != nil {
		// Agent registry / manifest are atlas paths — allow those too for callers.
		if err2 := assertAllowedAtlasPath(rel); err2 != nil {
			return err
		}
	}
	full, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove %s: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("remove %s: refusing symlink", rel)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("remove %s: not a regular file", rel)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("remove %s: read: %w", rel, err)
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("remove %s: %w", rel, err)
	}
	if fp != nil {
		fp.AddDeleted(fsafety.DeletedFile{
			Rel:      filepath.ToSlash(filepath.Clean(rel)),
			Expected: data,
			Mode:     info.Mode(),
			Info:     info,
		})
	}
	return nil
}

// materializeConfigureRuntime reprojects AGENTS.md + selected adapter entry/
// agent-pack surfaces from Atlas canonical state and removes Atlas-owned
// projections for deselected adapters. Regenerates assets.lock.yaml so Doctor
// checksums match the final adapters/runtime set. Preserves unknown/user-owned
// files under adapter trees. MCP/skills/hooks/provider cleanup are
// co-reconciled by PersistConfigure on the same adapter change.
func materializeConfigureRuntime(fp *fsafety.TransactionFootprint, root string, previous, doc ProjectDocument, homePath string) error {
	targets := RuntimeTargets(doc)
	for _, rel := range targets {
		full, joinErr := safeJoinRuntime(root, rel)
		if joinErr != nil {
			return joinErr
		}
		var existing []byte
		if data, readErr := os.ReadFile(full); readErr == nil {
			existing = data
		} else if !os.IsNotExist(readErr) {
			return fmt.Errorf("read %s: %w", rel, readErr)
		}
		content, renderErr := renderRuntimeFile(rel, doc, existing)
		if renderErr != nil {
			return renderErr
		}
		if err := recordTrackedWrite(fp, root, rel, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}

	registry := RenderAgentRegistry(doc.Project.Name, doc.Adapters.Selected, homePath)
	if err := recordTrackedWrite(fp, root, FileAgentRegistry, []byte(registry), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", FileAgentRegistry, err)
	}
	manifest, err := RenderRuntimeManifestYAML(doc.Project.Name, doc.Adapters.Selected)
	if err != nil {
		return fmt.Errorf("render %s: %w", FileRuntimeManifest, err)
	}
	if err := recordTrackedWrite(fp, root, FileRuntimeManifest, []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", FileRuntimeManifest, err)
	}

	lockDoc, lockErr := BuildAssetsLockDocumentFor(homePath, doc, version.Version)
	if lockErr != nil {
		return fmt.Errorf("build %s: %w", FileAssetsLock, lockErr)
	}
	lockYAML, marshalErr := marshalYAML(lockDoc)
	if marshalErr != nil {
		return fmt.Errorf("marshal %s: %w", FileAssetsLock, marshalErr)
	}
	if err := recordTrackedWrite(fp, root, FileAssetsLock, lockYAML, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", FileAssetsLock, err)
	}

	for _, rel := range AdapterRuntimePathsToRemove(previous.Adapters.Selected, doc.Adapters.Selected) {
		// Skip paths that are also in the new target set (shouldn't happen).
		skip := false
		for _, keep := range targets {
			if keep == rel {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if err := removeTrackedAtlasOwnedFile(fp, root, rel); err != nil {
			return err
		}
	}
	return nil
}

// reconcileDeselectedAdapterCleanup removes provider-owned OpenSpec tool
// projections for deselected adapters and prunes empty adapter directory trees
// when no user-owned content remains. Must run after Atlas MCP/skills/hooks/
// runtime reconcile so Atlas-owned files are already gone.
func reconcileDeselectedAdapterCleanup(fp *fsafety.TransactionFootprint, root string, previous, doc ProjectDocument) error {
	if adaptersEqualSet(previous.Adapters.Selected, doc.Adapters.Selected) {
		return nil
	}
	if err := openspec.RemoveStaleToolProjections(fp, root, doc.Adapters.Selected); err != nil {
		return fmt.Errorf("provider tool cleanup: %w", err)
	}
	if err := pruneEmptyDeselectedAdapterRoots(root, previous.Adapters.Selected, doc.Adapters.Selected); err != nil {
		return fmt.Errorf("adapter tree cleanup: %w", err)
	}
	return nil
}

// pruneEmptyDeselectedAdapterRoots removes empty directories under deselected
// adapter roots (and the root itself when fully empty). Never deletes a
// directory that still contains any entry.
func pruneEmptyDeselectedAdapterRoots(root string, previous, next []string) error {
	prev := map[string]bool{}
	for _, a := range normalizeSelectedAdapters(previous) {
		prev[a] = true
	}
	nextSet := map[string]bool{}
	for _, a := range normalizeSelectedAdapters(next) {
		nextSet[a] = true
	}
	for adapter := range prev {
		if nextSet[adapter] {
			continue
		}
		var prefix string
		switch adapter {
		case "cursor":
			prefix = ".cursor"
		case "opencode":
			prefix = ".opencode"
		default:
			continue
		}
		if err := pruneEmptyTree(root, prefix); err != nil {
			return err
		}
	}
	return nil
}

func pruneEmptyTree(root, prefix string) error {
	abs, err := fsafety.ContainedJoin(root, prefix)
	if err != nil {
		return nil
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	var dirs []string
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() {
			return walkErr
		}
		dirs = append(dirs, path)
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		_ = os.Remove(d) // empty only; fails silently when non-empty
	}
	return nil
}

// adaptersEqualSet reports whether two adapter selections are the same set.
func adaptersEqualSet(a, b []string) bool {
	return strings.TrimSpace(JoinChips(normalizeSelectedAdapters(a))) ==
		strings.TrimSpace(JoinChips(normalizeSelectedAdapters(b)))
}
