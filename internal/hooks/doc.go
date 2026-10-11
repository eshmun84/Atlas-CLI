// Package hooks defines the agent-neutral Atlas Hooks foundation.
//
// Atlas owns the canonical event catalog, validation, capability negotiation,
// and in-process action dispatch. Agent adapters translate native runtime
// signals into these events; consumers (Memory, Governance, Testing, …)
// register Actions against event types without importing provider packages.
//
// Architectural boundaries for this package:
//   - No mandatory MCP, daemon, or network.
//   - No automatic Git operations.
//   - No adapter projections (Cursor hooks / OpenCode plugins) — those live
//     under internal/adapters/<provider>/ in later slices.
//   - Coarse adapter gating reuses adapters.ClassHooks; fine-grained event
//     support is declared per adapter via Negotiate.
//   - Future projection ownership/conflict must reuse skills/MCP ownership
//     patterns and project/fsafety containment — not reinvent path I/O here.
//   - Event payloads (including task_summary) are not persisted by this package.
//
// Cancellation: Dispatch cancels action contexts on timeout, but does not
// forcibly stop goroutines. See Dispatcher.Dispatch for guarantees.
//
// Slice 36B.1 ships contracts, validation, dispatch, and tests only.
package hooks
