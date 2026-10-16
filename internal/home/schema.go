package home

import (
	"errors"
	"fmt"
)

// CurrentSchemaVersion is the authoritative Atlas Home schema this binary supports.
// Persisted as state/home.yaml schema_version. Do not scatter literal schema numbers.
const CurrentSchemaVersion = 1

// StateSchemaVersion is the schema stamped into state/home.yaml (alias of Current).
const StateSchemaVersion = CurrentSchemaVersion

// Schema compatibility sentinels (Home surface).
var (
	ErrSchemaNewer     = errors.New("home schema newer than this Atlas supports")
	ErrSchemaOlder     = errors.New("home schema older and unsupported")
	ErrSchemaMissing   = errors.New("home schema missing")
	ErrSchemaMalformed = errors.New("home schema malformed")
)

// SchemaError describes a Home schema compatibility failure without claiming mutation.
type SchemaError struct {
	Found   int
	Current int
	Kind    error // one of ErrSchema*
}

func (e *SchemaError) Error() string {
	if e == nil {
		return "atlas home schema error"
	}
	switch {
	case errors.Is(e.Kind, ErrSchemaNewer):
		return fmt.Sprintf("Atlas Home schema %d is newer than this Atlas supports (%d); no files were modified; use an Atlas version that supports schema %d",
			e.Found, e.Current, e.Found)
	case errors.Is(e.Kind, ErrSchemaOlder):
		return fmt.Sprintf("Atlas Home schema %d is older than this Atlas supports (%d); no migration is available; no files were modified",
			e.Found, e.Current)
	case errors.Is(e.Kind, ErrSchemaMissing):
		return fmt.Sprintf("Atlas Home state is present but schema_version is missing; no files were modified")
	case errors.Is(e.Kind, ErrSchemaMalformed):
		return fmt.Sprintf("Atlas Home schema_version is malformed; no files were modified")
	default:
		return fmt.Sprintf("Atlas Home schema incompatible (found %d, supported %d)", e.Found, e.Current)
	}
}

func (e *SchemaError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Kind
}

// CheckStateSchema validates a loaded Home state document for mutation/operation.
// Call only when state/home.yaml is present and parsed. Product atlas_version is
// provenance only and is not consulted here.
func CheckStateSchema(doc StateDocument) error {
	v := doc.SchemaVersion
	if v < 0 {
		return &SchemaError{Found: v, Current: CurrentSchemaVersion, Kind: ErrSchemaMalformed}
	}
	if v == 0 {
		return &SchemaError{Found: 0, Current: CurrentSchemaVersion, Kind: ErrSchemaMissing}
	}
	if v > CurrentSchemaVersion {
		return &SchemaError{Found: v, Current: CurrentSchemaVersion, Kind: ErrSchemaNewer}
	}
	if v < CurrentSchemaVersion {
		return &SchemaError{Found: v, Current: CurrentSchemaVersion, Kind: ErrSchemaOlder}
	}
	return nil
}
