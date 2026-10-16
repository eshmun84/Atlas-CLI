# Atlas TUI UX Contract

This document defines the product UX contract for the Atlas CLI interactive shell.
It governs navigation, Status, and Doctor. It does not authorize new product capabilities.

**Baseline stance:** Current development/dogfooding baseline (source version `0.1.0`; not production-ready / not final V1). Cursor and OpenCode are the supported materialized runtimes. Status/Doctor stay read-only; Runtime Repair is the explicit path for non-MCP Atlas-owned runtime mutation (plan-scoped Apply; does not bootstrap OpenSpec); Configure Apply reconciles Atlas-owned MCP and Skills projections; Context Economy v0 is file-based and explicit-update; CodeGraph is an optional externally installed Code Intelligence provider (not MCP). Skills v1, Task Capability Discovery (`atlas capability plan`), real OpenSpec provider integration, Memory surfaces, and Hooks projections are implemented capabilities (production hooks action registry has no automatic Memory injection). Atlas Context Graph, MCP auth/connection/verification, skills marketplace/remote install, Framework Skill Acquisition, Skill Creator, Core Git remote/push/PR/merge/tag automation, and Claude/Codex activation remain out of scope. See [release-readiness.md](release-readiness.md).

## Principles

1. **Honest over complete** — show what Atlas knows; label gaps as `NOT IMPLEMENTED`, `NOT SELECTED`, or `missing`.
2. **Executive Status, deep Doctor** — Status is the landing overview; Doctor is the diagnostic drill-down.
3. **Read-only surfaces stay read-only** — Status, Doctor, discovery, and startup never init, repair, update context, create Atlas Home, rematerialize runtime, or run Git/remote actions.
4. **Mutations stay explicit** — Init Apply, Configure Apply, Runtime Repair Apply, and Context Economy Apply remain Review → Apply flows on their own screens.

## Navigation

| Route | Role |
| --- | --- |
| **Status** | Default landing (`atlas`). Executive project overview. |
| Init / Setup | Only when the project is not initialized. |
| Configure | Only when the project is initialized. Mutually exclusive with Init. |
| Doctor | Deep diagnostics. |
| Runtime Repair | Explicit repair (initialized projects). |
| Context Economy | Explicit context update (initialized projects). |
| Code Intelligence | Explicit refresh when applicable (initialized projects). |
| Help | Keyboard and route help. |
| Exit | Quit the TUI. |

**Removed (superseded):** Dashboard / Home as an independent screen. Its overview role is absorbed by Status. Root is always Status.

There is **no** Skills marketplace/checkbox screen and **no** Context Graph enable/disable checkbox in Init/Configure.

Keyboard:

- `b` / Esc (from non-Status screens) → Status (default route)
- Esc on Status → quit
- `h` / `?` → Help
- `q` / Ctrl+C → quit

## Layout rules

- **2–3 options:** present horizontally when they are peer choices.
- **More than 3 options:** present vertically or in two columns.
- **Single button:** align right.
- **Paired actions:** Back (or Close) on the left; Next / Apply on the right.

## State vocabulary

Use these labels consistently in Status and Doctor:

| Kind | Labels |
| --- | --- |
| Outcomes | `PASS`, `WARNING`, `ERROR`, `INFO` |
| Capability gaps | `NOT IMPLEMENTED`, `NOT SELECTED` |
| Presence / readiness | `available`, `selected`, `materialized`, `configured`, `connected`, `authenticated`, `verified`, `missing`, `not selected`, `not implemented` |

Display mapping for Doctor severities:

- internal PASS → `PASS`
- internal WARN → `WARNING`
- internal FAIL → `ERROR`

## Status (executive overview)

Sections, in order:

