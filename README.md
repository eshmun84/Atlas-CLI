# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI initializes Atlas in a project, configures it, reports status, runs diagnostics, and prepares runtime governance for AI agents.

## Current MVP scope

Slice 1 shipped a minimal Go CLI foundation:

| Command | Behavior |
| --- | --- |
| `atlas` | Show root help |
| `atlas --version` | Print CLI version |
| `atlas init` | Read-only dry-run initialization plan |
| `atlas init --dry-run` | Same dry-run plan (explicit) |
| `atlas status` | Read-only workspace status report |
| `atlas doctor` | Read-only diagnostics (PASS/WARN/FAIL) |

Slice 2 added an internal config schema foundation (`internal/config`): defaults, validation, YAML load/save, path constants, and a mutability model. No project files are materialized yet.

Slice 3 added internal read-only workspace discovery (`internal/workspace`): filesystem markers, Git repo/branch/remotes, technology signals, and local tool availability. Nothing is materialized or mutated.

Slice 4 wired `atlas status` to that discovery package for a concise read-only status report. It does not create `.atlas`, `AGENTS.md`, or any other project files.

Slice 5 wired `atlas doctor` to read-only diagnostics over the same discovery result. Missing Atlas markers are warnings for now; doctor does not mutate the project.

Slice 6 replaced `atlas init` with a read-only dry-run plan (`internal/initplan`). It infers a default project config and lists files that a later materialization slice would create. No files are written.

## Requirements

- Go 1.22+ (developed with Go 1.27)

## Build & test

```bash
make build    # build bin/atlas
make test     # go test ./...
make fmt      # gofmt -w .
make vet      # go vet ./...
make check    # fmt check + vet + test
make clean    # remove bin/
```

Or directly:

```bash
go build -o bin/atlas ./cmd/atlas
go test ./...
```

## Project layout

```
cmd/atlas/           # CLI entrypoint
internal/app/        # application wiring
internal/cli/        # command routing and handlers
internal/config/     # config schema, defaults, validation, I/O
internal/doctor/     # read-only diagnostics checks and report
internal/initplan/   # read-only init dry-run planning
internal/workspace/  # read-only workspace discovery
internal/version/    # version string
```

## Intentionally out of scope

- Project materialization (`AGENTS.md`, `.atlas/`, rules, memory, adapters)
- TUI
- SQLite memory
- Assets registry / marketplace
- Cursor/OpenCode adapters
- OpenSpec, MCP, Jira, Git/GitHub automation
- Daily workflow commands such as `atlas start` or `atlas change new`; these are intentionally excluded from Atlas CLI.
