// Package hookactions provides Atlas-owned internal maintenance Actions for
// Context Economy and Code Intelligence.
//
// Phase B / B.2 (Automatic Context Lifecycle — session automation):
//   - Actions are registered in hooksengine.DefaultRegistry.
//   - TriggersEnabled defaults to true.
//   - On session.start, Action.Run admits work quickly and spawns a detached
//     worker process so adapter hook subprocesses (e.g. OpenCode's 10s child
//     budget) are not blocked by fingerprint/refresh cost.
//   - Cross-process: maintain.lock flock + maintain.lease fence (B.2) prevent
//     redundant workers after release-before-spawn.
//   - Worker results persist to maintain-last.yaml under Atlas Home (B.2).
//   - session.resume is not subscribed: Cursor/OpenCode have no native mapping.
//   - Status and Doctor remain read-only and never call these Actions.
//
// Phase B.8 (Cursor session maintenance fallback):
//   - Primary trigger remains session.start (Cursor sessionStart / OpenCode
//     session.created).
//   - When Cursor omits sessionStart, the first beforeSubmitPrompt (canonical
//     task.start) may admit maintenance if that conversation has not yet been
//     successfully admitted.
//   - Evaluation lives in hooksengine after Action dispatch via
//     MaybeAdmitCursorSessionFallback — Cursor-only, never changes task.start
//     Action subscriptions, governance, or deny semantics.
//   - Per-session fence: session-maintain.yaml under Atlas Home (admitted vs
//     complete; cleared on spawn/worker failure for controlled recovery).
//
// Phase C (deferred): post-code-change refresh, Init seed, native resume.
//
// Boundaries:
//   - Home-only payloads (no large context / graph.db in the product repo).
//   - No daemon, no MCP, no new public CLI commands, no package management.
//   - Non-blocking + fail-continue so maintenance never aborts agent work.
//   - Detached workers use ATLAS_INTERNAL_MAINTAIN (internal env, not a public verb).
package hookactions
