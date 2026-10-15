---
name: atlas-sdd-propose
description: SDD Propose — Atlas governance + OpenSpec provider workflow; await acceptance.
---

# Atlas SDD Propose

## Role

Own the **Propose** phase under `.atlas/contracts/sdd-openspec.md`.
Atlas owns governance gates; OpenSpec owns change scaffolding/instructions/validation.

## Procedure

1. Confirm OpenSpec provider is ready (CLI + project pin + initialized project). If not, stop blocked.
2. Load Explore/Research outputs and governance config.
3. Prefer OpenSpec projected workflow (`/opsx-propose` or `openspec-propose` skill) when available.
4. Otherwise, with OpenSpec CLI:
   - `openspec new change "<change-id>"` (scaffold only)
   - `openspec status --change "<change-id>" --json`
   - `openspec instructions <artifact> --change "<change-id>" --json`
   - Author artifacts exactly per OpenSpec instructions/templates (do not invent headers)
5. Ask for explicit human acceptance before Implement.
6. Stop. Do not implement.

## Output

```markdown
## ProposeResult
- status: completed | blocked | interaction_required
- change_id:
- problem:
- scope:
- acceptance_criteria:
- approach:
- artifacts:
- acceptance_required: true
- recommended_next_phase: implement | update | stop
```

## Hard rules

- MUST NOT implement before acceptance.
- MUST NOT silently widen scope.
- MUST NOT claim approval that was not granted.
- MUST NOT invent OpenSpec CLI results or emulate OpenSpec layouts.
- MUST NOT run silent Git/delivery.
- `AGENTS.md` wins on conflict.
