# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI is a launcher; the primary interface is a full-screen interactive TUI.

## TUI-first

Atlas is TUI-first.

`atlas --version` is the only normal console output. Every other command launches a full-screen TUI shell with:

- left sidebar navigation
- right content panel
- shared header and footer
- scrollable content (`PgUp` / `PgDn`)

| Command | Behavior |
| --- | --- |
| `atlas --version` | Console: print version only |
| `atlas` | TUI shell, Status content |
| `atlas help` / `--help` / `-h` | TUI shell, Help |
| `atlas init` / `atlas init --dry-run` | TUI shell, Init Plan (dry-run) |
| `atlas status` | TUI shell, Status |
| `atlas doctor` | TUI shell, Doctor |
| `atlas start` / `atlas change` | Header + centered Error dialog + footer (Salir) |

Sidebar entries: Init / Setup, Status, Doctor, Help, Exit.

`atlas start` and `atlas change` are intentionally unsupported. They are not real commands.

## Requirements

- Go 1.22+ (developed with Go 1.27)

## Build & test

```bash
make build
make test
make fmt
make vet
make check
make clean
```

## Project layout

```
cmd/atlas/           # process entrypoint
internal/app/        # application wiring
internal/cli/        # arg parse + TUI launcher
internal/tui/        # sidebar shell Bubble Tea + Lip Gloss TUI
internal/config/     # config schema
internal/doctor/     # diagnostics model
internal/initplan/   # init dry-run planning
internal/workspace/  # read-only discovery
internal/version/    # version string
```

## Intentionally out of scope

- Project materialization (`AGENTS.md`, `.atlas/`, rules, memory, adapters)
- Interactive init apply/force
- SQLite memory creation
- Assets registry / marketplace
- Cursor/OpenCode adapters
- OpenSpec, MCP, Jira, Git/GitHub automation
- Daily workflow commands such as `atlas start` or `atlas change new`
