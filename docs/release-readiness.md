# Atlas release-readiness checklist (dogfooding)

Version under test: **0.1.0** (source string; version bump deferred to a later hardening block).
Toolchain: **Go 1.27.1** (required by `go.mod`; local/CI builds must satisfy that directive).

This checklist marks readiness for **resumed physical dogfooding**. It does **not** claim production readiness or final V1.

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
- [x] unsupported commands exit non-zero and do not mutate
- [x] TUI-first routing
- [x] empty directory (fresh non-Atlas)
- [x] Git + README existing project
- [x] initialized Atlas project
- [x] Cursor-only init
- [x] OpenCode-only init
- [x] Cursor + OpenCode init
- [x] Status read-only
- [x] Doctor read-only
- [x] Runtime Repair
- [x] Context Economy update / read-only / stale detection
- [x] Configure Apply (config.yaml + MCP reconcile; no silent non-MCP rematerialize)
- [x] Init Home reset gate
- [x] `ATLAS_HOME` project isolation (default `~/.atlas` untouched)
- [x] same-name different-root isolation
- [x] no generated runtime artifacts in Atlas repo root

## Manual smoke (temporary Home + PREFIX)

Library-equivalent (non-interactive) helper:

```bash
export ATLAS_HOME=/tmp/atlas-home-dogfood
rm -rf "$ATLAS_HOME"
PROJECT=/tmp/atlas-dogfood-project
rm -rf "$PROJECT" && mkdir -p "$PROJECT"
make build
make install PREFIX=/tmp/atlas-prefix-dogfood
ATLAS_SMOKE_MANUAL_ROOT="$PROJECT" ATLAS_HOME="$ATLAS_HOME" go run ./scripts/manualalpha
```

Interactive TUI (optional confirmation):

```bash
export ATLAS_HOME=/tmp/atlas-home-dogfood
cd /tmp/atlas-dogfood-project
atlas   # or ./bin/atlas from the Atlas repo
```

Checklist (library-equivalent via `scripts/manualalpha`):

- [x] Init with Cursor + OpenCode → Apply
- [x] Verify `AGENTS.md`, `.cursor/rules/atlas.mdc`, `.opencode/atlas.md`
- [x] Verify Atlas agents under `.cursor/agents/` and `.opencode/agents/`
- [x] Verify `.atlas/agent-registry.md`, `runtime-manifest.yaml`, `assets.lock.yaml`
- [x] Verify `.atlas/contracts/sdd-openspec.md`
- [x] Context Economy → Update → `index.yaml`, `capsule.md`, pack under `$ATLAS_HOME/projects/<id>/context/…`
- [x] Status / Doctor → no mutation; Home not created by read-only paths when missing
- [x] Drift agent/contract → Runtime Repair restores Atlas files
- [x] Developer-owned files preserved; Context payload intact after repair
- [x] `~/.atlas` unchanged while `ATLAS_HOME` is set
- [x] Atlas repo root free of generated runtime artifacts

Interactive TUI click-through remains optional human confirmation.

## Slice 3 diagnostic invariants

- Shared inspection stack only: `inspect.Inspect` → `project.Inspect` → `runtime.EvaluateHealth` → `sdd.Evaluate`
- `atlas status` / `atlas doctor` are strictly read-only (no Home create, no project writes, no Git mutation, no Memory migrate, no CodeGraph rebuild, no rematerialize)
- Doctor severity remains PASS / INFO / WARN / FAIL
- Doctor checks may carry repairability: `atlas-repair` | `confirm-repair` | `diagnostic-only` | `manual` (+ optional recommended action)
- `source_control.mode=none`: missing Git repo is expected (Status omits Git WARN noise; Doctor PASS/INFO)
- `git_local` / `git_github` without repo → FAIL; missing git executable when Git is required → FAIL
- Signing: Doctor shows Atlas `signing_required` and read-only `commit.gpgsign`; policy required + gpgsign false → WARN (not FAIL); Repair never rewrites signing
- Required OpenSpec (`workflow=sdd` + `spec_engine=openspec`): real CLI provider required; missing/incompatible/pin-mismatch → Doctor FAIL (`manual`); absent/legacy project → Doctor FAIL with Init guidance; Runtime Repair does **not** bootstrap OpenSpec
- Local-only `.gitignore`: same analyzer for Init / Doctor / Repair; malformed markers → FAIL manual, no mutation
- Doctor `atlas-repair` / `confirm-repair` tags must match Runtime Repair plan/apply authority (DOGFOOD-019); Apply mutates only reviewed plan targets (DOGFOOD-018)

