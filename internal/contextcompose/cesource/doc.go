// Package cesource is the thin, read-only Context Economy source adapter for
// Context Composer (Slice 36B.6D).
//
// It loads existing Atlas Home CE artifacts via internal/context readers
// (LoadIndexAt, InspectContextLeaf, IsStale) and maps index/pack references
// into contextcompose.Candidates. It never calls ApplyUpdate, never rebuilds
// indexes onto disk, and never reads arbitrary project source-file contents.
package cesource
