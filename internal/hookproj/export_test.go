package hookproj

// SetApplyPhaseHookForTest installs a test-only seam at named apply phases.
// Phases: before_apply, after_wrapper, after_config, before_ownership,
// after_ownership, after_config_remove, after_wrapper_remove.
func SetApplyPhaseHookForTest(fn func(phase string) error) {
	applyPhaseHook = fn
}
