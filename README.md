# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI launches a full-screen interactive TUI. Product work happens in that shell, not as console report dumps.

## Alpha status (`0.1.0` — release candidate)

Practical Alpha RC: installable locally, validated by `make check` + `make smoke-mvp`, usable with Cursor and OpenCode.

### What Atlas does today

- TUI-first shell: Status (default landing), Init, Configure, Doctor, Runtime Repair, Context Economy, Help
- Init / Configure with persistence under `.atlas/`
- Atlas Home (`ATLAS_HOME` or `~/.atlas`) with mirrored bundled assets
- Runtime materialization: composed `AGENTS.md`, Cursor/OpenCode projections, **14 Atlas agents per selected runtime**, registry/manifest/lock, SDD/OpenSpec operational contract
- Status + Doctor runtime awareness (read-only)
- Explicit Runtime Repair (Review → Apply) with backup/quarantine of Atlas conflicts
- Explicit Context Economy update (index / capsule / pack under Atlas Home)
- `atlas --version` as the only normal console output

### What Atlas does **not** do yet

- No marketplace / remote asset registry
- No real OpenSpec CLI integration or live specs/tasks automation
- No Claude / Codex active adapters
- No MCP connections or credential storage
- No Git commit/push/automation
- No hooks, embeddings, daemon, or background service
- No daily workflow commands (`atlas start`, `atlas change`, `atlas mcp`)
- Context Graph remains a preference flag (Context Economy is the file-based Alpha surface)

## Install / local build

Requirements: Go toolchain compatible with `go.mod` (`go 1.27.1`).

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

Unsupported commands such as `atlas start`, `atlas change`, and `atlas mcp` open an Error dialog inside the TUI — they are not real commands.

## Recommended Alpha flow (Cursor + OpenCode)

1. `export ATLAS_HOME=...` if you want an isolated Home (recommended for trials).
2. From a project directory: `atlas init` → select **Cursor** and/or **OpenCode** → Review → **Apply config**.
3. Confirm materialization (below).
4. Open the project in Cursor / OpenCode; agents load from the materialized runtime paths.
5. In Atlas TUI: **Context Economy** → Review → Apply Update when you want a fresh capsule/pack.
6. Use **Status** / **Doctor** for read-only health.
7. Use **Runtime Repair** only when Atlas-owned runtime files drift or go missing.

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

Atlas Home (under `$ATLAS_HOME` / `~/.atlas`):

- Mirrored assets (`assets/…`), agents, adapters, contracts
- Project-scoped local data under `projects/<project-id>/` (identity from canonical root)
- Context Economy payloads under `projects/<project-id>/context/` after an explicit Update
- Runtime Repair backups under `projects/<project-id>/backups/<timestamp>/`

## What Status / Doctor validate

Read-only checks for config/state, `AGENTS.md` markers, selected adapters/projections, Atlas agent health, registry/manifest/lock, SDD contract presence/match, Atlas Home visibility, Context Economy state, backups, and basic tool/git presence. They never repair, rematerialize, or create Atlas Home. Status does not treat Home path as primary; Doctor may show Home path and project-local Home state as diagnostic detail.

## Runtime Repair

Explicit Review → Apply. Recomputes the plan before Apply. Restores missing/broken Atlas-owned runtime files; quarantines conflicting Atlas-surface artifacts after mandatory Home-backed backup. Preserves developer-owned files (e.g. `README.md`, `.gitignore`, non-Atlas agents). Does **not** delete Context Economy payloads under Atlas Home.

## Context Economy (v0)

Explicit Review → Apply Update. Writes `index.yaml`, `capsule.md`, and packs under Atlas Home `projects/<id>/context/`, plus minimal transitional refs in `.atlas/state.yaml`. Status/Doctor/Repair/discovery/startup do not create or refresh context payloads.

## Known Alpha limitations

- OpenSpec contract is operational guidance only — no OpenSpec CLI execution
- Claude/Codex adapters not active
- No marketplace, MCP, Git automation, hooks, embeddings, or daemon
- Configure Apply updates `.atlas/config.yaml` only; use Runtime Repair to rematerialize runtime files
- Context Graph preference has no graph engine behind it
- No git tag / GitHub release / publish in this RC unless a human explicitly approves later

## Validation

```bash
make check          # fmt + vet + unit tests
make smoke-mvp      # Alpha release-readiness smoke
```

`make smoke-mvp` runs `scripts/smoke-mvp.sh`: local build, temp `PREFIX` install, `atlas --version`, CLI routing, temp-workspace matrix under isolated `ATLAS_HOME`, focused package tests (including `home` + `context`), and repo-root cleanliness. Interactive TUI steps are printed at the end. For a non-interactive manual Alpha pass against a temp project, use `go run ./scripts/manualalpha` with `ATLAS_SMOKE_MANUAL_ROOT` + `ATLAS_HOME`.

See also:

- [docs/tui-ux-contract.md](docs/tui-ux-contract.md)
- [docs/release-notes.md](docs/release-notes.md)
- [docs/release-readiness.md](docs/release-readiness.md)

## Project layout

```
cmd/atlas/           # process entrypoint
internal/app/        # application wiring
internal/cli/        # arg parse + TUI launcher
internal/tui/        # sidebar shell (Bubble Tea + Lip Gloss)
internal/config/     # schema, drafts, persistence, runtime materialization
internal/home/       # Atlas Home resolve/inspect/mirror
internal/context/    # Context Economy (index/capsule/pack)
internal/doctor/     # diagnostics model
internal/initplan/   # init dry-run planning
internal/workspace/  # discovery, runtime health, repair
internal/assets/     # embedded agents, adapters, contracts
internal/version/    # version string (0.1.0 Alpha RC)
scripts/             # release smoke helpers
docs/                # release notes + readiness checklist
```