## Runtime Repair authority boundary (Slice 3 + Block D Slice A)

**May repair (explicit Review → Apply):** Atlas Home mirror, Atlas-owned runtime assets (AGENTS/agents/registries/manifest/lock/contracts/skills/hooks), reconcilable local-only `.gitignore` Atlas block (with project-scoped backup when writing). Apply mutates a managed surface only when that surface is represented by an approved repair target.

**Must not repair:** OpenSpec provider bootstrap / `openspec init`, Git init/branch/remote/push/commit, signing policy, `.atlas/config.yaml` rewrite, Memory migrate/init, CodeGraph rebuild, partial/malformed/legacy OpenSpec trees, malformed `.gitignore` delimiters, README, MCP live auth, arbitrary user files.

Transactional Apply + plan signature + rollback preserved; gitignore participates in the same transaction when planned. Repair is idempotent for healthy surfaces. OpenSpec first-time / legacy remediation belongs to **Atlas Init**.

## Slice 3 regression matrix (A–N)

Authoritative automated coverage: `TestSlice3_Matrix_*` in `internal/runtime/slice3_matrix_test.go`, plus `internal/doctor/slice3_diagnostics_test.go`, `internal/runtime/slice3_repair_test.go`, and Slice 1/2 materialization/contract tests.

| ID | Scenario | Primary test(s) |
| --- | --- | --- |
| A | Outside project / no Home create | `TestSlice3_Matrix_A_OutsideProject` |
| B | Fresh install idempotent | `TestSlice3_Matrix_B_FreshInstall` |
| C | Greenfield Git + SDD/OpenSpec | `TestSlice3_Matrix_C_GreenfieldGitSDD`, Slice1 greenfield |
| D | Greenfield `source_control=none` | `TestSlice3_Matrix_D_GreenfieldSourceControlNone` |
| E | Brownfield README + gitignore | `TestSlice3_Matrix_E_BrownfieldReadmeGitignore` |
| F | Brownfield existing Git | `TestSlice3_Matrix_F_BrownfieldExistingGit` |
| G | Missing Home fail-closed | `TestSlice3_Matrix_G_MissingHome` |
| H | OpenSpec absent/partial | `TestSlice3_Matrix_H_OpenSpecAbsentAndPartial`, repair tests |
| I | Malformed `.gitignore` | `TestSlice3_Matrix_I_MalformedGitignore` |
| J | Stale Atlas runtime assets | `TestSlice3_Matrix_J_StaleRuntimeAssets` |
| K | Signing required | `TestSlice3_Matrix_K_SigningRequired`, Slice2 signing |
| L | Provider/origin/adapter | `TestSlice3_Matrix_L_ProviderSemantics`, Dogfood010 |
| M | Memory read-only | `TestSlice3_Matrix_M_MemoryReadOnly` |
| N | CodeGraph read-only | `TestSlice3_Matrix_N_CodeGraphReadOnly` |

## DOGFOOD-001..015 mapping (Slice 3)

| ID | Classification |
| --- | --- |
| 001 | fixed S1 / regression S3 |
| 002 | fixed S1 / regression S3 |
| 003 | fixed S1 / diagnostics S3 |
| 004 | fixed S1 / diagnostics+repair S3 |
| 005 | fixed S1 / regression S3 |
| 006 | fixed S1 / diagnostics+repair S3 |
| 007 | fixed S1 / regression S3 |
| 008 | fixed S1/S2 / regression S3 |
| 009 | fixed S2 / regression S3 |
| 010 | fixed S2 / regression S3 |
| 011 | fixed S2 / regression S3 |
| 012 | semantics fixed S2; full delivery assist deferred (not a Core remote-ops engine) |
| 013 | fixed S2 / regression S3 |
| 014 | fixed S1 / repair regression S3 |
| 015 | **resolved (Block C)** — task-scoped capability discovery; technology is bounded evidence, not permanent skill selection (`atlas capability plan`) |
| 016 | **resolved (Block D Slice A)** — docs/UI product truth sync (OpenSpec provider + Capability Discovery) |
| 017 | **resolved (Block D Slice A)** — runtime agents require real OpenSpec; no CLI-optional / emulation bypass |
| 018 | **resolved (Block D Slice A)** — Repair Apply mutates only reviewed plan targets (hooks) |
| 019 | **resolved (Block D Slice A)** — Doctor repairability aligned with Repair/Init/manual authority |
| 022 | **resolved (Block D / D5)** — real OpenSpec Init + team-clone exact-pin dogfood (`scripts/dogfoodopenspec`); older/newer/missing CLI block; no silent repin; Repair refuses bootstrap; capability probes covered without SkipCapProbes |

