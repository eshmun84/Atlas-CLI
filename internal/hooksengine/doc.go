// Package hooksengine is the application-level Hooks execution layer (Slice 36B.2).
//
// Pipeline:
//
//	Canonical event JSON (untrusted)
//	  → validate schema_version==1 + project resolve
//	  → EventBridge lookup + Negotiate (adapters.ClassHooks ∧ declared events)
//	  → hooks.Dispatcher / hooks.Registry
//	  → structured sanitized JSON result
//
// It reuses the Slice 36B.1 foundation dispatcher and registry — it does not
// invent a second dispatch core. Canonical event catalog membership is never
// treated as adapter support; only EventBridge.DeclaredEvents participate in
// negotiation. Native Cursor/OpenCode bridges and ClassHooks enablement remain
// deferred. Missing bridges negotiate with an empty declaration set (fail closed).
//
// Boundaries: no daemon, network, MCP, Git, SQLite, arbitrary command
// execution, or implicit filesystem mutations.
package hooksengine
