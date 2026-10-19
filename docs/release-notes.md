# Atlas CLI release notes

## Current development / dogfooding baseline (`0.1.0` source string)

This document describes the **current product truth** on `develop` after Slice 1–3 closed. The source version string remains `0.1.0`; version selection, tagging, and publishing belong to a later hardening block. This is **not** final V1 and **not** a production-ready claim.

No git tag, GitHub release, or publish without explicit human approval.

### Included (implemented)

- TUI-first launcher: **Status** (default landing; Dashboard/Home screen removed), Init / Setup, Configure, Doctor, Runtime Repair, Context Economy, Code Intelligence refresh, Help
- Config persistence under `.atlas/` (`config.yaml`, `local.yaml`, `state.yaml`, `assets.lock.yaml`, `agent-registry.md`, `runtime-manifest.yaml`, `contracts/`)
- Atlas Home (`ATLAS_HOME` or `~/.atlas`) with project-scoped `projects/<project-id>/` isolation; Init requires a healthy Home and fails closed
- Runtime gateway: composed `AGENTS.md`; **Cursor and OpenCode** materialized runtimes; orchestrator + specialized SDD/review/worker agents
- Filesystem safety, mutation locking, brownfield backup into Atlas Home, transactional Apply with rollback
- SDD governance + **real OpenSpec provider**: operational contract at `.atlas/contracts/sdd-openspec.md`; Init Apply runs `openspec init`; exact project pin (`governance.openspec_version`); missing/incompatible/mismatched CLI blocks governed SDD; no Atlas emulation or mkdir-bootstrap; Runtime Repair refuses OpenSpec provider bootstrap
- Skills v1: local Home catalog, exact pins, deterministic digests, metadata resolver, progressive loading, Cursor/OpenCode projections, ownership/conflict handling, Repair integration (no marketplace UI)
- Task Capability Discovery (Block C; resolves DOGFOOD-015): `atlas capability plan`; bounded evidence; task-active skill selection; proactive governed retrieval planning; technology signals remain evidence helpers, not permanent stack→skill configuration
- Memory Core, governed retrieval, governed persistence, Memory diagnostics, Memory Skill, Context Composition integration; proactive path = capability plan → explicit retrieval → agent invokes Memory/context compose (native automatic injection deferred)
- Context Economy **v0**: file-based Home artifacts; TUI explicit Update; Phase B auto-maintain on `session.start` (detached worker); Cursor `beforeSubmitPrompt` fallback (B.8)
- Code Intelligence with optional externally installed **CodeGraph** provider (`internal/codeintel`; not MCP; may be unavailable); Phase B session auto-refresh when provider available; Cursor prompt fallback (B.8) — see [automatic-context-lifecycle.md](automatic-context-lifecycle.md)
- MCP foundation: Atlas Managed MCP V1 = **Context7 + Chrome DevTools** (opt-in); Atlas-owned projections to selected Cursor/OpenCode adapters (no secret storage; no live auth/connection verification; no Filesystem/GitHub/Jira/Custom MCP UI)
- Neutral Hooks engine + Cursor native hooks + OpenCode native hooks. Production registry includes CE/CodeIntel **maintenance** Actions (Phase B session admission + detached worker); **no** automatic Memory injection
- Source-control policy: local Git bootstrap when required, signing policy visibility, provider-neutral governance storage (`Keep local` / `Version with project`) with managed `.gitignore` upsert/cleanup
- ≥1 supported agent adapter required for Init/Configure Apply; evidence of verification is inherent (not user-editable)
- Slice 3 diagnostics: Doctor repairability (`atlas-repair` / `confirm-repair` / `diagnostic-only` / `manual`); config-aware Git; Status compact readiness; A–N regression matrix
- Block D / D6 polish: project-only Runtime Repair no longer silently remirrors Atlas Home; Hooks Doctor health; OpenSpec tool-projection diagnostics (Configure/Init, not Repair); one OpenSpec provider probe per Inspect; `delivery_assist` legacy/hidden; install PATH + safe uninstall/purge docs
- Pre-release configuration reconciliation (after D7): agent-required + available invariant; evidence inherent; governance storage provider-neutral for all Atlas-managed artifacts (including managed MCP); adapter add/remove/swap performs full reprojection from Atlas canonical state (not adapter-to-adapter); OpenCode Atlas MCP lives at `.opencode/opencode.json` so `local_only` can hide it without ignoring user root `opencode.json`
- Status and Doctor are **read-only**
- Runtime Repair remains for unrelated Atlas-owned drift (Review → Apply); Home remirror only when the reviewed plan includes an explicit Home target
- Configure Apply saves `.atlas/config.yaml` and fully reprojects Atlas-owned adapter runtime + MCP/Skills/Hooks when adapters/MCP change
- Configure OpenSpec tool sync/update participates in the Configure transaction (provider footprint snapshot + rollback on Apply failure where safely possible; not perfect external-process atomicity)
- Home/project `schema_version` compatibility gates mutation (fail closed on newer/unsupported older); product `atlas_version` is provenance only; no general migration/`atlas upgrade` command
- `atlas --version` as the only normal console output
- Validation: `make check`, `make smoke-mvp`, Slice 3 A–N tests