## Physical dogfooding readiness gate

Resume physical dogfooding only when all are green:

```bash
go test ./...
go test -race ./...
go vet ./...
make check
make smoke-mvp
# plus Slice 3 A–N matrix (covered by go test ./internal/runtime/ -run Slice3)
```

Evidence required: clean install; greenfield + brownfield init; Status/Doctor read-only; Repair idempotent and plan-scoped; `.gitignore` reconcilable repair safe; OpenSpec absent/legacy → Init (not Repair bootstrap); malformed shared state fail-closed; DOGFOOD-001..015 no blocker inside remediated scope; DOGFOOD-016..019 (Block D Slice A) truth/governance/repair-authority aligned; DOGFOOD-022 real OpenSpec + team-clone dogfood (`ATLAS_BIN=./bin/atlas go run ./scripts/dogfoodopenspec`).

Gate label when passing: **ready for resumed physical dogfooding** (not a production-ready claim).

## Guarantees (must hold)

- Status / Doctor / discovery / startup do not mutate
- Runtime Repair remains the explicit mutation path for non-MCP runtime artifacts (Review → Apply)
- Configure Apply saves config.yaml and reconciles Atlas-owned MCP projections; it does not silently rematerialize AGENTS/rules/agents
- Context update remains explicit
- Init Home reset requires explicit acceptance and deletes only `$ATLAS_HOME/projects/<project-id>/`
- Atlas Home is not created by read-only flows
- Tests/smoke use temporary `ATLAS_HOME`
- No credential writes
- No generated runtime artifacts left in the Atlas repo root
- No tag / release / publish without explicit human approval

## Honest capability boundaries

| Area | Current dogfooding stance |
| --- | --- |
| Materialized runtimes | Cursor + OpenCode only |
| Status / Doctor | Read-only |
| Runtime Repair | Explicit mutation path; idempotent for healthy surfaces |
| Context Economy v0 | File-based, explicit update |
| CodeGraph | Optional Code Intelligence provider (externally installed; not MCP; may be unavailable) |
| Atlas Context Graph | Preference/compat only; NOT IMPLEMENTED; no Init/Configure checkbox |
| MCP | Desired state + Atlas-owned projections; no auth/connection/verification; secrets not stored |
| OpenSpec | Real CLI provider required for governed SDD; exact project pin; Init runs `openspec init`; Repair refuses provider bootstrap; no emulation/mkdir substitute |
| Git / source control | Discovery + explicit Init local bootstrap + policy/diagnostics; no Core remote create/push/PR/merge/tag/repo-release automation |
| Skills v1 | Local Home catalog + exact pins + digests + Cursor/OpenCode projections (no marketplace UI) |
| Task Capability Discovery | Implemented (`atlas capability plan`); task-active skills + retrieval planning; technology = evidence only |
| Hooks | Engine + Cursor/OpenCode native projections; production action registry has no automatic Memory injection |
| Memory | Core + retrieval + governed persistence + diagnostics + skill; proactive = plan → CLI; native automatic injection deferred |
| Marketplace / community registry | Not implemented |
| Claude / Codex | Not activated |
| Tag / release / publish | Human approval required; no Atlas Core release publisher |

## Out of scope for this dogfooding baseline

Final V1 / production-ready claims, Atlas Context Graph engine, MCP authentication/connection/verification orchestration, secret manager, marketplace/community registry / remote skill install, Framework Skill Acquisition, Skill Creator, Atlas Native Spec Engine, additional Spec Engines, Claude/Codex activation, Core Git remote/push/PR/merge/tag/repo-release automation, embeddings, daemon/background service, Config History, generic provider plugin platform, native automatic Memory injection, new CLI commands, tag/release/publish without human approval.

**Implemented and in scope for dogfooding (do not list as missing):** Real OpenSpec provider integration, Task Capability Discovery, Hooks engine + native projections, Skills v1, MCP projections, CodeGraph provider, Memory surfaces listed above.
