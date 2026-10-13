package memorypersist

import (
	"errors"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

var (
	// ErrNotInitialized reports missing Atlas project identity/config.
	ErrNotInitialized = errors.New("memorypersist: atlas project not initialized")
	// ErrUnsafeFilesystem reports a fail-closed path safety refusal.
	ErrUnsafeFilesystem = errors.New("memorypersist: unsafe filesystem")
	// ErrPersistenceDisabled reports memory persistence mode off.
	ErrPersistenceDisabled = errors.New("memorypersist: persistence disabled")
)

// ExitError signals a non-zero process exit after JSON was written to stdout.
type ExitError struct {
	Status string
}

func (e ExitError) Error() string {
	if e.Status == "" {
		return "memorypersist: request failed"
	}
	return "memorypersist: " + e.Status
}

// ShouldExitNonZero reports whether status is a hard failure for process exit.
// Policy outcomes (rejected / confirmation_required / duplicate / disabled) are
// soft: development continues.
func ShouldExitNonZero(status string) bool {
	switch status {
	case StatusAccepted, StatusDuplicate, StatusUpdated,
		StatusRejected, StatusConfirmationRequired, StatusDisabled:
		return false
	default:
		return true
	}
}

func classify(err error) (status, message string) {
	if err == nil {
		return StatusAccepted, ""
	}
	switch {
	case errors.Is(err, ErrPersistenceDisabled):
		return StatusDisabled, "memory persistence is disabled"
	case errors.Is(err, memory.ErrDatabaseNotFound):
		return StatusUnavailable, "memory database unavailable"
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
	case errors.Is(err, ErrNotInitialized),
		errors.Is(err, os.ErrNotExist):
		return StatusNotInitialized, "atlas project not initialized"
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "database is locked") || strings.Contains(lower, "sqlite_busy") {
			return StatusUnavailable, "memory database unavailable"
		}
		return StatusInternalError, "memory persistence failed"
	}
}

func errorResponse(status, message string) Response {
	if message == "" {
		message = status
	}
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       CommandPropose,
		Status:        status,
		Error: &ErrorBody{
			Code:    status,
			Message: message,
		},
	}
}
