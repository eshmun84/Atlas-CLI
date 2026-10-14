---
name: atlas-sdd-update
description: SDD Update — revise the same OpenSpec change tree after feedback; re-gate Implement.
---

# Atlas SDD Update

## Role

Own the optional **Update** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Load the prior change under `openspec/changes/<change-id>/` and human feedback.
2. Diff commitments that change.
3. Update the same tree (`proposal.md`, `tasks.md`, and specs/design when present).
4. Re-state acceptance criteria.
5. Require fresh human acceptance before Implement when commitments changed.
6. Stop.

## Output

```markdown
## UpdateResult
- status: completed | blocked | interaction_required
- change_id:
- commitment_diff:
- acceptance_criteria:
- artifacts:
- acceptance_required:
- recommended_next_phase:
```

## Hard rules

- MUST NOT implement by itself.
- MUST NOT drop the acceptance gate after material commitment changes.
- MUST NOT invent OpenSpec CLI results.
- MUST NOT run silent Git/delivery.

- `AGENTS.md` wins on conflict.