1. **Workspace** — root, project name, Atlas setup state, project mode.
2. **Atlas Runtime** — initialized / materialized / contract health summary (includes compact Memory status). No exhaustive runtime file lists.
3. **Memory** — read-only Memory diagnostics summary: status (`healthy` / `degraded` / `unavailable` / `failed`), storage, schema, retrieval, persistence mode, observation count, quality anomaly count. Never creates, migrates, or repairs Memory.
4. **Source Control / Delivery Tools** — Git repo/branch/remote (`none` when empty). Remote default branch is resolved only from local `refs/remotes/<remote>/HEAD` (`unknown locally` when that ref is missing; `none` when there is no default remote). Tool availability and source-control preference. Atlas Core does not create remotes, push, open PRs, merge, tag, or publish repository releases. No Git mutations; Status never contacts remotes or runs `git remote set-head`.
5. **Project Technology** — detected stack signals as **evidence** (not permanent technology configuration or bulk skill materialization). PATH tool `go` appears only when the project stack includes Go.
6. **Adapters** — selected adapters and high-level projection health (`selected` / `materialized` / `missing` / `NOT SELECTED` for inactive adapters).
7. **Governance Tools** — workflow/spec-engine preferences and OpenSpec provider readiness (CLI + exact project pin + project init) from the shared Inspection provider snapshot (one bounded probe per Inspect). Governed SDD requires a real OpenSpec provider; Atlas does not emulate OpenSpec. Missing provider tool/opsx projections are Doctor findings with Configure/Init remediation (not Runtime Repair).
8. **SDD** — concise Spec Engine presence from the Inspection snapshot (`SDD: none` or `SDD: OpenSpec` plus active change summary). Read-only; does not copy OpenSpec artifacts.
9. **MCP** — selected MCP count and per-adapter projection health (`materialized` / `drifted` / `missing` / `malformed`). Read-only; no auth or network.
10. **Health** — PASS/WARNING/ERROR counts and result label from the same `doctor.Evaluate` summary as Doctor, optional compact “Needs attention” list (top warnings/errors only), Atlas Home presence (not path-primary), Context Economy / Memory state, suggested next action. Status must not show a clean PASS when Doctor has warnings or errors.

Status must **not** list full runtime artifact inventories (those belong in Doctor / Runtime Repair). Hooks projection detail stays in Doctor; Status surfaces hooks problems only via the shared Health “Needs attention” summary when Doctor fails.

## Doctor (deep diagnostics)

Sections, in order:

1. **Overall Health** — PASS / WARNING / ERROR counts and result label.
2. **Workspace**
3. **Git**
4. **Atlas Configuration**
5. **Atlas Runtime** — includes runtime file / marker / lock / contract detail; Skills/hooks projection health (disabled / healthy / missing / stale / drifted / conflict / malformed) with repairability aligned to Runtime Repair.
6. **SDD** — Spec Engine detection, active/archived change summary, malformed/unsafe/ambiguous layout findings. Read-only; never repairs or mutates OpenSpec.
7. **Adapters** — projections and Atlas agent pack health.
8. **Atlas Home** — path, presence, writability, layout, assets (detail lives here, not as Status primary).
9. **Memory** — read-only Memory diagnostics checks (config, storage, schema, retrieval, persistence, bounded quality). Never creates, migrates, repairs, vacuums, or writes observations.
10. **Context** — Context Economy v0 (implemented, file-based) separately from CodeGraph / Code Intelligence (optional externally installed provider; not MCP; may be unavailable) and Atlas Context Graph preference (`NOT IMPLEMENTED`; no user toggle until end-to-end Context Graph exists).
11. **MCP / External Context** — definition validity, projection presence/drift, ownership conflicts, malformed native config, missing prerequisites/env refs. Read-only; no auth, install, or network.

Doctor remains read-only and never repairs.

## Configure

