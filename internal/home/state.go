package home

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

// LoadState reads home state when present. Missing file returns empty doc + false.
func LoadState(homePath string) (StateDocument, bool, error) {
	path := StateFile(homePath)
	data, err := os.ReadFile(path)
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

// WriteState persists home state metadata.
func WriteState(homePath string, doc StateDocument) error {
	path := StateFile(homePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("atlas home: create state dir: %w", err)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return fmt.Errorf("atlas home: marshal state: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("atlas home: marshal state: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("atlas home: write state: %w", err)
	}
	return nil
}

func stamp(now time.Time) string {
	return now.UTC().Format(time.RFC3339)
}
