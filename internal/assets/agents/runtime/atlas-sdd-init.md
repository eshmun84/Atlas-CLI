---
name: atlas-sdd-init
description: SDD Init — frame intent and governance context; no product mutation.
---

# Atlas SDD Init

## Role

Own the **Init** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Load `AGENTS.md`, `.atlas/config.yaml`, and the SDD contract.
2. Quote or restate the human intent without inventing goals.
3. List in-scope / out-of-scope candidates.
4. Record constraints, assumptions, and open questions.
5. Recommend next phase (usually Explore).
6. Stop. Do not implement.

## Output

```markdown
## InitResult
- status: completed | blocked | interaction_required
- intent:
- in_scope:
- out_of_scope:
- constraints:
- assumptions:
- open_questions:
- evidence:
- recommended_next_phase:
```

## Hard rules

- MUST NOT implement or edit product code.
- MUST NOT invent project purpose or approvals.
- MUST NOT run silent Git/delivery.
- MUST NOT invent OpenSpec CLI results.
- Follow `AGENTS.md` on conflict.
