package memory

import "errors"

// Sentinel errors for stable classification by callers (Slice 36A+).
// Call sites may wrap these with fmt.Errorf("%w: …", Err…) for detail.
var (
	ErrDatabaseNotFound    = errors.New("memory: database not found")
	ErrInvalidSearchQuery  = errors.New("memory: invalid search query")
	ErrSearchQueryRequired = errors.New("memory: search query is required")
	ErrObservationNotFound = errors.New("memory: observation not found")
	ErrSchemaIncompatible  = errors.New("memory: schema incompatible")
	ErrUnsafePath          = errors.New("memory: path unsafe")
	ErrSymlinkDatabase     = errors.New("memory: refusing symlink database")
	ErrNotRegularDatabase  = errors.New("memory: database path is not a regular file")
)
