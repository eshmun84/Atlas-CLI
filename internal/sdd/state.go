package sdd

import "strings"

// NormalizeLifecycle maps free-form / ambiguous labels to an explicit state.
// Unknown inputs become LifecycleUnknown (fail-closed).
func NormalizeLifecycle(raw string) LifecycleState {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(LifecycleUnknown):
		return LifecycleUnknown
	case string(LifecycleIncomplete):
		return LifecycleIncomplete
	case string(LifecycleActive):
		return LifecycleActive
	case string(LifecycleReady):
		return LifecycleReady
	case string(LifecycleVerified):
		return LifecycleVerified
	case string(LifecycleArchived):
		return LifecycleArchived
	default:
		return LifecycleUnknown
	}
}

// NormalizeVerification maps free-form labels fail-closed.
func NormalizeVerification(raw string) VerificationState {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(VerificationUnknown):
		return VerificationUnknown
	case string(VerificationNone):
		return VerificationNone
	case string(VerificationIncomplete):
		return VerificationIncomplete
	case string(VerificationPass):
		return VerificationPass
	case string(VerificationFail):
		return VerificationFail
	default:
		return VerificationUnknown
	}
}

// IsTerminal reports whether the lifecycle is a closed/archived state.
func IsTerminal(s LifecycleState) bool {
	return s == LifecycleArchived
}

// IsGovernable reports whether Atlas can treat the change as an in-flight unit.
func IsGovernable(s LifecycleState) bool {
	switch s {
	case LifecycleActive, LifecycleReady, LifecycleVerified:
		return true
	default:
		return false
	}
}
