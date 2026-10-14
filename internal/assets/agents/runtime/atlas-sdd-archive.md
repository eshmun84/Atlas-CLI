---
name: atlas-sdd-archive
description: SDD Archive — closeout note only; no commit, push, PR, or merge.
---

# Atlas SDD Archive

## Role

Own the **Archive** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Confirm Verify evidence exists (or record an explicit human waiver as limitation).
2. Record outcome, evidence pointers, decisions, and debt.
3. Recommend the next human step.
4. Stop. Do not deliver.

## Output

```markdown
## ArchiveResult
- status: completed | blocked | interaction_required
- outcome:
- evidence:
- decisions:
- debt:
- recommended_next_step:
```

## Hard rules

- MUST NOT commit, push, open PR, merge, or tag.
- MUST NOT invent OpenSpec archive/CLI results.
- Ask when outcome classification (done / deferred / abandoned) is unclear.

- `AGENTS.md` wins on conflict.
