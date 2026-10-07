# Atlas CLI release notes

## 0.1.0 — Alpha release candidate

First practical Alpha RC: installable locally, Home-backed assets, Cursor/OpenCode agent pack, SDD/OpenSpec operational contract, and Context Economy v0. No git tag, GitHub release, or publish without explicit human approval.

### Included

- TUI-first launcher: Status (default landing), Init / Setup, Configure, Doctor, Runtime Repair, Context Economy, Help
- Config persistence under `.atlas/` (`config.yaml`, `local.yaml`, `state.yaml`, `assets.lock.yaml`, `agent-registry.md`, `runtime-manifest.yaml`, `contracts/`, `backups/`)
- Atlas Home (`ATLAS_HOME` or `~/.atlas`) created/updated only by mutating flows (Init Apply, Runtime Repair Apply, Context Update Apply)
- Runtime gateway: composed `AGENTS.md`; Cursor/OpenCode projections; **14 Atlas agents per selected runtime**
- SDD/OpenSpec operational contract materialized at `.atlas/contracts/sdd-openspec.md` (guidance only; no OpenSpec CLI)
- Read-only Status and Doctor with Home + Context Economy awareness
- Explicit Runtime Repair (Review → Apply) with mandatory backup/quarantine; preserves developer-owned files; does not wipe Context Economy payloads
- Explicit Context Economy update (index / capsule / pack under Atlas Home)
- Local install helper: `make install` → `$PREFIX/bin/atlas` (default `~/.local`)
- `atlas --version` as the only normal console output
- Alpha smoke: `make smoke-mvp` (build, temp install, Home isolation, init matrix, repair, context, cleanliness)

### Not included / deferred

- Marketplace / remote skills-agents registry
- Real OpenSpec CLI integration / live specs automation
- Claude / Codex active adapters
- Memory SQLite engines beyond file-based Context Economy
- MCP connections or credentials
- Git / Jira / GitHub automation
- Hooks, embeddings, daemon / background service
- Daily workflow commands (`atlas start`, `atlas change`, …)
- Broader adapter surfaces beyond Cursor and OpenCode
- Context Graph engine (preference flag only)

### Upgrade / migration notes

- Fresh projects: run `atlas init` and Apply config (optionally set `ATLAS_HOME` first).
- Existing initialized projects: Configure for config edits; Runtime Repair for runtime drift; Context Economy for capsule/pack refresh.
- Configure Apply still writes `.atlas/config.yaml` only and does not rematerialize runtime files.
- Status / Doctor / discovery / startup never create Atlas Home or mutate the workspace.

### Known Alpha limitations

- Contract file is not an OpenSpec runner
- No publish/tag/release automation in-tree
- Context Economy is deterministic file-based v0 (no embeddings)
- Tests and smoke must use temporary `ATLAS_HOME` to avoid touching `~/.atlas`

### Validation

```bash
make check
make smoke-mvp
```

Manual Alpha smoke checklist: see [release-readiness.md](release-readiness.md), `scripts/smoke-mvp.sh`, and `go run ./scripts/manualalpha`.
