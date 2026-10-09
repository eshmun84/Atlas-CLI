package mutatelock

// AcceptSelfOwned reports whether stUid matches the process effective UID.
// Extracted for portable unit tests of the ownership fail-closed decision.
func AcceptSelfOwned(stUID uint32, euid int) bool {
	return int(stUID) == euid
}

// AcceptCacheBaseMode reports whether mode has no group/other write bits
// (mode & 0o022 == 0). Does not require owner-only 0700 — UserCacheDir is
// shared OS state and must not be chmod'd by Atlas.
func AcceptCacheBaseMode(mode uint32) bool {
	return mode&0o022 == 0
}
