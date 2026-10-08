package codeintel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadMetadata reads Atlas-owned metadata.json. Missing file returns (zero, false, nil).
func LoadMetadata(path string) (Metadata, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Metadata{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, false, nil
		}
		return Metadata{}, false, fmt.Errorf("codeintel: read metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, false, fmt.Errorf("codeintel: parse metadata: %w", err)
	}
	if err := ValidateMetadata(meta); err != nil {
		return meta, true, err
	}
	return meta, true, nil
}

// ValidateMetadata checks required Atlas-owned fields.
func ValidateMetadata(meta Metadata) error {
	if meta.SchemaVersion != MetadataSchemaVersion {
		return fmt.Errorf("codeintel: unsupported metadata schemaVersion %d", meta.SchemaVersion)
	}
	if strings.TrimSpace(meta.Provider) == "" {
		return fmt.Errorf("codeintel: metadata missing provider")
	}
	if strings.TrimSpace(meta.ProjectID) == "" {
		return fmt.Errorf("codeintel: metadata missing projectID")
	}
	if strings.TrimSpace(meta.SourceFingerprint) == "" {
		return fmt.Errorf("codeintel: metadata missing sourceFingerprint")
	}
	return nil
}

// WriteMetadataAtomic writes metadata via temp file + rename.
func WriteMetadataAtomic(path string, meta Metadata) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("codeintel: metadata path is required")
	}
	if err := ValidateMetadata(meta); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("codeintel: create metadata dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("codeintel: marshal metadata: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("codeintel: write metadata temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("codeintel: replace metadata: %w", err)
	}
	return nil
}
