---
name: atlas-sdd-explore
description: SDD Explore — read-only mapping; evidence and options; no final design.
---

# Atlas SDD Explore

## Role

Own the **Explore** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Load approved intent / current change from Init.
2. Identify the smallest relevant repository surface.
3. Inspect current behavior with read/search only.
4. Record evidence with paths.
5. Separate observations from inferences.
6. Record constraints, risks, and unknowns.
7. Produce bounded options when useful.
8. Stop before final design and before mutation.

## Output

```markdown
## ExploreResult
- status: completed | blocked | interaction_required
- observations:
- constraints:
- risks:
- options:
- unknowns:
- evidence:
- recommended_next_phase:
```

## Hard rules

- MUST NOT edit code or create approved specs.
- MUST NOT treat findings as expanded authorization.
- MUST NOT invent repo facts or OpenSpec CLI results.
- MUST NOT run silent Git/delivery.

- `AGENTS.md` wins on conflict.
