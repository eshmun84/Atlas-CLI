# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI launches a full-screen interactive TUI. Product work happens in that shell, not as console report dumps.

## Alpha 2 status (`0.1.0` — release candidate)

Practical **Alpha 2** release candidate: installable locally, validated by `make check` + `make smoke-mvp`, usable with Cursor and OpenCode.

Alpha 2 is **not** final V1. Do not tag, create a GitHub release, or publish without explicit human approval.

### What Atlas does today

- TUI-first shell: Status (default landing), Init, Configure, Doctor, Runtime Repair, Context Economy, Help
- Init / Configure with persistence under `.atlas/`
- Atlas Home (`ATLAS_HOME` or `~/.atlas`) with mirrored bundled assets and project-scoped `projects/<id>/` storage
- Runtime materialization: composed `AGENTS.md`, **Cursor and OpenCode** projections, **14 Atlas agents per selected runtime**, registry/manifest/lock, SDD/OpenSpec operational contract
- Status + Doctor runtime awareness (**read-only**)
- Explicit **Runtime Repair** (Review → Apply) — restores Atlas-owned runtime artifacts only; preserves developer MCP configs, non-Atlas settings/rules, and developer agents
- Explicit **Context Economy v0** update (index / capsule / pack under Atlas Home project storage; file-based)
- Configure Apply saves `.atlas/config.yaml` and reconciles Atlas-owned MCP projections (secrets never stored; env/header refs + auth modes only)
- Optional **Code Intelligence** via externally installed CodeGraph (`internal/codeintel`) — Status/Doctor read-only; explicit refresh; no CodeGraph MCP
- Init Home reset gate when project-scoped Home data already exists
- `atlas --version` as the only normal console output

### What Atlas does **not** do yet

- Not final V1
- No Atlas Context Graph implementation (preference flag only; separate from Code Intelligence)
- No marketplace / community / remote asset registry
- No Skills v1
- No real OpenSpec CLI execution or live specs/tasks automation
- No Claude / Codex activation
- No MCP secret storage, OAuth client, or live connection verification (projections use references / agent-managed auth)
- No Git commit/push/automation
- No hooks, embeddings, daemon, or background service
- No daily workflow commands (`atlas start`, `atlas change`, `atlas mcp`)
- No git tag / GitHub release / publish without explicit human approval

## Install / local build

Requirements: **Go 1.27.1** (matches `go.mod`; use a toolchain that satisfies that directive).

```bash
make build                    # writes bin/atlas
./bin/atlas --version

# Optional local install (defaults to ~/.local/bin when PREFIX unset)
make install                  # PREFIX=$HOME/.local
# or temporary:
make install PREFIX=/tmp/atlas-prefix
```

For validation without touching your real Atlas Home:

```bash
export ATLAS_HOME=/tmp/atlas-home-test
make build
./bin/atlas --version
```

## Run

```bash
atlas                 # TUI → Status (default landing)
atlas init            # TUI → Init / Setup
atlas status          # TUI → Status (read-only)
atlas doctor          # TUI → Doctor (read-only)
atlas --version       # console: version only
```

Unsupported commands such as `atlas start`, `atlas change`, and `atlas mcp` open an Error dialog inside the TUI — they are not real commands and exit non-zero after the dialog closes.

## Recommended Alpha 2 flow (Cursor + OpenCode)

1. `export ATLAS_HOME=...` if you want an isolated Home (recommended for trials).
2. From a project directory: `atlas init` → select **Cursor** and/or **OpenCode** → Review → **Apply config**.
3. Confirm materialization (below).
4. Open the project in Cursor / OpenCode; agents load from the materialized runtime paths.
5. In Atlas TUI: **Context Economy** → Review → Apply Update when you want a fresh capsule/pack.
6. Use **Status** / **Doctor** for read-only health (including optional Code Intelligence when CodeGraph is installed).
7. Use **Runtime Repair** only when Atlas-owned runtime files drift or go missing (never to rewrite developer MCP).
8. Use **Configure** for config/MCP desired-state edits (Apply reconciles Atlas-owned MCP projections).

## What Init Apply materializes

Project-local (under the product repo):

| Path | Role |
| --- | --- |
| `.atlas/config.yaml`, `local.yaml`, `state.yaml` | Config + state |
| `.atlas/assets.lock.yaml` | Installed asset checksums + Home path |
| `.atlas/agent-registry.md` | Human-readable agent registry |
| `.atlas/runtime-manifest.yaml` | Machine-readable runtime agent map |
| `.atlas/contracts/sdd-openspec.md` | SDD/OpenSpec **operational contract** (not a live OpenSpec runner) |
| `AGENTS.md` | Composed contract (`ATLAS:BASE` + adapters + preserved `ATLAS:USER`) |
| `.cursor/rules/atlas.mdc` | Cursor entrypoint (when Cursor selected) |
| `.opencode/atlas.md` | OpenCode entrypoint (when OpenCode selected) |
| `.cursor/agents/atlas-*.md` | 14 Atlas agents (when Cursor selected) |
| `.opencode/agents/atlas-*.md` | 14 Atlas agents (when OpenCode selected) |
| `.cursor/mcp.json` / `opencode.json` | Atlas-owned MCP entries merged when selected (developer entries preserved). `opencode.jsonc` is **not** merged in Slice 32 — OpenCode MCP reconcile blocks if JSONC is present |

