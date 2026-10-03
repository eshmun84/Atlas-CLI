# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI initializes Atlas in a project, configures it, reports status, runs diagnostics, and prepares runtime governance for AI agents.

## Current MVP scope (Slice 1)

This repository currently ships a minimal Go CLI foundation:

| Command | Behavior |
| --- | --- |
| `atlas` | Show root help |
| `atlas --version` | Print CLI version |
| `atlas init` | Placeholder (initialization not implemented yet) |
| `atlas status` | Placeholder (status inspection not implemented yet) |
| `atlas doctor` | Placeholder (diagnostics not implemented yet) |

No command mutates the filesystem in this slice.

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
internal/version/    # version string
```

## Out of scope (later slices)

- Project materialization (`AGENTS.md`, `.atlas/`, rules, memory, adapters)
- TUI
- SQLite memory
- Assets registry / marketplace
- Cursor/OpenCode adapters
- OpenSpec, MCP, Jira, Git/GitHub automation
- Daily workflow commands such as `atlas start` or `atlas change new`
