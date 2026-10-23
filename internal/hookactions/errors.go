package hookactions

import (
	"context"
	"errors"
)

// Sentinel admission / worker errors. Mapped to fail-open Outcomes; public
// hooks JSON still uses fixed outcome messages (no sensitive detail leaked).
var (
	errMaintainBusy     = errors.New("hookactions: maintain already in progress")
	errAdmitLeaseActive = errors.New("hookactions: maintain admission lease active")
	errAdmitSpawn       = errors.New("hookactions: maintain worker spawn failed")
	errAdmitProject     = errors.New("hookactions: maintain project invalid")
	errAdmitUnexpected  = errors.New("hookactions: maintain admission unexpected error")

	// errPendingCorrupt is returned when maintain-pending.yaml exists but cannot
	// be trusted (parse failure or incompatible schema). Callers must not treat
	// this as an empty queue and must not overwrite the evidence file.
	errPendingCorrupt            = errors.New("hookactions: maintain-pending corrupt or incompatible")
	errPendingMalformed          = errors.New("hookactions: maintain-pending malformed")
	errPendingSchemaIncompatible = errors.New("hookactions: maintain-pending schema incompatible")

	errLeaseCorrupt            = errors.New("hookactions: maintain lease corrupt or incompatible")
	errLeaseMalformed          = errors.New("hookactions: maintain lease malformed")
	errLeaseSchemaIncompatible = errors.New("hookactions: maintain lease schema incompatible")

	errSessionCorrupt            = errors.New("hookactions: session-maintain corrupt or incompatible")
	errSessionMalformed          = errors.New("hookactions: session-maintain malformed")
	errSessionSchemaIncompatible = errors.New("hookactions: session-maintain schema incompatible")
)

// ErrPendingCorrupt reports whether err indicates untrusted pending state.
func ErrPendingCorrupt(err error) bool {
	return errors.Is(err, errPendingCorrupt)
}

func pendingCorruptionRecoverable(err error) bool {
	return errors.Is(err, errPendingMalformed)
}

func leaseCorruptionRecoverable(err error) bool {
	return errors.Is(err, errLeaseMalformed) && !errors.Is(err, errLeaseSchemaIncompatible)
}

func sessionCorruptionRecoverable(err error) bool {
	return errors.Is(err, errSessionMalformed) && !errors.Is(err, errSessionSchemaIncompatible)
}

// AdmitClass is an internal classification for diagnostics and tests.
type AdmitClass string

const (
	AdmitClassOK          AdmitClass = "ok"
	AdmitClassSkipped     AdmitClass = "skipped"
	AdmitClassBusy        AdmitClass = "busy"
	AdmitClassLeaseActive AdmitClass = "lease_active"
	AdmitClassSpawnFail   AdmitClass = "spawn_failure"
	AdmitClassInvalidProj AdmitClass = "invalid_project"
	AdmitClassTimeout     AdmitClass = "timeout"
	AdmitClassCancelled   AdmitClass = "cancelled"
	AdmitClassUnexpected  AdmitClass = "unexpected"
)

// ClassifyAdmitError maps an admission error to a non-sensitive internal class.
// Public hooks JSON messages remain fixed; this is for tests and diagnostics.
func ClassifyAdmitError(err error) AdmitClass {
	if err == nil {
		return AdmitClassOK
	}
	switch {
	case errors.Is(err, errMaintainBusy):
		return AdmitClassBusy
	case errors.Is(err, errAdmitLeaseActive):
		return AdmitClassLeaseActive
	case errors.Is(err, errAdmitSpawn):
		return AdmitClassSpawnFail
	case errors.Is(err, errAdmitProject):
		return AdmitClassInvalidProj
	case errors.Is(err, context.Canceled):
		return AdmitClassCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return AdmitClassTimeout
	default:
		return AdmitClassUnexpected
	}
}
