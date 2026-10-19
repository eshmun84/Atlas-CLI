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
)

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
