# Atlas Alpha release-readiness checklist

Version under test: **0.1.0** (Alpha RC).  
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
- [x] TUI-first routing + unsupported commands
- [x] init fresh (`new`) / init existing
- [x] Cursor / OpenCode / Cursor+OpenCode
- [x] Atlas Home with temporary `ATLAS_HOME`
- [x] agent + contract materialization (14 agents/runtime)
- [x] Context Economy update
- [x] status/doctor read-only
- [x] runtime repair + developer-owned preservation
- [x] no accidental writes to default `~/.atlas` when `ATLAS_HOME` is set
- [x] no generated runtime artifacts in Atlas repo root

## Manual smoke (temporary Home)

Library-equivalent (non-interactive) helper:

```bash
export ATLAS_HOME=/tmp/atlas-home-test-22
rm -rf "$ATLAS_HOME"
PROJECT=/tmp/atlas-alpha-project-22
rm -rf "$PROJECT" && mkdir -p "$PROJECT"
make build
make install PREFIX=/tmp/atlas-prefix-test-22
ATLAS_SMOKE_MANUAL_ROOT="$PROJECT" ATLAS_HOME="$ATLAS_HOME" go run ./scripts/manualalpha
```

Interactive TUI (optional confirmation):

```bash
export ATLAS_HOME=/tmp/atlas-home-test-22
cd /tmp/atlas-alpha-project-22
atlas   # or ./bin/atlas from the Atlas repo
```

Checklist (library-equivalent via `scripts/manualalpha` executed for Slice 22):

- [x] Init with Cursor + OpenCode → Apply
- [x] Verify `AGENTS.md`, `.cursor/rules/atlas.mdc`, `.opencode/atlas.md`
- [x] Verify 14 agents under `.cursor/agents/` and `.opencode/agents/`
- [x] Verify `.atlas/agent-registry.md`, `runtime-manifest.yaml`, `assets.lock.yaml`
- [x] Verify `.atlas/contracts/sdd-openspec.md`
- [x] Context Economy → Update → `index.yaml`, `capsule.md`, pack under `$ATLAS_HOME/context/projects/…`
- [x] Status / Doctor → no mutation; Home not created by read-only paths when missing
- [x] Drift agent/contract → Runtime Repair restores Atlas files
- [x] Developer-owned files preserved; Context payload intact after repair
- [x] `~/.atlas` unchanged while `ATLAS_HOME` is set
- [x] Atlas repo root free of generated runtime artifacts

Interactive TUI click-through remains optional human confirmation.

## Guarantees (must hold)

- Status / Doctor / discovery / startup do not mutate
- Runtime Repair remains explicit (Review → Apply)
- Context update remains explicit
- Atlas Home is not created by read-only flows
- Tests/smoke use temporary `ATLAS_HOME`
- No credential writes
- No generated runtime artifacts left in the Atlas repo root

## Out of scope for this RC

Marketplace, real OpenSpec CLI, Claude/Codex adapters, Git automation, hooks, embeddings, daemon, new major features.