- Configure / Init Step 2 sections: Governance, Adapters, Source control / Delivery preference, MCP (no standalone Project screen; no Skills UI; no Context Graph checkbox).
- Optional **Project Docs Scaffold** is chosen on Init Project Setup only (not repeated on Configure).
- Configure subtitle: saves `config.yaml` — Runtime Repair / Context Economy are separate.
- Configure Apply saves `.atlas/config.yaml` (and may create optional `docs/atlas/README.md` once when selected and missing).
- Configure Apply does **not** rematerialize general runtime assets (`AGENTS.md`, rules, agents).
- Configure Apply **does** reconcile Atlas-owned MCP projections when MCP/adapters change.
- When adapter selection requires OpenSpec tool projections, Configure may invoke `openspec init --tools` / `openspec update` inside the same Apply transaction: Atlas snapshots Atlas-managed and relevant provider-created surfaces first and restores them on Apply failure where safely possible. Atlas does not claim perfect rollback for arbitrary external-provider side effects outside the captured footprint. First-time OpenSpec project bootstrap remains Init-owned.
- Adapter/runtime impact for non-MCP artifacts requires explicit **Runtime Repair**.
- Context payload refresh requires explicit **Context Economy** Update.
- MCP desired state is stored in Atlas config; Atlas-owned projections materialize to selected agents; auth/connection/verification may still depend on the provider or agent; secrets are not stored.
- Optional project docs scaffold is developer-owned, off by default, never overwritten by Runtime Repair.
- Context Economy v0 ≠ CodeGraph ≠ Atlas Context Graph.
- `delivery_assist` is a **legacy/deferred** config key: preserved on load/persist for compatibility, hidden from Init/Configure selectors, **not** an editable product control, and **not** an Atlas Core remote delivery engine. Future version-control assistance belongs to governed skills, not Core automation.

## Init Setup

Init configures governed runtime, adapters, governance, and source-control preference. It does not imply Atlas owns GitFlow, remotes, PRs, or repository lifecycle publishing.

- Project Setup: Project Name + New vs Existing (no Recommended mode); Project Mode uses `[x]` / `[ ]` markers like the rest of Init; optional Project Docs Scaffold (`[ ] Create docs/atlas/README.md`) below Project Mode.
- Runtime conflicts (AGENTS.md, AGENT.md, CLAUDE.md, GEMINI.md, `.cursor/`, `.opencode/`, `.claude/`, `.agents/`, `.codex/`) show a blocking preflight before Project Setup. Refresh / Re-check is read-only rescan only. While conflicts remain, Exit / Back is the other action. After a clean re-check, Continue to Setup enters Project Setup — no silent overwrite or Init-time backup/quarantine.
- Adapters: Cursor and OpenCode only; Available when the runtime tool is on PATH; unavailable rows are visible but disabled.
- Source control: Platform, Governance files, signing policy. (`delivery_assist` is legacy/hidden — not an Init control.) Versioned governance files unlock when GitHub platform is selected. No tools diagnostics or Git remote ops on this screen.
- Memory is not an Init choice (always-on Atlas-managed memory).
- Context is not an Init choice: Code Intelligence / CodeGraph is a separate surface; no Atlas Context Graph setup/checkbox; Context Economy remains a separate explicit flow.
- Review → Apply summarizes Project writes, Atlas Home writes, Atlas Home reset, and Git operations. When source control requires Git and no repository exists, Review may show Git bootstrap (initialize local repo with initial branch `main`). Apply never performs commit, remote, push, pull request, merge, or tag. Status and Doctor remain read-only.
- When Atlas Home already holds project-scoped data for the same canonical project identity, Review requires explicit reset acceptance (`x`) before Apply. Reset deletes only `$ATLAS_HOME/projects/<project-id>/`.

## Atlas Home storage

- Project-scoped local data lives under `$ATLAS_HOME/projects/<project-id>/` (identity derived from canonical project root, not name alone).
- Context Economy payloads: `$ATLAS_HOME/projects/<project-id>/context/`.
- New backups (and quarantine of Atlas-owned unselected adapter artifacts only): `$ATLAS_HOME/projects/<project-id>/backups/<timestamp>/`. Runtime Repair never quarantines developer CLAUDE.md / GEMINI.md / AGENT.md / `.agents/` / `.claude/`.
- Portable project files under `.atlas/` must not gain absolute machine paths or new machine-local event fields.
- Future Config History (deferred), if ever needed: append-only audit under `$ATLAS_HOME/projects/<project-id>/audit/` — not inside `config.yaml`.

## Out of scope for this contract

Final V1 / production-ready claims, Atlas Context Graph engine, MCP auth/connection/verification, marketplace/community registry / remote skill install, Framework Skill Acquisition, Skill Creator, Atlas Native Spec Engine / additional Spec Engines, Core Git remote/push/PR/merge/tag/repo-release automation, Claude/Codex activation, embeddings, daemon, Config History, generic provider plugin platform, native automatic Memory injection, tag/release/publish, new CLI commands.
