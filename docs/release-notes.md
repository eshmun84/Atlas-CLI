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
- SDD governance + current OpenSpec integration: operational contract at `.atlas/contracts/sdd-openspec.md`; layout/bootstrap observation; change discovery; agents may create OpenSpec-compatible change artifacts. **OpenSpec CLI is probed on PATH only today and is currently optional for Atlas artifact governance** — real CLI/provider hardening is the next block (Block B), not a shipped claim here
- Skills v1: local Home catalog, exact pins, deterministic digests, metadata resolver, progressive loading, Cursor/OpenCode projections, ownership/conflict handling, Repair integration (no marketplace UI)
- Memory Core, governed retrieval, governed persistence, Memory diagnostics, Memory Skill, Context Composition integration
- Context Economy **v0**: file-based, explicit Update (index / capsule / pack under Atlas Home)
- Code Intelligence with optional externally installed **CodeGraph** provider (`internal/codeintel`; not MCP; may be unavailable)
- MCP foundation: desired state in Atlas config + **Atlas-owned projections** to Cursor/OpenCode (no secret storage; no live auth/connection verification)
- Neutral Hooks engine + Cursor native hooks + OpenCode native hooks. Production hooks **action registry has no automatic Memory injection / side-effect actions** yet
- Source-control policy: local Git bootstrap when required, signing policy visibility, local-only `.gitignore` Atlas block management
- Slice 3 diagnostics: Doctor repairability (`atlas-repair` / `confirm-repair` / `diagnostic-only` / `manual`); config-aware Git; Status compact readiness; A–N regression matrix
- Status and Doctor are **read-only**
- Runtime Repair is the **explicit** runtime mutation path (Review → Apply)
- Configure Apply saves `.atlas/config.yaml` and reconciles Atlas-owned MCP projections; does not silently rematerialize non-MCP runtime
- `atlas --version` as the only normal console output
- Validation: `make check`, `make smoke-mvp`, Slice 3 A–N tests

### Not included / deferred / removed

- Final V1 / production-ready claims
- Atlas Context Graph engine (compatibility preference may persist; **no** Init/Configure checkbox; Memory/CE/Composition/CodeIntel/CodeGraph are **not** a completed Context Graph)
- OpenSpec CLI execution and provider hardening (Block B target: required compatible provider; missing/incompatible blocks governed SDD; no emulation/fallback)
- Atlas Native Spec Engine (deferred); additional Spec Engines (future)
- Native automatic Memory injection (deferred); embeddings (planned/deferred); daemon/background service (deferred architectural option)
- Config History as durable audit UX (deferred — current config/state/ownership/backups/tx/locks/rollback suffice)
- Skills marketplace / Framework Skill Acquisition / Skill Creator (planned/future; not shipped)
- Task Capability Discovery (Block C; resolves DOGFOOD-015) — `atlas capability plan`; technology signals remain evidence helpers, not permanent stack→skill configuration
- Generic provider plugin platform (**removed** from current V1 plan; keep domain boundaries only)
- Atlas Core repository-release publisher (**removed**); Core does not automate remote create / push / PR / merge / tag / repo release publish
- Claude / Codex activation
- MCP secret manager / OAuth client / live connection verification
- Daily workflow commands (`atlas start`, `atlas change`, `atlas mcp`)
- Tag / GitHub release / publish of Atlas without explicit human approval

### Spec Engine direction (documentation)

| | Current | Next (Block B) |
| --- | --- | --- |
| Supported engine | OpenSpec only (V1 config) | OpenSpec only |
| CLI | PATH probe; currently optional | Required compatible OpenSpec CLI/provider for governed SDD |
| `openspec/` | Atlas may bootstrap empty dirs; inspects layout | Lifecycle/semantics belong to OpenSpec |
| Atlas role | SDD governance, agents, authority | Unchanged — not provider FS owner |

### Upgrade / migration notes

- Fresh projects: run `atlas init` and Apply config (optionally set `ATLAS_HOME` first).
- Existing initialized projects: Configure for config/MCP edits; Runtime Repair for runtime drift; Context Economy for capsule/pack refresh.
- Re-init when Home already holds project-scoped data requires explicit Home reset acceptance; reset deletes only `$ATLAS_HOME/projects/<project-id>/`.
- Same project name under different roots uses distinct Home project IDs.
- Status / Doctor / discovery / startup never create Atlas Home or mutate the workspace.

### Known limitations

- Dogfooding baseline, not final V1 / not production-ready
- Contract file is not an OpenSpec runner; CLI provider hardening pending
- Context Economy is deterministic file-based v0 (no embeddings, no Atlas Context Graph engine)
- Hooks plumbing exists; automatic Memory injection actions are not registered in production
- No Claude/Codex activation
- No Core Git remote/PR/merge/tag/release automation beyond Init local bootstrap
- Tests and smoke must use temporary `ATLAS_HOME` to avoid touching `~/.atlas`

### Validation

```bash
make check
make smoke-mvp
go test ./internal/runtime/ -run Slice3
```

Manual dogfooding checklist: see [release-readiness.md](release-readiness.md), `scripts/smoke-mvp.sh`, and `go run ./scripts/manualalpha`.
