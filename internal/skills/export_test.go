package skills

// SetReconcileAfterWriteHookForTest installs a test-only seam after projected
// writes are digest-verified and before ownership save.
func SetReconcileAfterWriteHookForTest(fn func() error) {
	reconcileAfterWriteHook = fn
}
