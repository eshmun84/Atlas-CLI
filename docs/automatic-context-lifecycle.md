# Automatic Context Lifecycle

Approved architectural direction: Atlas administers **Context Economy** and
**Code Intelligence** during AI-assisted development without habitual developer
intervention, while keeping Status/Doctor read-only, Home-only storage, no
daemon, no mandatory MCP, and no new public CLI commands.

## Status legend

| Label | Meaning |
| --- | --- |
| **IMPLEMENTED** | Shipped and usable as described |
| **PARTIAL** | Infrastructure present; production behavior incomplete |
| **DEFERRED** | Explicitly out of scope for the current phase |

## Phase A — Internal maintenance Actions (**IMPLEMENTED**)

Package: `internal/hookactions`.

| Action ID | Responsibility |
| --- | --- |
| `context.economy.maintain` | Missing/stale → `ApplyUpdate`; fresh → no-op; unreadable → controlled failure |
| `codeintel.refresh.maintain` | Unavailable/incompatible → skip; missing → initial; stale → incremental; fresh → no-op |

## Phase B — Session start automation (**IMPLEMENTED**)

### Trigger gate

`hookactions.TriggersEnabled` defaults to **true**.

### Events

| Event | Production adapters | Behavior |
| --- | --- | --- |
| `session.start` | Cursor `sessionStart`; OpenCode `session.created` | **Primary** — admit + maintain |
| `task.start` (Cursor only, conditional) | Cursor `beforeSubmitPrompt` when the conversation has not yet been successfully admitted | **Fallback (B.8)** — admit + maintain once per session |
| `session.resume` | **Not supported** — neither Cursor nor OpenCode bridge declares/emits a native resume event. Actions do **not** subscribe. Canonical `hooks.EventSessionResume` remains for future mapping only. | No automation |

Not automated on session path: OpenCode `session.prompt` / non-Cursor `task.start`, Status, Doctor, Configure, Repair, Context Compose.
Post-code tool events are Phase **C.2c** (below).

**Honest contract:** do not invent provider events. Canonical translation stays
`sessionStart → session.start` and `beforeSubmitPrompt → task.start`. Maintenance
Actions still subscribe **only** to `session.start`. The Cursor fallback is an
engine-internal evaluation after Action dispatch — it does not redefine
`task.start` and does not alter governance or deny semantics.

### Cursor sessionStart gap (B.8)

Some Cursor Agent conversations emit `beforeSubmitPrompt` without a prior
`sessionStart`. Without a fallback, Context Economy / Code Intelligence never
run for those sessions.

Phase B.8 behavior:

| Scenario | Result |
| --- | --- |
| `sessionStart` then `beforeSubmitPrompt` | One admission (session fence) |
| Only `beforeSubmitPrompt` | Fallback admits once |
| Further prompts, same `conversation_id` / `session_id` | No re-admit |
| New session id | New admission allowed |
| Empty / invalid session id | Fail-open — no invent, no admit |
| Spawn failure / worker timeout / cancel / incomplete | Session fence cleared — controlled retry |
| Worker finished with CE/CI operational errors | Session marked complete (no per-prompt respawn) |
| Fresh project (CE + CI already current) | Worker no-op both services |

Session identity priority: native `session_id` / `conversation_id` on the
payload (never `generation_id`, PID, timestamp, or project path).

### Safe non-blocking lifecycle

Adapter hook subprocesses are short-lived (OpenCode child budget = **10s**).
`hooks.ModeNonBlocking` does **not** mean fire-and-forget: Dispatch still waits
up to the Action timeout.

Phase B / B.2 / B.8 therefore:

1. **Admission (hook process, ≤3s):** resolve project → in-process event dedupe →
   per-session fence → acquire `maintain.lock` → write `maintain.lease`
   (`admitted`) → mark session `admitted` → spawn detached worker → release
   lock → return `ok` / `skipped`.
2. **Worker (detached process, ≤3m):** acquire flock → lease `running` → write
   `maintain-last.yaml` start → Context Economy then Code Intelligence → write
   `maintain-last.yaml` complete → finalize session (`complete` or clear on
   failure) → clear lease → exit. Survives hook process exit via `Setsid` /
   detached process group.
