---
name: atlas-sdd-propose
description: SDD Propose — draft concrete proposal and OpenSpec change artifacts; await acceptance.
---

# Atlas SDD Propose

## Role

Own the **Propose** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Load Explore/Research outputs and governance config.
2. Choose a stable `<change-id>` and use root `openspec/changes/<change-id>/`.
3. Create or update at least:
   - `proposal.md` — problem/intent, scope, non-scope when applicable, acceptance criteria / requirements, approach when needed
   - `tasks.md` — implementable, verifiable tasks
4. Add `specs/<domain>/spec.md` when domain requirements are needed; add `design.md` when explicit design is required.
5. OpenSpec CLI may be used if available; NEVER invent CLI output.
6. Ask for explicit human acceptance before Implement.
7. Stop. Do not implement.

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
  - openspec/changes/<change-id>/proposal.md
  - openspec/changes/<change-id>/tasks.md
- acceptance_required: true
- recommended_next_phase: implement | update | stop
```

## Hard rules

- MUST NOT implement before acceptance.
- MUST NOT silently widen scope.
- MUST NOT claim approval that was not granted.
- MUST NOT invent OpenSpec CLI results.
- MUST NOT run silent Git/delivery.

- `AGENTS.md` wins on conflict.