Atlas Home (under `$ATLAS_HOME` / `~/.atlas`):

- Mirrored assets (`assets/…`), agents, adapters, contracts
- Project-scoped local data under `projects/<project-id>/` (identity from canonical root; same name + different root → different ID)
- Context Economy payloads under `projects/<project-id>/context/` after an explicit Update
- Runtime Repair backups under `projects/<project-id>/backups/<timestamp>/`
- MCP ownership + backups under `projects/<project-id>/mcp/`
- Optional CodeGraph data under `projects/<project-id>/codegraph/`

## What Status / Doctor validate

Read-only checks for config/state, `AGENTS.md` markers, selected adapters/projections, Atlas agent health, registry/manifest/lock, SDD contract presence/match, Atlas Home visibility, Context Economy state, MCP projection health, optional Code Intelligence health, backups, and basic tool/git presence. They never repair, rematerialize, or create Atlas Home.

## Runtime Repair

Explicit Review → Apply. Recomputes the plan before Apply. Restores missing/broken **Atlas-owned** runtime files (with Home-backed backup before replace/quarantine of Atlas-owned paths). Never moves or deletes developer/external surfaces such as `CLAUDE.md`, `GEMINI.md`, `AGENT.md`, `.agents/`, or `.claude/` — Status/Doctor may report coexistence only. Preserves `.cursor/mcp.json`, `opencode.json`, non-Atlas rules/settings, `README.md`, `.gitignore`, and non-Atlas agents. Does **not** delete Context Economy payloads under Atlas Home. Does **not** reconcile MCP (use Configure Apply).

## Configure

Configure Apply saves `.atlas/config.yaml` (and may create optional `docs/atlas/README.md` once when selected on Init and still missing). Does **not** rematerialize general runtime assets (`AGENTS.md`, rules, agents). **Does** reconcile Atlas-owned MCP projections when MCP/adapters change. Custom remote MCP supports auth modes: none, environment header (`env` + optional prefix such as `Bearer `), external OAuth, provider managed — references only, never raw secrets.

## Context Economy (v0)

Explicit Review → Apply Update. Writes `index.yaml`, `capsule.md`, and packs under Atlas Home `projects/<id>/context/`, plus minimal transitional refs in `.atlas/state.yaml`. Status/Doctor/Repair/discovery/startup do not create or refresh context payloads. This is **not** an Atlas Context Graph engine.

## Code Intelligence

Optional. CodeGraph is an externally installed provider behind `internal/codeintel`. See [docs/code-intelligence/](docs/code-intelligence/README.md). No CodeGraph MCP. Atlas Context Graph remains deferred.

## Known Alpha 2 limitations

- Not final V1
- OpenSpec contract is operational guidance only — no OpenSpec CLI execution
- Cursor + OpenCode only; Claude/Codex not activated
- No Atlas Context Graph implementation
- No marketplace, Skills v1, MCP secret manager / live connection verification, Git automation, hooks, embeddings, or daemon
- Configure Apply updates `.atlas/config.yaml` and MCP projections; use Runtime Repair to rematerialize non-MCP runtime files
- OpenCode MCP: Atlas merges only `opencode.json`. If `opencode.jsonc` exists, OpenCode MCP reconciliation is blocked (no parallel `opencode.json` creation, no ownership mutation) until JSONC-safe merging exists
- No git tag / GitHub release / publish in this RC unless a human explicitly approves later

## Validation

```bash
make check          # fmt + vet + unit tests
make smoke-mvp      # Alpha 2 release-readiness smoke
```

`make smoke-mvp` runs `scripts/smoke-mvp.sh`: local build, temp `PREFIX` install, `atlas --version`, CLI routing, temp-workspace matrix under isolated `ATLAS_HOME`, focused package tests, and repo-root cleanliness.

See also:

- [docs/tui-ux-contract.md](docs/tui-ux-contract.md)
- [docs/release-notes.md](docs/release-notes.md)
- [docs/release-readiness.md](docs/release-readiness.md)
- [docs/code-intelligence/](docs/code-intelligence/README.md)

## Project layout

```
cmd/atlas/           # process entrypoint
internal/app/        # application wiring
internal/cli/        # arg parse + TUI launcher
internal/tui/        # sidebar shell (Bubble Tea + Lip Gloss)
internal/config/     # schema, drafts, persistence, Init/Configure apply
internal/mcp/        # Atlas-owned MCP desired state, projectors, ownership, reconcile
internal/inspect/    # composed project inspection (canonical discovery)
internal/project/    # filesystem/git/tools snapshot + fsafety
internal/runtime/    # runtime health + Runtime Repair
internal/codeintel/  # Code Intelligence provider boundary + CodeGraph adapter
internal/home/       # Atlas Home resolve/inspect/mirror
internal/context/    # Context Economy (index/capsule/pack)
internal/doctor/     # diagnostics model
internal/initplan/   # init dry-run planning
internal/assets/     # embedded agents, adapters, contracts
internal/version/    # version string (0.1.0 Alpha 2 RC)
scripts/             # release smoke helpers
docs/                # release notes, readiness, code intelligence
```
