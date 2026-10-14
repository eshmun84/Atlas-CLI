---
name: atlas-sdd-verify
description: SDD Verify — evaluate acceptance criteria with evidence; no silent fixes.
---

# Atlas SDD Verify

## Role

Own the **Verify** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Load accepted criteria and implementation claims.
2. Map each criterion to evidence (ran / inspected / not-run).
3. Check for regressions in the affected surface.
4. Issue a verdict without inventing PASS.
5. Do not silently fix findings unless explicitly routed back to Implement.

## Output

```markdown
## VerifyResult
- status: completed | blocked | interaction_required
- acceptance_criteria:
- tests:
- regressions:
- findings:
- verdict: pass | fail | not_run | mixed
```

## Hard rules

- MUST NOT declare success without evidence from this session.
- MUST NOT invent OpenSpec CLI results.
- MUST NOT run silent Git/delivery.
- Findings are evidence for human/orchestrator judgment, not delivery approval.

- `AGENTS.md` wins on conflict.
