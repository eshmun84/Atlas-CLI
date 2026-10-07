package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

const (
	ApplySuccessTitle        = "Atlas configuration and runtime initialized."
	ApplySuccessBody         = "Runtime files materialized."
	ConfigureApplySuccess    = "Configuration changes saved."
	ConfigureApplyFooterNote = "Close discards unsaved changes. Apply changes writes .atlas/config.yaml."
)

// Paths Apply must never create, modify, backup, replace, or delete.
var forbiddenApplyRelPaths = []string{
	".agents",
	".claude",
	"CLAUDE.md",
	"GEMINI.md",
	"README.md",
	".gitignore",
	FileMemoryDB,
	FileCapsule,
}

// ApplyInput is the in-memory init state used to persist Atlas config and runtime files.
type ApplyInput struct {
	Root  string
	Draft ConfigDraft
	MCP   MCPDraft
	Now   func() time.Time
}

// ApplyResult lists Atlas-owned and runtime paths created by Apply.
type ApplyResult struct {
	Directories  []string
	Files        []string
	RuntimeFiles []string
	BackupDir    string
	HomePath     string
	HomeCreated  bool
}

// ApplyConfig persists Atlas-owned configuration under .atlas/ and materializes
// allowlisted runtime entrypoints. It never modifies Git and never stores secrets.
func ApplyConfig(in ApplyInput) (ApplyResult, error) {
	root := filepath.Clean(strings.TrimSpace(in.Root))
	if root == "" || root == "." {
		return ApplyResult{}, fmt.Errorf("apply config: workspace root is required")
	}

	doc := BuildProjectDocument(in.Draft, in.MCP)
	if err := ValidateProjectDocument(doc); err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}

	now := time.Now().UTC()
	if in.Now != nil {
		now = in.Now().UTC()
	}

	targets := RuntimeTargets(doc)
	if err := validateRuntimeTargets(root, targets); err != nil {
		return ApplyResult{}, err
	}

	homeResult, err := home.EnsureAndMirror(now)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}

	result := ApplyResult{
		HomePath:    homeResult.HomePath,
		HomeCreated: homeResult.Created,
	}
	for _, rel := range []string{DirAtlas, DirBackups} {
		path, err := safeJoinAtlas(root, rel)
		if err != nil {
			return ApplyResult{}, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: create %s: %w", rel, err)
		}
		result.Directories = append(result.Directories, rel)
	}

	backupDir, _, err := BackupExistingTargets(root, targets, now)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: %w", err)
	}
	result.BackupDir = backupDir

	registry := RenderAgentRegistry(doc.Project.Name, doc.Adapters.Selected, homeResult.HomePath)
	manifest, err := RenderRuntimeManifestYAML(doc.Project.Name, doc.Adapters.Selected)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: render %s: %w", FileRuntimeManifest, err)
	}
	lockDoc := BuildAssetsLockDocumentFor(homeResult.HomePath, doc, version.Version)

	type atlasWrite struct {
		rel  string
		data []byte
	}
	var atlasFiles []atlasWrite
	for _, item := range []struct {
		rel  string
		data any
	}{
		{rel: FileConfig, data: doc},
		{rel: FileLocal, data: BuildLocalDocument(in.Draft)},
		{rel: FileAssetsLock, data: lockDoc},
	} {
		payload, marshalErr := marshalYAML(item.data)
		if marshalErr != nil {
			return ApplyResult{}, fmt.Errorf("apply config: marshal %s: %w", item.rel, marshalErr)
		}
		atlasFiles = append(atlasFiles, atlasWrite{rel: item.rel, data: payload})
	}
	atlasFiles = append(atlasFiles,
		atlasWrite{rel: FileAgentRegistry, data: []byte(registry)},
		atlasWrite{rel: FileRuntimeManifest, data: []byte(manifest)},
	)
	if DependsOnSDDOpenSpecContract(doc) {
		contract, renderErr := RenderSDDOpenSpecContract()
		if renderErr != nil {
			return ApplyResult{}, fmt.Errorf("apply config: render %s: %w", FileSDDOpenSpecContract, renderErr)
		}
		atlasFiles = append(atlasFiles, atlasWrite{rel: FileSDDOpenSpecContract, data: []byte(contract)})
	}
	for _, file := range atlasFiles {
		path, joinErr := safeJoinAtlas(root, file.rel)
		if joinErr != nil {
			return ApplyResult{}, joinErr
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: create parent for %s: %w", file.rel, err)
		}
		if err := os.WriteFile(path, file.data, 0o644); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: write %s: %w", file.rel, err)
		}
		result.Files = append(result.Files, file.rel)
	}

	for _, rel := range targets {
		full, err := safeJoinRuntime(root, rel)
		if err != nil {
			return ApplyResult{}, err
		}
		var existing []byte
		if data, readErr := os.ReadFile(full); readErr == nil {
			existing = data
		} else if !os.IsNotExist(readErr) {
			return ApplyResult{}, fmt.Errorf("apply config: read %s: %w", rel, readErr)
		}
		content, err := renderRuntimeFile(rel, doc, existing)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: create parent for %s: %w", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: write %s: %w", rel, err)
		}
		result.RuntimeFiles = append(result.RuntimeFiles, rel)
	}

	statePath, err := safeJoinAtlas(root, FileState)
	if err != nil {
		return ApplyResult{}, err
	}
	state := BuildStateDocument(in.Draft, now.Format(time.RFC3339), true)
	stateData, err := marshalYAML(state)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: marshal %s: %w", FileState, err)
	}
	if err := os.WriteFile(statePath, stateData, 0o644); err != nil {
		return ApplyResult{}, fmt.Errorf("apply config: write %s: %w", FileState, err)
	}
	result.Files = append(result.Files, FileState)

	return result, nil
}

