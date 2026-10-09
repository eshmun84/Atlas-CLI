package home

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

const StateSchemaVersion = 1

// StateDocument is $ATLAS_HOME/state/home.yaml.
type StateDocument struct {
	SchemaVersion int               `yaml:"schema_version"`
	HomePath      string            `yaml:"home_path"`
	CreatedAt     string            `yaml:"created_at,omitempty"`
	UpdatedAt     string            `yaml:"updated_at,omitempty"`
	Source        string            `yaml:"source"`
	AtlasVersion  string            `yaml:"atlas_version,omitempty"`
	Assets        []StateAssetEntry `yaml:"assets"`
}

// StateAssetEntry is minimal metadata for one mirrored Home asset.
type StateAssetEntry struct {
	Family       string `yaml:"family"`
	ID           string `yaml:"id"`
	Version      string `yaml:"version,omitempty"`
	Checksum     string `yaml:"checksum,omitempty"`
	Source       string `yaml:"source"`
	ResolvedPath string `yaml:"resolved_path"`
}

// StateRelPath is the Home-relative path of state/home.yaml.
const StateRelPath = "state/home.yaml"

// LoadState reads home state via symlink-safe contained read of state/home.yaml.
// Missing file returns empty doc + false. Symlink parents/leaves, non-regular
// files, and containment failures return error (fail closed before mutation).
func LoadState(homePath string) (StateDocument, bool, error) {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return StateDocument{}, false, fmt.Errorf("atlas home: home path is required")
	}
	data, err := fsafety.ReadFileContained(homePath, StateRelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return StateDocument{}, false, nil
		}
		return StateDocument{}, false, fmt.Errorf("atlas home: read state: %w", err)
	}
	var doc StateDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return StateDocument{}, false, fmt.Errorf("atlas home: parse state: %w", err)
	}
	return doc, true, nil
}

func marshalHomeState(doc StateDocument) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("atlas home: marshal state: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("atlas home: marshal state: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteState persists home state metadata.
func WriteState(homePath string, doc StateDocument) error {
	data, err := marshalHomeState(doc)
	if err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Join("state", "home.yaml"))
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, DirPermHome, ".atlas-home-*.tmp"); err != nil {
		return fmt.Errorf("atlas home: write state: %w", err)
	}
	return nil
}

func stamp(now time.Time) string {
	return now.UTC().Format(time.RFC3339)
}
