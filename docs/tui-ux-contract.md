# Atlas TUI UX Contract (Alpha 2)

This document defines the product UX contract for the Atlas CLI interactive shell.
It governs navigation, Status, and Doctor. It does not authorize new product capabilities.

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
| Help | Keyboard and route help. |
| Exit | Quit the TUI. |

**Removed:** Dashboard as an independent screen. Its overview role is absorbed by Status.

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
2. **Atlas Runtime** — initialized / materialized / contract health summary. No exhaustive runtime file lists.
3. **Source Control / Delivery Tools** — Git repo/branch/remote (`none` when empty), `gh` availability, delivery-assist preference. No Git mutations.
4. **Project Technology** — detected stack and libraries. PATH tool `go` appears only when the project stack includes Go.
5. **Adapters** — selected adapters and high-level projection health (`selected` / `materialized` / `missing` / `NOT SELECTED` for inactive adapters).
6. **Governance Tools** — workflow/spec-engine preferences and OpenSpec CLI availability (`NOT IMPLEMENTED` where Atlas does not run OpenSpec).
7. **MCP / External Context** — configured MCP entries as preferences only; connection/auth remain `NOT IMPLEMENTED`.
8. **Health** — PASS/WARNING/ERROR counts and result label from the same `doctor.Evaluate` summary as Doctor, optional compact “Needs attention” list (top warnings/errors only), Atlas Home presence (not path-primary), Context Economy state, suggested next action. Status must not show a clean PASS when Doctor has warnings or errors.

Status must **not** list full runtime artifact inventories (those belong in Doctor / Runtime Repair).

## Doctor (deep diagnostics)

Sections, in order:

1. **Overall Health** — PASS / WARNING / ERROR counts and result label.
2. **Workspace**
3. **Git**
4. **Atlas Configuration**
5. **Atlas Runtime** — includes runtime file / marker / lock / contract detail.
6. **Adapters** — projections and Atlas agent pack health.
7. **Atlas Home** — path, presence, writability, layout, assets (detail lives here, not as Status primary).
8. **Context** — Context Graph preference + Context Economy index/capsule/pack state.
9. **MCP / External Context** — configured vs connected (connected stays `NOT IMPLEMENTED` in Alpha).

Doctor remains read-only and never repairs.

## Out of scope for this contract

Init Setup full rework, storage migration, CodeGraph, real MCP materialization, marketplace, Skills v1, Git automation, new CLI commands.
