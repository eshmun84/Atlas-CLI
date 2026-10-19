# Memory context-delivery capability (design)

Slice **36B.5** ships the canonical `memory` skill and CLI teaching.
Automatic context injection into agents is **not** enabled.

This document records the future delivery contract. It does **not** introduce
new Go interfaces or change `hooks.Result` for speculative compatibility.

## Capability levels

Per `(adapter, canonical_event)`:

| Level | Meaning |
|-------|---------|
| `unsupported` | Do not auto-retrieve for delivery on this event |
| `cli_skill` | Agent retrieves via `atlas memory … --json` + `memory` skill (default) |
| `native_inject` | Adapter can inject bounded Memory text into agent context |

Catalog membership of a hook event does **not** imply delivery support.

## Evaluation matrix (current evidence)

| Canonical event | Cursor native | Cursor delivery | OpenCode native | OpenCode delivery | 36B.5 level |
|-----------------|---------------|-----------------|-----------------|-------------------|-------------|
| `session.start` | `sessionStart` | Docs allow `additional_context`; IDE race can drop it; Atlas returns `{}` | `session.created` | Fire-and-forget; no inject | `cli_skill` (auto inject `unsupported`) |
| `task.start` | `beforeSubmitPrompt` | Docs: `continue` / `user_message` only — **no** agent context field | `session.prompt` | Prompt mutation persists as user text (risky); better candidate is `session.hook("context")` (not wired) | `cli_skill` |
| `context.before_compaction` | `preCompact` | `user_message` only (UI) | `session.compaction` | Summary path, not agent-loop inject | `cli_skill` |

## Cursor limitations

- Atlas `TranslateResponse` currently emits allow/deny/continue/`{}` only.
- `hooks.Result` has no context payload; Action messages are fixed public strings.
- Prompt/tool/transcript bodies are dropped before canonical translation (intentional).
- `TaskSummaryState` is `absent` on adapter-mapped `task.start` events.
- Documented `sessionStart.additional_context` is **not** proven reliable in Agent Window (Composer handle race). Do not enable auto Memory on that path without E2E proof that the **model** sees a canary.

## OpenCode context-hook candidate

OpenCode V2 plugins can register:

```ts
await ctx.session.hook("context", (event) => {
  event.system.push({ type: "text", text: "…" })
})
```

That mutates the outgoing model call (not persisted history). Atlas’s projected
plugin does **not** register this hook today. Prefer this over mutating
`session.prompt` text if automatic delivery is pursued later.

## Required E2E evidence before `native_inject`

1. Hook/plugin returns Memory canary text on the chosen event.
2. Agent model output cites the canary (not merely “merged” in a hooks log).
3. Missing DB / timeout / budget fail open — agent continues.
4. Project A Memory never appears under Project B.
5. Injected text is bounded and framed as observations (not executable policy).

Until those pass, production `hooksengine.DefaultRegistry()` must remain free of
Memory Actions that claim delivery. (Phase A may register Context Economy /
Code Intelligence *maintenance* Actions with triggers disabled; that does not
authorize Memory inject Actions.)

## Security and budget constraints

- Read-only retrieval only (`memory.OpenReadOnly` / CLI).
- Project isolation via existing resolve path.
- No transcript/prompt capture into Memory or hook payloads.
- No secrets in generated skill or agent instructions.
- Treat observation content as untrusted data (no instruction injection).
- No automatic Git.
- Any future native inject must reuse `memoryretrieve` budgets (and likely a
  stricter inject cap). Full Context Economy composition is Slice **36B.6**.
- Quality-governed automatic persistence is Slice **36C** (`atlas memory propose --json`).

## Default for 36B.5

All adapters: **`cli_skill`**. Automatic `native_inject` remains deferred.