// PersistConfigure writes the current draft/MCP state to .atlas/config.yaml only.
// It does not materialize runtime files and does not store credentials.
func PersistConfigure(in ApplyInput) error {
	root := filepath.Clean(strings.TrimSpace(in.Root))
	if root == "" || root == "." {
		return fmt.Errorf("persist configure: workspace root is required")
	}

	doc := BuildProjectDocument(in.Draft, in.MCP)
	if err := ValidateProjectDocument(doc); err != nil {
		return fmt.Errorf("persist configure: %w", err)
	}
	path, err := safeJoinAtlas(root, FileConfig)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("persist configure: create .atlas: %w", err)
	}
	data, err := marshalYAML(doc)
	if err != nil {
		return fmt.Errorf("persist configure: marshal %s: %w", FileConfig, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("persist configure: write %s: %w", FileConfig, err)
	}
	return nil
}

func validateRuntimeTargets(root string, targets []string) error {
	for _, rel := range targets {
		full, err := safeJoinRuntime(root, rel)
		if err != nil {
			return err
		}
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("apply config: stat %s: %w", rel, err)
		}
		if info.IsDir() {
			return fmt.Errorf("apply config: %s exists as a directory; expected a file", rel)
		}
	}
	return nil
}

func marshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isAllowedRuntimePath(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	switch clean {
	case FileAgentsMD, FileCursorAtlasMDC, FileOpenCodeAtlas:
		return true
	default:
		return IsAtlasAgentRuntimePath(clean)
	}
}

func assertAllowedAtlasPath(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("apply config: refused path %q", rel)
	}
	if clean != DirAtlas && !strings.HasPrefix(clean, DirAtlas+"/") {
		return fmt.Errorf("apply config: refused non-atlas path %q", rel)
	}
	for _, forbidden := range forbiddenApplyRelPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("apply config: refused forbidden path %q", rel)
		}
	}
	return nil
}

func assertAllowedRuntimePath(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("apply config: refused path %q", rel)
	}
	if !isAllowedRuntimePath(clean) {
		return fmt.Errorf("apply config: refused runtime path %q", rel)
	}
	for _, forbidden := range forbiddenApplyRelPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("apply config: refused forbidden path %q", rel)
		}
	}
	return nil
}

func safeJoinAtlas(root, rel string) (string, error) {
	if err := assertAllowedAtlasPath(rel); err != nil {
		return "", err
	}
	return safeJoinRoot(root, rel)
}

func safeJoinRuntime(root, rel string) (string, error) {
	if err := assertAllowedRuntimePath(rel); err != nil {
		return "", err
	}
	return safeJoinRoot(root, rel)
}

func assertAllowedConflictPath(rel string) error {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("backup: refused path %q", rel)
	}
	allowed := []string{
		FileAgentsMD,
		FileAgentRegistry,
		FileRuntimeManifest,
		FileAssetsLock,
		FileSDDOpenSpecContract,
		"AGENT.md",
		"CLAUDE.md",
		"GEMINI.md",
		".cursor",
		".opencode",
		".agents",
		".claude",
		".codex",
	}
	for _, prefix := range allowed {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return nil
		}
	}
	return fmt.Errorf("backup: refused conflict path %q", rel)
}

func safeJoinRoot(root, rel string) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("apply config: resolve root: %w", err)
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("apply config: resolve path: %w", err)
	}
	sep := string(os.PathSeparator)
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+sep) {
		return "", fmt.Errorf("apply config: path escapes workspace: %q", rel)
	}
	return full, nil
}
