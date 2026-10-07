# Atlas CLI release notes

## 0.1.0 — Alpha 2 release candidate

Alpha 2 RC for local validation. This is **not** final V1. No git tag, GitHub release, or publish without explicit human approval.

Baseline closes Slices 24–28 on `develop` (Status landing, Init Setup, Home project storage, Configure config-only, text-quality hardening), plus workspace Git fingerprint flake hardening for smoke stability.

### Included

- TUI-first launcher: Status (default landing), Init / Setup, Configure, Doctor, Runtime Repair, Context Economy, Help
- Config persistence under `.atlas/` (`config.yaml`, `local.yaml`, `state.yaml`, `assets.lock.yaml`, `agent-registry.md`, `runtime-manifest.yaml`, `contracts/`)
- Atlas Home (`ATLAS_HOME` or `~/.atlas`) created/updated only by mutating flows (Init Apply, Runtime Repair Apply, Context Update Apply)
- Project-scoped Home storage under `projects/<project-id>/` (identity from canonical root, not name alone)
- Runtime gateway: composed `AGENTS.md`; **Cursor and OpenCode** are the supported materialized runtimes; **14 Atlas agents per selected runtime**
- SDD/OpenSpec operational contract at `.atlas/contracts/sdd-openspec.md` (guidance only; no OpenSpec CLI execution)
- Status and Doctor are **read-only**
- Runtime Repair is the **explicit** runtime mutation path (Review → Apply), with Home-backed backup/quarantine; preserves developer-owned files; does not wipe Context Economy payloads
- Configure Apply is **config-only** (writes `.atlas/config.yaml`; optional Init-time docs scaffold); does not rematerialize runtime
- MCP selections are **preference/config only** (no materialization, auth, or verification)
- Context Economy **v0**: file-based, explicit Update (index / capsule / pack under Atlas Home)
- Local install helper: `make install` → `$PREFIX/bin/atlas` (default `~/.local`)
- `atlas --version` as the only normal console output
- Alpha smoke: `make smoke-mvp` (build, temp install, isolation, init matrix, Configure, Home reset gate, repair, context, cleanliness)

### Not included / deferred

- Final V1 product claims
- CodeGraph (not implemented)
- Atlas Context Graph engine (preference flag only; not implemented)
- Marketplace / community / remote skills-agents registry
- Skills v1
- Real OpenSpec CLI integration / live specs automation
- Claude / Codex activation (adapters not materialized)
- MCP materialization, auth, or verification
- Git commit/push/PR/tag automation
- Hooks, embeddings, daemon / background service
- Daily workflow commands (`atlas start`, `atlas change`, `atlas mcp`)
- Broader adapter surfaces beyond Cursor and OpenCode
- Tag / GitHub release / publish without explicit human approval

### Upgrade / migration notes

- Fresh projects: run `atlas init` and Apply config (optionally set `ATLAS_HOME` first).
- Existing initialized projects: Configure for config edits; Runtime Repair for runtime drift; Context Economy for capsule/pack refresh.
- Re-init when Home already holds project-scoped data requires explicit Home reset acceptance; reset deletes only `$ATLAS_HOME/projects/<project-id>/`.
- Same project name under different roots uses distinct Home project IDs.
- Status / Doctor / discovery / startup never create Atlas Home or mutate the workspace.

### Known Alpha limitations

- Alpha 2 is a release candidate, not final V1
- Contract file is not an OpenSpec runner
- Context Economy is deterministic file-based v0 (no embeddings, no CodeGraph, no Context Graph engine)
- MCP remains preference-only
- No Claude/Codex activation
- No Skills v1 / marketplace
- No Git automation
- No publish/tag/release automation in-tree
- Tests and smoke must use temporary `ATLAS_HOME` to avoid touching `~/.atlas`

### Validation

```bash
make check
make smoke-mvp
```

Manual Alpha 2 smoke checklist: see [release-readiness.md](release-readiness.md), `scripts/smoke-mvp.sh`, and `go run ./scripts/manualalpha`.
