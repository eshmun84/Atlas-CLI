package hookactions

import "sync/atomic"

// TriggersEnabled controls whether registered maintenance Actions perform
// admission (session spawn or post-code bump/scheduler) when invoked via
// hooks.Action.Run.
//
// Default is true (Phase B session + Phase C.2c post-code).
// Direct Maintain* helpers always run inline and ignore this flag.
//
// Tests may call SetTriggersEnabled(false) to restore Phase A skip behavior.
var TriggersEnabled atomic.Bool

func init() {
	// Phase B: automatic session maintenance is on by default.
	TriggersEnabled.Store(true)
}

// SetTriggersEnabled is a test/production seam for the trigger gate.
func SetTriggersEnabled(enabled bool) {
	TriggersEnabled.Store(enabled)
}

// TriggersAreEnabled reports the current gate state.
func TriggersAreEnabled() bool {
	return TriggersEnabled.Load()
}