### Not included / deferred / removed

- Final V1 / production-ready claims
- Atlas Context Graph engine (compatibility preference may persist; **no** Init/Configure checkbox; Memory/CE/Composition/CodeIntel/CodeGraph are **not** a completed Context Graph)
- Atlas Native Spec Engine (deferred); additional Spec Engines (future)
- Native automatic Memory injection (deferred); embeddings (planned/deferred); daemon/background service (deferred architectural option)
- Config History as durable audit UX (deferred — current config/state/ownership/backups/tx/locks/rollback suffice)
- Skills marketplace / Framework Skill Acquisition / Skill Creator (planned/future; not shipped)
- Generic provider plugin platform (**removed** from current V1 plan; keep domain boundaries only)
- Atlas Core repository-release publisher (**removed**); Core does not automate remote create / push / PR / merge / tag / repo release publish
- Claude / Codex activation
- MCP secret manager / OAuth client / live connection verification / generic MCP registry
- Future `atlas-mcp-setup` skill (deferred; not stubbed)
- Daily workflow commands (`atlas start`, `atlas change`, `atlas mcp`)
- Tag / GitHub release / publish of Atlas without explicit human approval

### Spec Engine (OpenSpec — implemented)

| | Behavior |
| --- | --- |
| Supported engine | OpenSpec only (V1 config) |
| CLI | Required compatible OpenSpec CLI for governed SDD; exact project pin match |
| Init | Atlas Init Apply runs real `openspec init`; adopts pin when empty |
| Configure | May run `openspec init --tools` / `openspec update` for adapter projection sync; snapshots + rolls back captured provider footprint with Atlas surfaces on Apply failure |
| `openspec/` | Lifecycle/semantics belong to OpenSpec — no Atlas mkdir-bootstrap substitute |
| Repair | Does **not** bootstrap absent/legacy OpenSpec; Init owns remediation |
| Atlas role | SDD governance, agents, authority — not provider FS owner |

### Upgrade / migration notes

- Fresh projects: run `atlas init` and Apply config (optionally set `ATLAS_HOME` first).
- Existing initialized projects: Configure for config/MCP edits; Runtime Repair for runtime drift; Context Economy for capsule/pack refresh.
- Re-init when Home already holds project-scoped data requires explicit Home reset acceptance; reset deletes only `$ATLAS_HOME/projects/<project-id>/`.
- Same project name under different roots uses distinct Home project IDs.
- Status / Doctor / discovery / startup never create Atlas Home or mutate the workspace.

### Known limitations

- Dogfooding baseline, not final V1 / not production-ready
- OpenSpec provider required for governed SDD; contract is governance — not an OpenSpec emulator
- Context Economy is deterministic file-based v0 (no embeddings, no Atlas Context Graph engine)
- Hooks plumbing exists; automatic Memory injection actions are not registered in production
- No Claude/Codex activation
- No Core Git remote/PR/merge/tag/release automation beyond Init local bootstrap
- `delivery_assist` is legacy/deferred (not an editable Core capability)
- Install does not modify shell PATH; uninstall/purge of Home is docs-only and destructive when chosen
- Tests and smoke must use temporary `ATLAS_HOME` to avoid touching `~/.atlas`

### Validation

```bash
make check
make smoke-mvp
go test ./internal/runtime/ -run Slice3
```

Manual dogfooding checklist: see [release-readiness.md](release-readiness.md), `scripts/smoke-mvp.sh`, and `go run ./scripts/manualalpha`.
