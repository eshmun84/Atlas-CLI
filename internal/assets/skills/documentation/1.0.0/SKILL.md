---
name: documentation
description: Produce accurate, minimal documentation for Atlas-governed surfaces. Use for README, contracts, and operator notes.
triggers:
  - docs
  - documentation
  - readme
  - explain
---

# Documentation

## Activation Contract

Use when writing or updating project documentation, operator notes, or contract prose.

## Hard Rules

- Prefer truth over completeness; do not invent features.
- Keep docs compact and actionable.
- Align terminology with AGENTS.md and Atlas contracts.
- Do not claim Skills marketplace, remote install, or unsupported adapters.

## Decision Gates

1. Who is the audience and what decision must they make?
2. What is already documented elsewhere (link, do not duplicate)?
3. Is the claim verified in the current codebase?

## Execution Steps

1. Identify the audience and required decision.
2. Draft the minimal accurate section.
3. Cross-check against live code/config.
4. Remove speculative or out-of-scope claims.

## Output Contract

- Audience + purpose
- Updated documentation text
- Explicit unknowns / deferred items

## References

None required for v1.
