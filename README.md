# Atlas CLI

Atlas is a governed AI-assisted software engineering framework.

Atlas is **not** a coding agent. The CLI launches a full-screen interactive TUI. Product work happens in that shell, not as console report dumps.

## Current development / dogfooding baseline

Source version string remains `0.1.0` (see `internal/version`). Version selection and release tagging belong to a later hardening block — do not treat `0.1.0` as a frozen product identity.

This repository is the post–Slice 3 dogfooding baseline: installable locally, validated by `make check` + `make smoke-mvp` and the Slice 3 A–N regression matrix, usable with Cursor and OpenCode. It is **not** production-ready and **not** final V1.

Do not tag, create a GitHub release, or publish without explicit human approval.

### What Atlas does today

- TUI-first shell with **Status** as the default landing (`atlas` → Status). No separate Dashboard/Home screen.
- Init / Configure, Doctor (read-only), Runtime Repair (Review → Apply), Context Economy, Code Intelligence refresh, Help
- Atlas Home (`ATLAS_HOME` or `~/.atlas`) with mirrored bundled assets and project-scoped `projects/<id>/` isolation
- Filesystem safety and mutation locking for Atlas-owned writes
- Runtime materialization: composed `AGENTS.md`, Cursor and OpenCode projections, runtime agents (orchestrator, SDD, review, worker), registry/manifest/lock
- SDD governance with **real OpenSpec provider** integration (exact project pin; Init runs `openspec init`; Doctor fails closed on missing/incompatible CLI; see Spec Engine notes below)
- Task Capability Discovery: `atlas capability plan "<task>" --json` (bounded evidence → task-active skills → proactive governed retrieval planning; technology is evidence only)
- Skills v1: local Home catalog, exact pins, deterministic digests, metadata resolver, progressive loading, Cursor/OpenCode projections, ownership/conflict handling, Repair integration
- Memory Core, governed retrieval, governed persistence, Memory diagnostics, Memory Skill, Context Composition integration
- Context Economy v0 (explicit index / capsule / pack under Atlas Home)
- Code Intelligence with optional externally installed CodeGraph provider (`internal/codeintel`)
- MCP foundation with Atlas-owned Cursor/OpenCode projections (secrets never stored; env/header refs + auth modes only)
- Neutral Hooks engine plus Cursor native hooks and OpenCode native hooks (production action registry has **no** automatic Memory injection or other side-effect actions yet)
- Git local bootstrap when source control requires a repo; signing policy; local-only `.gitignore` Atlas block; brownfield backup into Atlas Home
- Doctor repairability metadata (`atlas-repair` / `confirm-repair` / `diagnostic-only` / `manual`) and Slice 3 regression/readiness evidence
- `atlas --version` as the only normal console output

### What Atlas does **not** do yet

- Not production-ready / not final V1
- No Atlas Context Graph (compatibility flag may exist in config; **no** Init/Configure enable/disable checkbox until an end-to-end Context Graph exists)
- No embeddings; no daemon / background service
- No native automatic Memory injection into agent context (explicit/proactive governed retrieval via Memory Skill / CLI / composition is supported)
- No remote/community skills marketplace; no Framework Skill Acquisition; no Skill Creator
- No Atlas Native Spec Engine; no additional Spec Engines beyond OpenSpec
- No OpenSpec emulation / mkdir-bootstrap of `openspec/` (provider init belongs to OpenSpec via Atlas Init; Runtime Repair does not bootstrap OpenSpec)
- No generic provider plugin platform (domain-specific boundaries only: adapters, Spec Engines, Code Intelligence, MCP)
- No Core automation for remote repository creation, push, PR, merge, tag, or repository-release publishing (policy/diagnostics/governance remain; future skills may assist developer/agent workflows)
- No MCP secret storage, OAuth client, or live connection verification
- No Claude / Codex activation
- No daily workflow commands (`atlas start`, `atlas change`, `atlas mcp`)

## Spec Engine (OpenSpec)

**Supported Spec Engine = OpenSpec only.** Atlas Native Spec Engine is deferred. Additional engines are future.

