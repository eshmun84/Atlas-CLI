package memoryretrieve

import (
	"errors"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// classify maps domain errors onto stable CLI status codes.
// Uses errors.Is against typed sentinels only — never errmsg substring matching.
// Messages are intentionally generic — never echo paths or raw SQL.
func classify(err error) (status, message string) {
	if err == nil {
		return StatusSuccess, ""
	}

	switch {
	case errors.Is(err, memory.ErrDatabaseNotFound):
		return StatusMissingMemoryDB, "memory database not found"
	case errors.Is(err, memory.ErrInvalidSearchQuery),
		errors.Is(err, memory.ErrSearchQueryRequired):
		return StatusInvalidQuery, "invalid search query"
	case errors.Is(err, memory.ErrSchemaIncompatible):
		return StatusIncompatibleSchema, "memory schema incompatible"
	case errors.Is(err, memory.ErrSymlinkDatabase),
		errors.Is(err, memory.ErrNotRegularDatabase),
		errors.Is(err, memory.ErrUnsafePath),
		errors.Is(err, ErrUnsafeFilesystem),
		errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return StatusUnsafeFilesystem, "filesystem path unsafe"
	case errors.Is(err, memory.ErrObservationNotFound):
		return StatusNotFound, "observation not found"
	case errors.Is(err, ErrNotInitialized),
		errors.Is(err, os.ErrNotExist):
		return StatusNotInitialized, "atlas project not initialized"
	default:
		return StatusInternalError, "memory request failed"
	}
}

func errorResponse(command, status, message string) Response {
	if message == "" {
		message = status
	}
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Status:        status,
		Count:         0,
		Error: &ErrorBody{
			Code:    status,
			Message: message,
		},
	}
}

// ExitError signals a non-zero process exit after JSON was written to stdout.
type ExitError struct {
	Status string
}

func (e ExitError) Error() string {
	if e.Status == "" {
		return "memory: request failed"
	}
	return "memory: " + e.Status
}

// ShouldExitNonZero reports whether status is a failure for process exit.
func ShouldExitNonZero(status string) bool {
	switch status {
	case StatusSuccess, StatusNoResults:
		return false
	default:
		return true
	}
}
