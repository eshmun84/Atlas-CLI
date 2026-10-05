# Atlas CLI release notes

## 0.1.0 — MVP release candidate

First usable MVP for project-local Atlas setup. No Atlas Home. No new product systems beyond the hardened TUI shell and runtime gateway.

### Included

- TUI-first launcher: Dashboard, Init / Setup, Configure, Status, Doctor, Runtime Repair, Help
- Config persistence under `.atlas/` (`config.yaml`, `local.yaml`, `state.yaml`, `assets.lock.yaml`, `backups/`)
- Runtime gateway materialization: `AGENTS.md` always; Cursor/OpenCode adapter projections when selected
- Hardened `AGENTS.md` contract with managed/user marker sections
- Read-only Status and Doctor with runtime awareness
- Explicit Runtime Repair (Review → Apply) with mandatory backup/quarantine of conflicting runtime artifacts
- Context Graph preference flag only (no graph engine)
- `atlas --version` as the only normal console output

### Not included / deferred

- Atlas Home, skills/agents marketplace, assets registry
- Memory SQLite / context capsule file generation
- MCP connections or credentials
- Git / Jira / GitHub automation
- Daily workflow commands (`atlas start`, `atlas change`, …)
- Broader adapter surfaces beyond Cursor and OpenCode projections
- Context Graph engine

### Upgrade / migration notes

- Fresh projects: run `atlas init` and Apply config.
- Existing initialized projects: use Configure for config edits; use Runtime Repair to rematerialize or quarantine runtime drift.
- Configure Apply still writes `.atlas/config.yaml` only and does not rematerialize runtime files.

### Validation

```bash
make check
make smoke-mvp
```

Interactive TUI steps are listed by `scripts/smoke-mvp.sh` after automated checks pass.
