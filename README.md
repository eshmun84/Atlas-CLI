# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI launches a full-screen interactive TUI. Product work happens in that shell, not as console report dumps.

## MVP status (`0.1.0`)

Usable first release candidate for project-local Atlas setup:

- TUI-first shell (Dashboard, Init, Configure, Status, Doctor, Runtime Repair, Help)
- Init / Configure with config persistence under `.atlas/`
- Runtime gateway materialization (`AGENTS.md` + selected Cursor/OpenCode projections)
- Status + Doctor runtime awareness
- Explicit Runtime Repair (Review → Apply) with backup/quarantine of conflicts
- Context Graph as a preference placeholder only (no graph engine)

Not included: Atlas Home, memory engines, MCP connections, Git automation, or daily workflow commands.

## Install / local build

Requirements: Go toolchain compatible with go.mod (`go 1.27.1`).
Offline environments must have Go 1.27.1 installed locally.

```bash
make build          # writes bin/atlas
./bin/atlas --version
```

Optionally add `bin/` to your `PATH`, or run via `./bin/atlas`.

## Run

```bash
atlas                 # TUI → Dashboard
atlas init            # TUI → Init / Setup
atlas status          # TUI → Status
atlas doctor          # TUI → Doctor
atlas --version       # console: version only
```

TUI-first is the primary UX. `atlas --version` is the only normal console output. Every other route opens the interactive shell (left sidebar + right content panel). Unsupported commands such as `atlas start`, `atlas change`, and `atlas mcp` open an Error dialog inside the TUI — they are not real commands.

## What each surface does

| Surface | Role |
| --- | --- |
| **Init / Setup** | Three-step wizard: Project Setup → Initial Configuration → Review / Materialization Plan. **Apply config** writes `.atlas/` and materializes runtime gateway files. |
| **Configure** | Edit persisted settings after init. **Apply changes** updates `.atlas/config.yaml` only; it does not rematerialize runtime files. |
| **Status** | Read-only runtime health (config/state, `AGENTS.md` markers, selected adapters/projections, Context Graph preference, backups). |
| **Doctor** | Read-only PASS/WARN/FAIL checks for the same surface, including basic drift. Does not repair. |
| **Runtime Repair** | Explicit Review → Apply. Recomputes the plan before Apply. Creates/replaces missing or broken Atlas runtime files; quarantines conflicting artifacts after mandatory backup. |

## Runtime files and adapters

Init Apply always materializes `AGENTS.md` (Atlas managed/user markers + compact hardened gateway contract).

Selected adapters only:

- Cursor → `.cursor/rules/atlas.mdc`
- OpenCode → `.opencode/atlas.md`

Projections must not bypass or duplicate the full `AGENTS.md` contract. Broader surfaces (`CLAUDE.md`, `GEMINI.md`, `.agents/`, `.claude/`) are not materialized.

## Backup / quarantine

When Runtime Repair finds conflicting active runtime artifacts (unmarked `AGENTS.md`, competing `AGENT.md` / `CLAUDE.md` / `GEMINI.md` / `.agents/` / `.claude/`, extra or unselected `.cursor` / `.opencode` content), it:

1. Backs them up under `.atlas/backups/<timestamp>/` with `manifest.json`
2. Moves them out of the active surface
3. Writes governed Atlas runtime files

Backup is mandatory. There is no skip, merge, or silent delete. Valid `ATLAS:USER` content is preserved.

## Context Graph

Configure/Init expose **Enable Context Graph** as a preference only. There is no graph engine, database, embeddings, index, capsules, or packs in this MVP.

## Known limitations

- No Atlas Home / skills catalog marketplace
- No SQLite memory or context capsule file generation
- Configure Apply does not rematerialize runtime files (use Runtime Repair)
- No MCP server connections or credential storage
- No Git commit/push/automation
- `atlas start` / `atlas change` / `atlas mcp` intentionally unsupported
- Context Graph preference has no engine behind it

## Validation

```bash
make check          # fmt + vet + unit tests
make smoke-mvp      # MVP release-readiness smoke (build + matrix)
```

`make smoke-mvp` runs `scripts/smoke-mvp.sh`: local build, `atlas --version`, CLI routing checks, temp-workspace init/status/doctor/repair matrix, focused package tests, and a repo-root cleanliness check. Interactive TUI navigation steps are printed at the end for manual confirmation.

See also [docs/release-notes.md](docs/release-notes.md).

## Project layout

```
cmd/atlas/           # process entrypoint
internal/app/        # application wiring
internal/cli/        # arg parse + TUI launcher
internal/tui/        # sidebar shell (Bubble Tea + Lip Gloss)
internal/config/     # schema, drafts, persistence, runtime materialization
internal/doctor/     # diagnostics model
internal/initplan/   # init dry-run planning
internal/workspace/  # discovery, runtime health, repair
internal/version/    # version string
scripts/             # release smoke helpers
docs/                # release notes
```
