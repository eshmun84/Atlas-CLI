package codegraphsource

// Test hooks for snapshot open safety.
var (
	PrepareQuerySnapshotForTest   = prepareQuerySnapshot
	CopyRegularFileBoundedForTest = copyRegularFileBounded
	MaxSnapshotBytesForTest       = maxSnapshotBytes
	SnapshotOpenStrategyForTest   = SnapshotOpenStrategy
)