3. **Cursor fallback:** `hooksengine.Engine.Dispatch` calls
   `hookactions.MaybeAdmitCursorSessionFallback` **after** Action dispatch.
   Cursor-only; bounded by `AdmissionTimeout`; never changes the dispatch
   response.
4. Internal entry: env `ATLAS_INTERNAL_MAINTAIN=1` (not a public CLI verb);
   optional `ATLAS_INTERNAL_MAINTAIN_SESSION` carries the session fence id.

### Concurrency (B.2 + B.8)

| Mechanism | Scope |
| --- | --- |
| `inProcessAdmit` map | Same Dispatch (CE + CodeIntel Actions share one spawn) |
| `$ATLAS_HOME/projects/<id>/state/maintain.lock` flock | Cross-process exclusive work |
| `$ATLAS_HOME/projects/<id>/state/maintain.lease` | Closes release-before-spawn TOCTOU (fresh `admitted` → skip) |
| `$ATLAS_HOME/projects/<id>/state/session-maintain.yaml` | Per-session admit/complete fence (B.8); TTL prune |
| Existing `mutatelock` | Inside ApplyUpdate / Refresh |

**States (do not conflate):**

| State | Meaning |
| --- | --- |
| Session admitted | Spawn succeeded; further admits for that session suppressed while TTL live |
| Worker running | Holds `maintain.lock`; lease `running` |
| Worker complete | `maintain-last` complete + session `complete` |
| Worker timeout / cancel / incomplete | Session fence cleared — retry allowed |
| Worker finished (incl. CE/CI operational errors) | Session marked `complete` — no per-prompt respawn |

Admission OK is **not** the same as maintenance completed.

### Observability (B.2 + B.8)

`$ATLAS_HOME/projects/<id>/state/maintain-last.yaml` records worker start/finish,
duration, CE/CI outcomes (`ok|skipped|failed|timeout|cancelled|unavailable`),
optional `session_id`, and a non-sensitive `error_code`. Status `incomplete`
marks a prior run that never finished (e.g. SIGKILL) when the next worker
starts and clears that session fence.

`$ATLAS_HOME/projects/<id>/state/session-maintain.yaml` maps session id →
`admitted|complete` with timestamps. Entries expire (`SessionRecordTTL` = 24h;
`SessionAdmittedTTL` = worker budget + grace). Cap: 256 sessions per project.

Observability write failures never block the agent session.

### Fail-open

- Not initialized / missing project context → Action `skipped`.
- Lock busy / active admission lease / session already admitted|complete → `skipped`.
- Missing/invalid Cursor session id on fallback → skip (no invent).
- Provider unavailable / incompatible → CodeIntel skip inside worker.
- CE unreadable / refresh errors → worker continues; session fence cleared for retry.
- Spawn failure → Action/fallback `failed` but **non_blocking** + `fail_continue`; fence cleared.
- Public hooks JSON messages remain fixed (`action ok` / `action skipped` / …).
- Fallback never converts an allowed Cursor event into a denial.

### Performance

- Admission path avoids fingerprint/rebuild in the hook process.
- Worker is detached; prompt path is not blocked on CE/CI work.
- Per-prompt cost after the first admit is a cheap session-fence read under lock contention only when racing.

### Still manual

- TUI Context Economy Update / Code Intelligence Refresh (override / force full).
- Init does **not** seed CE or CodeGraph (Phase C candidate — deferred).

### Limitations

- OpenCode has no `beforeSubmitPrompt` fallback; it relies on `session.created`.
- Mid-session resume without a new session id does not re-trigger once `complete`.
- Cursor conversations that omit both `sessionStart` and a valid
  `conversation_id`/`session_id` on prompts cannot be maintained automatically.

## Ownership

| Data | Owner | Location |
| --- | --- | --- |
| CE index/capsule/pack | Atlas | `$ATLAS_HOME/projects/<id>/context/` |
| CodeGraph DB + metadata | Atlas | `$ATLAS_HOME/projects/<id>/codegraph/` |
| Maintain lock / lease / last / session fence | Atlas | `$ATLAS_HOME/projects/<id>/state/` |
| Maintain pending / pending.lock / post-code owner | Atlas | `$ATLAS_HOME/projects/<id>/state/` |
| Product source tree | Developer | untouched |
| Status / Doctor | Atlas read-only | never call maintenance Actions |

