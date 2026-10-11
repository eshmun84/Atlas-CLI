package hooksengine

import (
	"errors"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

var (
	// ErrNotInitialized reports missing Atlas project identity/config.
	ErrNotInitialized = errors.New("hooksengine: atlas project not initialized")
	// ErrUnsafeFilesystem reports a fail-closed path safety refusal.
	ErrUnsafeFilesystem = errors.New("hooksengine: unsafe filesystem")
	// ErrProjectMismatch reports payload project_id ≠ resolved project identity.
	ErrProjectMismatch = errors.New("hooksengine: project identity mismatch")
	// ErrInvalidAdapter reports an unknown or empty adapter id.
	ErrInvalidAdapter = errors.New("hooksengine: invalid adapter")
	// ErrInvalidRequest reports malformed/oversized/unknown-field request JSON.
	ErrInvalidRequest = errors.New("hooksengine: invalid request")
)

// ExitError signals a non-zero process exit after JSON was written to stdout.
type ExitError struct {
	Status string
}

func (e ExitError) Error() string {
	if e.Status == "" {
		return "hooks: request failed"
	}
	return "hooks: " + e.Status
}

// ShouldExitNonZero reports whether status is a failure for process exit.
func ShouldExitNonZero(status string) bool {
	return status != StatusSuccess
}

// classify maps domain errors onto stable CLI status codes.
// Uses errors.Is against typed sentinels only — never errmsg substring matching.
func classify(err error) (status, message string) {
	if err == nil {
		return StatusSuccess, ""
	}
	switch {
	case errors.Is(err, ErrInvalidAdapter):
		return StatusInvalidAdapter, "invalid adapter identity"
	case errors.Is(err, ErrInvalidRequest):
		return StatusInvalidRequest, "invalid hooks request"
	case errors.Is(err, ErrProjectMismatch):
		return StatusProjectMismatch, "project identity mismatch"
	case errors.Is(err, ErrNotInitialized),
		errors.Is(err, os.ErrNotExist):
		return StatusNotInitialized, "atlas project not initialized"
	case errors.Is(err, ErrUnsafeFilesystem),
		errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return StatusUnsafeFilesystem, "filesystem path unsafe"
	case errors.Is(err, hooks.ErrHooksDisabled):
		return StatusHooksDisabled, "adapter hooks capability disabled"
	case errors.Is(err, hooks.ErrUnsupportedEvent):
		return StatusUnsupportedEvent, "event unsupported by adapter"
	case errors.Is(err, hooks.ErrUnknownEventType),
		errors.Is(err, hooks.ErrInvalidEvent):
		return StatusInvalidEvent, "invalid event"
	case errors.Is(err, hooks.ErrContinueDenied):
		return StatusDenied, "action denied continuation"
	case errors.Is(err, hooks.ErrDispatchAborted):
		return StatusAborted, "dispatch aborted"
	case errors.Is(err, hooks.ErrActionCancelled):
		return StatusCancelled, "action cancelled"
	case errors.Is(err, hooks.ErrActionTimeout):
		return StatusAborted, "action timed out"
	default:
		return StatusInternalError, "hooks request failed"
	}
}
