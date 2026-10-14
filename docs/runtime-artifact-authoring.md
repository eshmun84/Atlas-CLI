# Atlas Runtime Artifact Authoring Standard

Normative guide for authors of Atlas runtime content consumed by LLMs.

This document is for Atlas developers. It is **not** materialized into customer projects.

## Purpose

Runtime artifacts are **instruction contracts for LLMs**, not essays, tutorials, or product marketing.

They must maximize operational signal and minimize repeated context.

## Surfaces covered

- `AGENTS.md` sources (`agents/base.md`, adapter blocks)
- Orchestrator and phase agents
- Review agents and worker
- `SKILL.md` packages
- Adapter entrypoints (Cursor rules, OpenCode projections)
- Operational contracts (for example SDD/OpenSpec)
- Registries only insofar as they teach agents what exists

## Core norms

1. Prefer imperative verbs: Classify, Resolve, Load, Stop, Report.
2. Prefer observable conditions over vague judgment language.
3. Make branches explicit (`If … then … else …`).
4. Make stop conditions explicit.
5. Make required outputs explicit.
6. Prefer positive procedure alongside important prohibitions.
7. Use MUST / MUST NOT / MAY only when precision requires it.
8. Put depth in `references/` (or the authoritative contract), not in every hot-path file.
9. Own each rule once at the most appropriate layer. Repeat only for safety-critical authority.
10. Do not invent skills, agents, providers, tools, evidence, or CLI results.

## Recommended shape (use only what the artifact needs)

Not every artifact must use every section. Prefer the minimum that makes the contract executable:

1. Frontmatter (agents/skills when required by the adapter)
2. Role / purpose
3. Activation or when-to-use
4. Hard rules
5. Decision gates
6. Execution procedure
7. Output contract
8. References

Avoid symmetric boilerplate “Relation to X” sections when `AGENTS.md` or the SDD contract already owns that rule.

## Density guidance

Guidelines, not hard byte quotas:

| Surface | Guidance |
|---------|----------|
| Skill hot-path body | Target ~180–450 tokens; prefer `references/` beyond that; avoid >~1000 without strong reason |
| Phase agents | Prefer ~40–60 lines when responsibility allows |
| `AGENTS.md` | Thin constitutional/router surface |
| Adapter entrypoints | Pointers only; never duplicate full AGENTS |

## Output contracts

When another agent or the orchestrator consumes a result, define a stable Markdown shape with fixed headings/fields.

Do not introduce JSON solely for fashion. Use machine serialization only when a real code consumer parses it.

## Project documentation identity boundary

Project-facing documentation describes the **project**, not Atlas governance runtime.

MUST NOT introduce by default: Atlas/Cursor/OpenCode identity, workflow/spec engine, Atlas Home, runtime paths, Memory/Context/CodeGraph internals, hooks, governance state, delivery config, or agent metadata.

Exception: the user explicitly asks to document those surfaces.

## Governance routing language

MUST NOT teach escape hatches such as:

- inline work by default for material product changes
- planning optional when configured governance requires SDD
- SDD only if the user names OpenSpec/planning/spec

Configured `governance.workflow` and `governance.spec_engine` are authoritative for material work.

## Delivery and signing language

Teach clearly:

- Cursor/OpenCode = execution adapter
- GitHub/etc. = hosting provider
- `origin` = Git remote name
- Cursor Origin = separate product; never inferred from adapter or the word `origin`

When signing is required: signing failure stops; no unsigned equivalent fallback; verify successful signatures.

## Progressive disclosure

Top-level artifact: only what the agent needs immediately.
References: taxonomy, edge cases, diagnostics, long examples.

Align with Context Economy: load deeper material only when the task needs it.

## Quality checklist

- [ ] Imperative and testable rules
- [ ] Decision gates cover real forks only
- [ ] Failure semantics are fail-closed for REQUIRED guarantees
- [ ] Output contract is explicit
- [ ] No duplicated authority paragraphs without need
- [ ] No brand/persona voice
- [ ] No invented capabilities