## Phase C.2b — Post-code infrastructure (**IMPLEMENTED**)

Home-only queue + finite detached scheduler (no daemon):

| Artifact | Role |
| --- | --- |
| `maintain-pending.yaml` | Generations, due_at, owner token/fence/lease |
| `pending.lock` | Serialize pending mutations |
| Post-code scheduler | Debounce → CE then CI → commit gen0; reclaim on lease expiry |

Session fence (`session-maintain.yaml`) is **not** used for post-code work.
`session.start` admission best-effort calls `EnsurePostCodeSchedulerIfDirty`
(recovery only; does not change Phase B outcomes).

## Phase C.2c — Automatic post-code trigger (**IMPLEMENTED** — simulated/integration)

### Events

| Adapter | Native event | Canonical |
| --- | --- | --- |
| Cursor | `postToolUse` | `tool.after_execution` |
| OpenCode | `tool.execute.after` | `tool.after_execution` |

Action: `context.postcode.maintain` (`ModeNonBlocking`, `fail_continue`,
`AdmissionTimeout`). Flow: allowlist → resolve project →
`BumpPendingAndEnsureScheduler` → return. **Never waits for CE/CI.**

### Allowlist (closed, exact match, per adapter)

| Adapter | Allowed `tool_name` | Evidence |
| --- | --- | --- |
| Cursor | `Write`, `Delete` | Cursor hooks docs matchers; Claude `Edit` → Cursor `Write` |
| OpenCode | `write`, `edit`, `apply_patch` | [opencode.ai/docs/tools](https://opencode.ai/docs/tools/) |

**Excluded (non-exhaustive):** `Shell` / `bash`, reads (`Read`/`read`,
`Grep`/`grep`, …), MCP tools, Task, unknown names, error/failed/denied/
cancelled status, empty `tool_name`, triggers disabled.

No argv/command regex. Case-sensitive names (Cursor PascalCase ≠ OpenCode
lowercase). Cross-adapter name reuse is not accepted.

### Possible mutation semantics

An allowed event is a **possible** filesystem mutation signal, not a confirmed
content change. CE/CI fingerprints remain the freshness authority; gen0 commit
rules from C.2b still apply when edits arrive during maintain.

### Recovery

- Dirty pending + no vigente owner → `EnsurePostCodeSchedulerIfDirty` (tool path
  or `session.start` defer).
- Spawn failure → Action `failed` (fail-open to agent); ownership cleared; work
  remains dirty for later recovery.
- Corrupt pending → quarantine + dirty epoch (FIX-01/04); fail-closed for Atlas
  state mutations.

### Observability

Same Home artifacts as C.2b (`maintain-pending.yaml`, `maintain-last.yaml` when
the scheduler runs CE/CI). No tool bodies, prompts, file contents, or secrets
are stored from the hook payload.

### Adapter validation status

| Check | Cursor | OpenCode |
| --- | --- | --- |
| Native → canonical projection (unit) | **PASS** | **PASS** |
| Action → pending (integration/simulated) | **PASS** | **PASS** |
| Physical agent E2E (real Cursor/OpenCode session) | **NOT VERIFIED** | **NOT VERIFIED** |

### Limitations (C.2c)

- Shell/bash-driven edits are intentionally ignored (no command reinterpretation).
- Cursor `postToolUseFailure` is not projected today; failed tools do not bump.
- Tab/file-edit-specific Cursor events (`afterFileEdit` / `TabWrite`) are not
  wired; only `postToolUse` → `tool.after_execution`.
- Physical E2E with live agents is out of this slice’s automated evidence.

## Phase C — Still deferred

Init-time CE/CodeGraph seed; native `session.resume` adapter mapping;
Status/Doctor automation.

## Explicitly out of scope

Daemon/watchers, embeddings, Atlas Context Graph engine, MCP for CodeGraph,
Memory automatic injection Actions, new public CLI verbs, TUI redesign for this
capability.