| | Current behavior |
| --- | --- |
| Provider | Real OpenSpec CLI required when `workflow=sdd` + `spec_engine=openspec` |
| Pin | Exact `governance.openspec_version`; local CLI must match (older/newer mismatch blocks governed SDD) |
| Init | Atlas Init Apply runs real `openspec init` (+ tool projections); adopts pin when empty |
| Filesystem | `openspec/` lifecycle belongs to OpenSpec — Atlas does **not** mkdir-bootstrap as a substitute |
| Repair | Runtime Repair **refuses** OpenSpec provider bootstrap; Init owns first-time / legacy remediation |
| Emulation | No silent fallback to an internal engine; malformed/legacy fail closed |
| Atlas ownership | SDD governance, agents, authority, fail-closed policy — not provider FS semantics |

## Install / local build

Requirements: **Go 1.27.1** (matches `go.mod`; use a toolchain that satisfies that directive).

```bash
make build                    # writes bin/atlas
./bin/atlas --version

# Optional local install (defaults to ~/.local when PREFIX unset)
make install                  # installs binary to $PREFIX/bin (default: ~/.local/bin)
# or temporary:
make install PREFIX=/tmp/atlas-prefix
```

`make install` does **not** modify your shell `PATH`. Ensure the install bin directory is on `PATH`, for example:

