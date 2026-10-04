# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI is a launcher; the primary interface is a full-screen interactive TUI.

## TUI-first

Atlas is TUI-first.

`atlas --version` is the only normal console output. Every other command launches a full-screen TUI shell with:

- left sidebar navigation
- right content panel
- shared compact header and content-sized footer
- content-fit shell on short screens: action row sits directly above the footer, without stretching the body to fill the terminal
- one-line top gap on normal-height terminals
- scrollable content (`PgUp` / `PgDn`) when the screen is taller than the terminal

| Command | Behavior |
| --- | --- |
| `atlas --version` | Console: print version only |
| `atlas` | TUI shell, Dashboard |
| `atlas help` / `--help` / `-h` | TUI shell, Help |
| `atlas init` / `atlas init --dry-run` | TUI shell, Init / Setup wizard |
| `atlas mcp` | TUI shell, MCP integrations |
| `atlas status` | TUI shell, Status |
| `atlas doctor` | TUI shell, Doctor |
| `atlas start` / `atlas change` | Header + centered Error dialog + footer (Salir) |

Sidebar entries:

- Not initialized: Dashboard, Init / Setup, MCP, Status, Doctor, Help, Exit
- Initialized (valid `.atlas/config.yaml`): Dashboard, Configure, MCP, Status, Doctor, Help, Exit

Init / Setup is a three-step in-memory wizard:

1. **Project Setup** — project name, New/Existing mode, runtime artifact gate
2. **Initial Configuration** — sectioned selector with Governance, Adapters, Source Control, Memory, and MCP
3. **Review / Materialization Plan** — preview of planned creates, backups, replacements, and preservations

Init Step 1, Init Step 2, Init Step 3, Configure, and MCP place their action row immediately after the screen content. The global footer follows that row. On tall terminals the extra space stays below the shell, not between the action row and the footer.

Init Step 2 uses vertical checkbox/radio-style rows. Arrow keys move focus only. Values change only with Space or Enter. Workflow currently supports SDD only. Spec engine supports OpenSpec or None. Source Control includes Atlas governance files (Local only / Versioned). Memory strategy supports SQLite, Context Capsule, or SQLite + Context Capsule. MCP follows Memory and uses the same in-memory MCP draft as the standalone MCP screen. Runtime and Skills/Registry stay internal Atlas behavior. There is no Project Stack configuration in init.

Init Step 3 is preview-only. It summarizes the in-memory project setup, configuration, and MCP draft. Apply / materialization is not implemented. No files are written.

Configure uses the same visible sections in post-init mode. Edits stay in memory only.

MCP can be configured from Init Step 2 or from the standalone MCP sidebar screen. Both use the same in-memory MCP draft. Built-in MCPs are Jira, Context7, and Chrome DevTools (multi-select). Add MCP creates a custom MCP entry with Name, Transport (stdio / http / sse), Command or URL, Arguments, and Environment references. Custom entries can be removed in memory with `d`. Custom is not a built-in row. Review summarizes selected built-ins and custom entries. MCP does not connect to servers, validate credentials, or persist files yet.

No configuration is persisted yet. No materialization exists yet. `.atlas` and `AGENTS.md` are still not created.

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
internal/config/     # config schema + ConfigDraft + MCPDraft models
internal/doctor/     # diagnostics model
internal/initplan/   # init dry-run planning
internal/workspace/  # read-only discovery
internal/version/    # version string
```

## Intentionally out of scope

- Project materialization (`AGENTS.md`, `.atlas/`, rules, memory, adapters)
- Interactive init apply / file writes
- SQLite memory creation
- Assets registry / marketplace
- Cursor/OpenCode adapters
- OpenSpec automation
- MCP connections, credentials, or persistence
- Jira / Git / GitHub automation
- Daily workflow commands such as `atlas start` or `atlas change new`
