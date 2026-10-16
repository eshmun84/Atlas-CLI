package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// CurrentProjectSchemaVersion is the authoritative .atlas/config.yaml schema
// this Atlas binary supports. Schema compatibility is independent of product
// version.Version (atlas_version is provenance only).
const CurrentProjectSchemaVersion = 1

// Project schema compatibility sentinels.
var (
	ErrProjectSchemaNewer     = errors.New("project schema newer than this Atlas supports")
	ErrProjectSchemaOlder     = errors.New("project schema older and unsupported")
	ErrProjectSchemaMalformed = errors.New("project schema malformed")
)

// ProjectSchemaError describes a project .atlas schema compatibility failure.
type ProjectSchemaError struct {
	Found   int
	Current int
	Kind    error
}

func (e *ProjectSchemaError) Error() string {
	if e == nil {
		return "project schema error"
	}
	switch {
	case errors.Is(e.Kind, ErrProjectSchemaNewer):
		return fmt.Sprintf("project schema %d is newer than this Atlas supports (%d); no files were modified; use an Atlas version that supports schema %d",
			e.Found, e.Current, e.Found)
	case errors.Is(e.Kind, ErrProjectSchemaOlder):
		return fmt.Sprintf("project schema %d is older than this Atlas supports (%d); no migration is available; no files were modified",
			e.Found, e.Current)
	case errors.Is(e.Kind, ErrProjectSchemaMalformed):
		return fmt.Sprintf("project schema_version is malformed; no files were modified")
	default:
		return fmt.Sprintf("project schema incompatible (found %d, supported %d)", e.Found, e.Current)
	}
}

func (e *ProjectSchemaError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Kind
}

// CheckProjectSchema validates an existing project document for mutation.
//
// Legacy policy: schema_version omitted/0 on an otherwise present config is
// treated as CurrentProjectSchemaVersion (compatible). Fresh projects with no
// config.yaml are not checked here (call RequireProjectSchemaCompatible).
// Product atlas_version is not consulted.
func CheckProjectSchema(doc ProjectDocument) error {
	v := doc.SchemaVersion
	if v < 0 {
		return &ProjectSchemaError{Found: v, Current: CurrentProjectSchemaVersion, Kind: ErrProjectSchemaMalformed}
	}
	if v == 0 {
		// Legacy Atlas projects predating the config schema marker.
		return nil
	}
	if v > CurrentProjectSchemaVersion {
		return &ProjectSchemaError{Found: v, Current: CurrentProjectSchemaVersion, Kind: ErrProjectSchemaNewer}
	}
	if v < CurrentProjectSchemaVersion {
		return &ProjectSchemaError{Found: v, Current: CurrentProjectSchemaVersion, Kind: ErrProjectSchemaOlder}
	}
	return nil
}

// RequireProjectSchemaCompatible loads .atlas/config.yaml when present and
// refuses mutation on unsupported schema skew. Missing config (fresh project)
// is allowed. Unreadable/malformed config fails closed.
func RequireProjectSchemaCompatible(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return fmt.Errorf("project schema: workspace root is required")
	}
	full, err := fsafety.ContainedJoin(root, FileConfig)
	if err != nil {
		return fmt.Errorf("project schema: %w", err)
	}
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("project schema: inspect %s: %w", FileConfig, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("project schema: %s is not a safe regular file", FileConfig)
	}
	doc, err := LoadProjectDocumentAt(root)
	if err != nil {
		return fmt.Errorf("project schema: existing config unreadable: %w", err)
	}
	return CheckProjectSchema(doc)
}
