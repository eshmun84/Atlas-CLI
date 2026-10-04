package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ApplySuccessTitle        = "Atlas configuration initialized."
	ApplySuccessBody         = "No runtime files were materialized."
	ConfigureApplySuccess    = "Configuration changes saved."
	ConfigureApplyFooterNote = "Close discards unsaved changes. Apply changes writes .atlas/config.yaml."
)

// Runtime paths that Apply must never create, modify, backup, replace, or delete.
var forbiddenApplyRelPaths = []string{
	"AGENTS.md",
	".cursor",
	".opencode",
	".agents",
	".claude",
	"CLAUDE.md",
	"GEMINI.md",
	"README.md",
	".gitignore",
	FileMemoryDB,
	FileCapsule,
}

// ApplyInput is the in-memory init state used to persist Atlas config files.
type ApplyInput struct {
	Root  string
	Draft ConfigDraft
	MCP   MCPDraft
	Now   func() time.Time
}

// ApplyResult lists Atlas-owned paths created by Apply.
type ApplyResult struct {
	Directories []string
	Files       []string
}

// ApplyConfig persists Atlas-owned configuration under .atlas/.
// It never materializes runtime files, never modifies Git, and never stores secrets.
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

	files := []struct {
		rel  string
		data any
	}{
		{rel: FileConfig, data: doc},
		{rel: FileLocal, data: BuildLocalDocument(in.Draft)},
		{rel: FileState, data: BuildStateDocument(in.Draft, now.Format(time.RFC3339))},
		{rel: FileAssetsLock, data: BuildAssetsLockDocument()},
	}

	dirs := []string{DirAtlas, DirBackups}
	for _, rel := range dirs {
		if err := assertAllowedAtlasPath(rel); err != nil {
			return ApplyResult{}, err
		}
	}
	for _, file := range files {
		if err := assertAllowedAtlasPath(file.rel); err != nil {
			return ApplyResult{}, err
		}
	}

	result := ApplyResult{}
	for _, rel := range dirs {
		path, err := safeJoin(root, rel)
		if err != nil {
			return ApplyResult{}, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: create %s: %w", rel, err)
		}
		result.Directories = append(result.Directories, rel)
	}

	for _, file := range files {
		path, err := safeJoin(root, file.rel)
		if err != nil {
			return ApplyResult{}, err
		}
		data, err := marshalYAML(file.data)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: marshal %s: %w", file.rel, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return ApplyResult{}, fmt.Errorf("apply config: write %s: %w", file.rel, err)
		}
		result.Files = append(result.Files, file.rel)
	}

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
	if err := assertAllowedAtlasPath(FileConfig); err != nil {
		return err
	}
	path, err := safeJoin(root, FileConfig)
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

func safeJoin(root, rel string) (string, error) {
	if err := assertAllowedAtlasPath(rel); err != nil {
		return "", err
	}
	full := filepath.Join(root, rel)
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
