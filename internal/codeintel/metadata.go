package codeintel

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// LoadMetadata reads Atlas-owned metadata.json under explicit Home.
// Missing regular file returns (zero, false, nil).
// Symlink / unsafe leaf returns error and never follows the target.
func LoadMetadata(homePath, metadataPath string) (Metadata, bool, error) {
	metadataPath = strings.TrimSpace(metadataPath)
	if metadataPath == "" {
		return Metadata{}, false, nil
	}
	data, present, err := ReadStorageLeaf(homePath, metadataPath)
	if err != nil {
		return Metadata{}, false, err
	}
	if !present {
		return Metadata{}, false, nil
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

// WriteMetadataAtomic writes metadata via symlink-safe temp+rename under the
// explicit Atlas Home root. metadataPath must be contained beneath homePath;
// there is no fallback that promotes the metadata parent to a trusted root.
func WriteMetadataAtomic(homePath, metadataPath string, meta Metadata) error {
	homePath = strings.TrimSpace(homePath)
	metadataPath = strings.TrimSpace(metadataPath)
	if homePath == "" {
		return fmt.Errorf("codeintel: metadata write requires Atlas Home path")
	}
	if metadataPath == "" {
		return fmt.Errorf("codeintel: metadata path is required")
	}
	if err := ValidateMetadata(meta); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("codeintel: marshal metadata: %w", err)
	}
	data = append(data, '\n')

	homeAbs, err := filepath.Abs(homePath)
	if err != nil {
		return fmt.Errorf("codeintel: resolve Atlas Home: %w", err)
	}
	metaAbs, err := filepath.Abs(metadataPath)
	if err != nil {
		return fmt.Errorf("codeintel: metadata path: %w", err)
	}
	rel, err := filepath.Rel(homeAbs, metaAbs)
	if err != nil {
		return fmt.Errorf("codeintel: metadata containment: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("codeintel: metadata path escapes Atlas Home")
	}
	if err := fsafety.AtomicWriteContainedDir(homeAbs, rel, data, 0o600, home.DirPermHome, ".atlas-meta-*.tmp"); err != nil {
		return fmt.Errorf("codeintel: write metadata: %w", err)
	}
	return nil
}
