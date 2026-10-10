# Atlas Alpha 2 release-readiness checklist

Version under test: **0.1.0** (Alpha 2 release candidate).
Toolchain: **Go 1.27.1** (required by `go.mod`; local/CI builds must satisfy that directive).
Alpha 2 is **not** final V1.
No git tag / GitHub release / publish unless a human explicitly approves later.

## Automated gate

- [x] `gofmt -l .` (clean)
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] `make check`
- [x] `make smoke-mvp`
- [x] `git diff --check`

`make smoke-mvp` must cover:

- [x] build
- [x] local install to temporary `PREFIX`
- [x] `atlas --version`
- [x] unsupported commands exit non-zero and do not mutate
- [x] TUI-first routing
- [x] empty directory (fresh non-Atlas)
- [x] Git + README existing project
- [x] initialized Atlas project
- [x] Cursor-only init
- [x] OpenCode-only init
- [x] Cursor + OpenCode init
- [x] Status read-only
- [x] Doctor read-only
- [x] Runtime Repair
- [x] Context Economy update / read-only / stale detection
- [x] Configure Apply (config.yaml + MCP reconcile; no silent non-MCP rematerialize)
- [x] Init Home reset gate
- [x] `ATLAS_HOME` project isolation (default `~/.atlas` untouched)
- [x] same-name different-root isolation
- [x] no generated runtime artifacts in Atlas repo root

## Manual smoke (temporary Home + PREFIX)

Library-equivalent (non-interactive) helper:

```bash
export ATLAS_HOME=/tmp/atlas-home-alpha2-rc
rm -rf "$ATLAS_HOME"
PROJECT=/tmp/atlas-alpha2-project
rm -rf "$PROJECT" && mkdir -p "$PROJECT"
make build
make install PREFIX=/tmp/atlas-prefix-alpha2-rc
ATLAS_SMOKE_MANUAL_ROOT="$PROJECT" ATLAS_HOME="$ATLAS_HOME" go run ./scripts/manualalpha
```

Interactive TUI (optional confirmation):

```bash
export ATLAS_HOME=/tmp/atlas-home-alpha2-rc
cd /tmp/atlas-alpha2-project
atlas   # or ./bin/atlas from the Atlas repo
```

Checklist (library-equivalent via `scripts/manualalpha`):

- [x] Init with Cursor + OpenCode → Apply
- [x] Verify `AGENTS.md`, `.cursor/rules/atlas.mdc`, `.opencode/atlas.md`
- [x] Verify 14 agents under `.cursor/agents/` and `.opencode/agents/`
- [x] Verify `.atlas/agent-registry.md`, `runtime-manifest.yaml`, `assets.lock.yaml`
- [x] Verify `.atlas/contracts/sdd-openspec.md`
- [x] Context Economy → Update → `index.yaml`, `capsule.md`, pack under `$ATLAS_HOME/projects/<id>/context/…`
- [x] Status / Doctor → no mutation; Home not created by read-only paths when missing
- [x] Drift agent/contract → Runtime Repair restores Atlas files
- [x] Developer-owned files preserved; Context payload intact after repair
- [x] `~/.atlas` unchanged while `ATLAS_HOME` is set
- [x] Atlas repo root free of generated runtime artifacts

Interactive TUI click-through remains optional human confirmation.

## Guarantees (must hold)

- Status / Doctor / discovery / startup do not mutate
- Runtime Repair remains the explicit mutation path for non-MCP runtime artifacts (Review → Apply)
- Configure Apply saves config.yaml and reconciles Atlas-owned MCP projections; it does not silently rematerialize AGENTS/rules/agents
- Context update remains explicit
- Init Home reset requires explicit acceptance and deletes only `$ATLAS_HOME/projects/<project-id>/`
- Atlas Home is not created by read-only flows
- Tests/smoke use temporary `ATLAS_HOME`
- No credential writes
- No generated runtime artifacts left in the Atlas repo root
- No tag / release / publish without explicit human approval

## Honest capability boundaries

| Area | Alpha 2 RC stance |
| --- | --- |
| Materialized runtimes | Cursor + OpenCode only |
| Status / Doctor | Read-only |
| Runtime Repair | Explicit mutation path |
| Context Economy v0 | File-based, explicit update |
| CodeGraph | Optional Code Intelligence provider (externally installed; not MCP; may be unavailable) |
| Atlas Context Graph | Preference only; NOT IMPLEMENTED |
| MCP | Desired state + Atlas-owned projections; no auth/connection/verification; secrets not stored |
| OpenSpec | Operational contract only; no CLI execution |
| Git | Discovery only; no automation |
| Skills v1 | Local Home catalog + exact pins + Cursor/OpenCode projections (no marketplace) |
| Marketplace / community registry | Not implemented |
| Claude / Codex | Not activated |
| Tag / release / publish | Human approval required |

## Out of scope for this RC

Final V1, Atlas Context Graph engine, MCP authentication/connection/verification orchestration, secret manager, marketplace/community registry / remote skill install, real OpenSpec CLI, Claude/Codex activation, Git automation, hooks, embeddings, daemon, new CLI commands, tag/release/publish.
