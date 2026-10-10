package fsafety

import "errors"

// Sentinel errors for fail-closed path safety. Call sites wrap with %w for detail.
// Callers should classify with errors.Is — never by matching errmsg text.
var (
	ErrSymlink        = errors.New("fsafety: symlink refused")
	ErrPathTraversal  = errors.New("fsafety: path traversal refused")
	ErrPathEscape     = errors.New("fsafety: path escapes root")
	ErrAbsolutePath   = errors.New("fsafety: absolute path refused")
	ErrNotRegularFile = errors.New("fsafety: not a regular file")
)
