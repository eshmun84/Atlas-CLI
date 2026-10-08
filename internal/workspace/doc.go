// Package workspace is a compatibility facade pending removal.
//
// Slice 31.1 moved ownership to:
//
//	internal/inspect  — canonical Inspection (project.Snapshot + runtime.Health)
//	internal/project  — project facts
//	internal/runtime  — Atlas runtime health + repair
//	internal/delivery — Delivery mode meaning
//
// Do not add new productive consumers of this package. Prefer:
//
//	inspect.Inspect / inspect.Inspection
//	project.* / runtime.* / delivery.*
//
// Existing aliases and Discover forwarding exist only to keep legacy tests and
// gradual migration green.
package workspace
