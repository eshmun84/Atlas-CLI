package hooks

import "errors"

// Sentinel errors for stable classification by callers.
// Call sites may wrap with fmt.Errorf("%w: …", Err…) for detail.
// Callers must classify with errors.Is — never errmsg substring matching.
var (
	ErrUnknownEventType  = errors.New("hooks: unknown event type")
	ErrUnsupportedEvent  = errors.New("hooks: event unsupported by adapter")
	ErrInvalidEvent      = errors.New("hooks: invalid event")
	ErrInvalidAction     = errors.New("hooks: invalid action registration")
	ErrDuplicateAction   = errors.New("hooks: duplicate action id")
	ErrHooksDisabled     = errors.New("hooks: adapter hooks capability disabled")
	ErrActionTimeout     = errors.New("hooks: action timed out")
	ErrActionCancelled   = errors.New("hooks: action cancelled")
	ErrDispatchAborted   = errors.New("hooks: dispatch aborted")
	ErrOwnershipConflict = errors.New("hooks: ownership conflict")
	ErrContinueDenied    = errors.New("hooks: action denied continuation")
)
