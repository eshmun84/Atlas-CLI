package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// LoadAt reads and validates Config from .atlas/config.yaml under root using
// symlink-safe contained reads. Symlink parents/leaves are refused.
func LoadAt(root string) (Config, error) {
	doc, err := LoadProjectDocumentAt(root)
	if err == nil {
		cfg := doc.ToConfig()
		if err := Validate(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	// Fall back to legacy Config shape via the same contained bytes.
	data, readErr := fsafety.ReadFileContained(root, FileConfig)
	if readErr != nil {
		return Config{}, fmt.Errorf("load config: %w", readErr)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadProjectDocumentAt reads .atlas/config.yaml under root with symlink-safe
// containment. External symlink targets are never parsed as Atlas config.
func LoadProjectDocumentAt(root string) (ProjectDocument, error) {
	data, err := fsafety.ReadFileContained(root, FileConfig)
	if err != nil {
		return ProjectDocument{}, fmt.Errorf("load config: %w", err)
	}
	return parseProjectDocument(data)
}

func parseProjectDocument(data []byte) (ProjectDocument, error) {
	var persisted ProjectDocument
	if err := yaml.Unmarshal(data, &persisted); err != nil {
		return ProjectDocument{}, fmt.Errorf("parse config: %w", err)
	}
	if !persisted.IsProjectDocument() {
		return ProjectDocument{}, fmt.Errorf("load config: not a persisted project document")
	}
	if err := ValidateProjectDocument(persisted); err != nil {
		return ProjectDocument{}, err
	}
	return persisted, nil
}

// LoadStateDocumentAt reads .atlas/state.yaml under root with symlink-safe
// containment. External symlink targets are never parsed as Atlas state.
func LoadStateDocumentAt(root string) (StateDocument, error) {
	data, err := fsafety.ReadFileContained(root, FileState)
	if err != nil {
		return StateDocument{}, fmt.Errorf("load state: %w", err)
	}
	return parseStateDocument(data)
}

func parseStateDocument(data []byte) (StateDocument, error) {
	var state StateDocument
	if err := yaml.Unmarshal(data, &state); err != nil {
		return StateDocument{}, fmt.Errorf("parse state: %w", err)
	}
	return state, nil
}

// AtlasOwnedFileExists reports whether an Atlas-owned relative path exists as a
// regular non-symlink file under root.
//
//	missing        => false, nil
//	regular file   => true, nil
//	directory      => false, error (malformed / not a file)
//	unsafe symlink => false, error
//	other non-reg  => false, error
func AtlasOwnedFileExists(root, rel string) (bool, error) {
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// ContainedJoin symlink refusal is an error (unsafe), not "missing".
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("atlas-owned: not a regular file: %s", rel)
	}
	return true, nil
}

// AtlasOwnedRel returns the project-relative slash path for a joined abs path under root.
func AtlasOwnedRel(root, abs string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, absPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