```bash
# bash / zsh (add to ~/.bashrc or ~/.zshrc if you want it permanent)
export PATH="$HOME/.local/bin:$PATH"
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

## Recommended dogfooding flow (Cursor + OpenCode)

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
| `.cursor/agents/atlas-*.md` | Atlas runtime agents (when Cursor selected) |
| `.opencode/agents/atlas-*.md` | Atlas runtime agents (when OpenCode selected) |
| Skill projections | Cursor/OpenCode skill surfaces for pinned Skills v1 packages |
| Hooks projections | Cursor native hooks / OpenCode hooks plugin when selected |
| `.cursor/mcp.json` / `opencode.json` | Atlas-owned MCP entries merged when selected (developer entries preserved). `opencode.jsonc` is **not** merged — OpenCode MCP reconcile blocks if JSONC is present |

Atlas Home (under `$ATLAS_HOME` / `~/.atlas`):

- Mirrored assets (`assets/…`), agents, adapters, contracts, skills
- Project-scoped local data under `projects/<project-id>/` (identity from canonical root; same name + different root → different ID)
- Context Economy payloads under `projects/<project-id>/context/` after an explicit Update
- Memory storage under the project Home footprint
- Runtime Repair backups under `projects/<project-id>/backups/<timestamp>/`
- MCP ownership + backups under `projects/<project-id>/mcp/`
- Optional CodeGraph data under `projects/<project-id>/codegraph/`

## What Status / Doctor validate

Read-only checks for config/state, `AGENTS.md` markers, selected adapters/projections, Atlas agent health, Skills/hooks health where applicable, registry/manifest/lock, SDD contract presence/match, OpenSpec layout when required, Atlas Home visibility, Memory diagnostics, Context Economy state, MCP projection health, optional Code Intelligence health, backups, signing policy visibility, and basic tool/git presence. They never repair, rematerialize, or create Atlas Home.

## Runtime Repair

Explicit Review → Apply. Recomputes the plan before Apply. Restores missing/broken **Atlas-owned** runtime files (with Home-backed backup before replace/quarantine of Atlas-owned paths). Never moves or deletes developer/external surfaces such as `CLAUDE.md`, `GEMINI.md`, `AGENT.md`, `.agents/`, or `.claude/` — Status/Doctor may report coexistence only. Preserves `.cursor/mcp.json`, `opencode.json`, non-Atlas rules/settings, `README.md`, and non-Atlas agents. May reconcile reconcilable local-only `.gitignore` Atlas blocks. Apply mutates only surfaces represented by the reviewed repair plan (e.g. hooks only when a hook target is planned). Does **not** bootstrap OpenSpec (use Atlas Init). Does **not** delete Context Economy payloads under Atlas Home. Does **not** reconcile MCP (use Configure Apply).

## Configure

Configure Apply saves `.atlas/config.yaml` (and may create optional `docs/atlas/README.md` once when selected on Init and still missing). Does **not** rematerialize general runtime assets (`AGENTS.md`, rules, agents). **Does** reconcile Atlas-owned MCP projections when MCP/adapters change. Custom remote MCP supports auth modes: none, environment header (`env` + optional prefix such as `Bearer `), external OAuth, provider managed — references only, never raw secrets. No Skills marketplace UI and no Context Graph checkbox.

## Context Economy (v0)

Explicit Review → Apply Update. Writes `index.yaml`, `capsule.md`, and packs under Atlas Home `projects/<id>/context/`, plus minimal transitional refs in `.atlas/state.yaml`. Status/Doctor/Repair/discovery/startup do not create or refresh context payloads. This is **not** an Atlas Context Graph engine.

## Code Intelligence

Optional. CodeGraph is an externally installed provider behind `internal/codeintel`. See [docs/code-intelligence/](docs/code-intelligence/README.md). No CodeGraph MCP. Atlas Context Graph remains deferred and separate.

## Skills

Skills v1 is implemented: bundled catalog in Atlas Home, exact ID/version pins, digests, metadata-based resolve, Cursor/OpenCode projections, ownership handling, and Repair integration. There is **no** Skills UI, marketplace screen, or per-project checkbox catalog.

**Task Capability Discovery (Block C)** resolves DOGFOOD-015: task → bounded evidence → capability/context needs → registry → task-active skills → governed retrieval actions. Technology discovery is one evidence source — **not** permanent capability selection. CLI: `atlas capability plan "<task>" --json` (read-only; no Memory/CodeGraph/MCP during planning). Projected pins remain for adapter compatibility; selected-for-task skills are a separate concept.

Future direction (not implemented):

- **Framework Skill Acquisition** — inspect/validate/review/approve/install a skill into Atlas Home for availability across projects (does **not** auto-project into every project)
- **Skill Creator** — help author Atlas-compatible `SKILL.md` packages for later acquisition

## Memory

Supported today: explicit/proactive governed retrieval, Memory Skill, CLI, governed persistence, diagnostics, and use inside Context Composition. Native automatic Memory injection via adapter/host hooks is **deferred**. The production hooks action registry remains empty for those automatic actions.

## Source control boundary

Atlas Core retains local Git bootstrap, source-control mode/policy, signing policy, diagnostics, and governance/authority rules.

Atlas Core does **not** create remotes, push, open PRs, merge, tag, or publish repository releases. Those operations belong to developer ↔ AI agent workflows; future skills may assist them under governance. This boundary applies to version-control remote/repository operations only — not to Docker, Kubernetes, deployment, CI/CD, or infrastructure domains.

## Deferred / planned / removed (documentation)

| Topic | Status |
| --- | --- |
| Atlas Context Graph | Deferred |
| Embeddings | Planned / deferred (until FTS/metadata/CodeGraph/CE limits justify it) |
| Daemon / background service | Deferred architectural option (prefer hooks + on-demand execution) |
| Config History | Deferred (current config/state/ownership/backups/tx/locks/rollback suffice) |
| Generic provider plugin platform | Removed from current V1 plan |
| Atlas Core repository-release publisher | Removed |
| Native automatic Memory injection | Deferred |

## Uninstall / purge (docs-only this cycle)

Atlas does **not** ship `make uninstall`. Removal is intentional and layered:

| Goal | What to remove | Notes |
| --- | --- | --- |
| Uninstall binary | `$PREFIX/bin/atlas` (default `~/.local/bin/atlas`) | Does not touch Atlas Home or projects |
| Remove Atlas from a project | Project-local `.atlas/`, Atlas-owned runtime files (`AGENTS.md`, adapter projections, hooks wrappers, etc.) as you choose | Developer-owned files (README, MCP configs, OpenSpec trees) are separate |
| Purge Atlas Home | `$ATLAS_HOME` (default `~/.atlas`) | **Destructive / optional.** Home may contain project Memory DBs, CodeGraph DBs, backups, skill assets/state. Do **not** treat `rm -rf ~/.atlas` as a casual uninstall. |

Example binary removal only:

```bash
rm -f "${PREFIX:-$HOME/.local}/bin/atlas"
```

Optional Home purge (destructive — back up first if you need Memory/CodeGraph/skills data):

```bash
# WARNING: deletes local Atlas state under ATLAS_HOME
# rm -rf "${ATLAS_HOME:-$HOME/.atlas}"
```

## Known limitations

- Not production-ready / not final V1
- OpenSpec: real provider required for governed SDD; exact pin; Init owns `openspec init`; Repair does not bootstrap OpenSpec; missing opsx/tool projections are Configure/Init (not Runtime Repair)
- Cursor + OpenCode only; Claude/Codex not activated
- No Atlas Context Graph implementation
- No marketplace/remote skill install; no Framework Skill Acquisition / Skill Creator
- MCP projections without secret manager / live connection verification
- No Core Git remote/PR/merge/tag/release automation beyond Init local bootstrap
- `delivery_assist` is a legacy/deferred config key (not an editable Core capability)
- Configure Apply updates `.atlas/config.yaml` and MCP projections; use Runtime Repair to rematerialize non-MCP runtime files
- OpenCode MCP: Atlas merges only `opencode.json`. If `opencode.jsonc` exists, OpenCode MCP reconciliation is blocked until JSONC-safe merging exists
- No git tag / GitHub release / publish of Atlas itself unless a human explicitly approves later

## Validation

```bash
make check          # fmt + vet + unit tests
make smoke-mvp      # dogfooding smoke
```

`make smoke-mvp` runs `scripts/smoke-mvp.sh`: local build, temp `PREFIX` install, `atlas --version`, CLI routing, temp-workspace matrix under isolated `ATLAS_HOME`, focused package tests, and repo-root cleanliness.

Slice 3 A–N regression evidence: `go test ./internal/runtime/ -run Slice3` (see [docs/release-readiness.md](docs/release-readiness.md)).

See also:

- [docs/tui-ux-contract.md](docs/tui-ux-contract.md)
- [docs/release-notes.md](docs/release-notes.md)
- [docs/release-readiness.md](docs/release-readiness.md)
- [docs/code-intelligence/](docs/code-intelligence/README.md)
- [docs/memory-delivery-capability.md](docs/memory-delivery-capability.md)

## Project layout

```
cmd/atlas/              # process entrypoint
internal/app/           # application wiring
internal/cli/           # arg parse + TUI launcher
internal/tui/           # sidebar shell (Bubble Tea + Lip Gloss)
internal/config/        # schema, drafts, persistence, Init/Configure apply
internal/adapters/      # Cursor / OpenCode adapter surfaces
internal/mcp/           # Atlas-owned MCP desired state, projectors, ownership, reconcile
internal/skills/        # Skills v1 catalog, pins, digest, resolve, reconcile
internal/hooks/         # Neutral hooks types / ownership
internal/hooksengine/   # Adapter-neutral hooks dispatch
internal/hookproj/      # Hooks projection reconcile
internal/memory/        # Memory Core
internal/memoryretrieve/
internal/memorypersist/
internal/memoryhealth/
internal/inspect/       # composed project inspection
internal/project/       # filesystem/git/tools snapshot + fsafety + mutatelock
internal/runtime/       # runtime health + Runtime Repair
internal/sdd/           # Spec Engine boundary + SDD service
internal/specengine/    # OpenSpec adapter (current implementation)
internal/codeintel/     # Code Intelligence provider boundary + CodeGraph adapter
internal/context/       # Context Economy (index/capsule/pack)
internal/contextcompose/# Context Composition
internal/delivery/      # source-control platform preference (not remote ops engine)
internal/home/          # Atlas Home resolve/inspect/mirror
internal/doctor/        # diagnostics model + repairability
internal/initplan/      # init dry-run planning
internal/assets/        # embedded agents, adapters, contracts, skills
internal/version/       # version string (currently 0.1.0; bump later)
scripts/                # smoke / e2e helpers
docs/                   # readiness, notes, code intelligence, memory delivery
```
