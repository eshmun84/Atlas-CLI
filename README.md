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
| `atlas status` | TUI shell, Status |
| `atlas doctor` | TUI shell, Doctor |
| `atlas start` / `atlas change` / `atlas mcp` | Header + centered Error dialog + footer (Salir) |

Sidebar entries:

- Not initialized: Dashboard, Init / Setup, Status, Doctor, Help, Exit
- Initialized (valid `.atlas/config.yaml`): Dashboard, Configure, Status, Doctor, Help, Exit

Init / Setup is a three-step in-memory wizard:

1. **Project Setup** — project name, New/Existing mode, runtime artifact gate
2. **Initial Configuration** — sectioned selector with Governance, Adapters, Source Control, Memory, Context, and MCP
3. **Review / Materialization Plan** — preview of planned creates, backups, replacements, and preservations

Init Step 1, Init Step 2, Init Step 3, and Configure place their action row immediately after the screen content. The global footer follows that row. On tall terminals the extra space stays below the shell, not between the action row and the footer.

Init Step 2 uses vertical checkbox/radio-style rows. Arrow keys move focus only. Values change only with Space or Enter. Workflow currently supports SDD only. Spec engine supports OpenSpec or None. Source Control includes Atlas governance files (Local only / Versioned). Memory strategy supports SQLite, Context Capsule, or SQLite + Context Capsule. Context exposes a single preference checkbox: **Enable Context Graph** (default enabled; preference only — no graph engine yet). MCP follows Context. The project remains a compact gateway; Atlas Home stays the canonical source of skills/agents/rules. There is no Project Stack configuration in init.

Init Step 3 summarizes the in-memory project setup, configuration, and MCP draft. **Apply config** writes Atlas-owned files under `.atlas/` (`config.yaml`, `local.yaml`, `state.yaml`, `assets.lock.yaml`, and `backups/`) and materializes runtime entrypoints: `AGENTS.md` always; `.cursor/rules/atlas.mdc` when Cursor is selected; `.opencode/atlas.md` when OpenCode is selected. Existing Atlas-managed targets are backed up under `.atlas/backups/<timestamp>/` with `manifest.json` before replacement. Apply does not commit, push, or modify Git.

Configure uses the same visible sections after initialization, including MCP. It loads values from `.atlas/config.yaml` when present. The action row always shows **[ Close ]** and **[ Apply changes ]** (including while editing the MCP section). **Close** discards unsaved edits. **Apply changes** persists the current Configure draft (including MCP) to `.atlas/config.yaml` only and shows `Configuration changes saved.`

MCP is configured during Init Step 2 or later in Configure. Built-in MCPs are Jira, Context7, and Chrome DevTools (multi-select). Add MCP creates a custom MCP entry with Name, Transport (stdio / http / sse), Command or URL, Arguments, and Environment references. Custom entries can be removed in memory with `d`. Custom is not a built-in row. Review summarizes selected built-ins and custom entries. Init **Apply config** and Configure **Apply changes** both persist MCP enablement and custom connection references in `.atlas/config.yaml`. MCP does not connect to servers or store credentials. There is no standalone MCP sidebar item and `atlas mcp` is unsupported.

Init Apply creates `.atlas` config files and the allowlisted runtime entrypoints above. `AGENTS.md` uses Atlas managed/user marker sections and a compact 1–10 gateway blueprint (Rules through Agent/Subagent Orchestration), with a minimal Context Graph preference note. Configure **Apply changes** still updates `.atlas/config.yaml` only and does not rematerialize runtime files.

`atlas start`, `atlas change`, and `atlas mcp` are intentionally unsupported. They are not real commands.

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
internal/config/     # config schema + ConfigDraft + MCPDraft + .atlas persistence
internal/doctor/     # diagnostics model
internal/initplan/   # init dry-run planning
internal/workspace/  # read-only discovery
internal/version/    # version string
```

## Intentionally out of scope

- SQLite memory / context capsule files
- Assets registry / marketplace
- Broader adapter surfaces beyond `.cursor/rules/atlas.mdc` and `.opencode/atlas.md`
- CLAUDE.md / GEMINI.md / `.agents/` / `.claude/` materialization
- OpenSpec automation
- MCP connections or credentials
- Jira / Git / GitHub automation
- Daily workflow commands such as `atlas start` or `atlas change new`
