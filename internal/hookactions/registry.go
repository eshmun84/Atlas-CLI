package hookactions

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// Registrations returns the Atlas-owned maintenance Action registrations.
// Order is irrelevant: hooks.NewRegistry sorts by action ID.
//
// Phase B: TriggersEnabled defaults to true. Session Actions admit quickly and
// spawn a detached worker; timeouts here bound admission only.
// Phase C.2c: PostCodeAction admits on tool.after_execution (allowlisted tools)
// via BumpPendingAndEnsureScheduler — never waits for CE/CI.
// Direct Maintain* helpers remain available for workers/tests.
func Registrations() []hooks.Registration {
	return []hooks.Registration{
		{
			Action:        &ContextEconomyAction{},
			FailurePolicy: hooks.FailContinue,
			Timeout:       AdmissionTimeout,
		},
		{
			Action:        &CodeIntelAction{},
			FailurePolicy: hooks.FailContinue,
			Timeout:       AdmissionTimeout,
		},
		{
			Action:        &PostCodeAction{},
			FailurePolicy: hooks.FailContinue,
			Timeout:       AdmissionTimeout,
		},
	}
}

// NewRegistry builds a hooks.Registry containing only maintenance Actions.
func NewRegistry() (hooks.Registry, error) {
	reg, err := hooks.NewRegistry(Registrations())
	if err != nil {
		return hooks.Registry{}, fmt.Errorf("hookactions: %w", err)
	}
	return reg, nil
}
