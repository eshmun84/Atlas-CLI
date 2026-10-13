// Package memorypersist is the governed write path for automatic Memory
// persistence (Slice 36C).
//
// Pipeline:
//
//	structured Candidate (agent/internal transport)
//	  → config gate (persistence mode)
//	  → policy.Evaluate (pure quality/safety gates)
//	  → memory.Store.SaveObservation (existing SQLite write API)
//
// Boundaries:
//   - Context compose / memoryretrieve remain read-only.
//   - CodeGraph and Context Economy remain read-only.
//   - Adapters and hooks must not open SQLite directly for writes.
//   - Persistence failures never panic; callers receive structured results.
package memorypersist
