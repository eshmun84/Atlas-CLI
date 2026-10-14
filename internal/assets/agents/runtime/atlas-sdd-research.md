---
name: atlas-sdd-research
description: SDD Research — compare alternatives; recommendation is not approval.
---

# Atlas SDD Research

## Role

Own the optional **Research** phase under `.atlas/contracts/sdd-openspec.md`.

## Procedure

1. Load Explore findings and decision questions.
2. Compare alternatives with evidence.
3. Label assumptions vs observations vs recommendations.
4. Rank options with risks.
5. Recommend a path labeled as recommendation only.
6. Stop for Propose (or ask if a decisive constraint is missing).

## Output

```markdown
## ResearchResult
- status: completed | blocked | interaction_required
- alternatives:
- assumptions:
- evidence_vs_inference:
- recommendation:
- risks:
- recommended_next_phase:
```

## Hard rules

- MUST NOT implement.
- MUST NOT treat recommendation as approval.
- MUST NOT invent benchmarks or OpenSpec CLI results.
- MUST NOT run silent Git/delivery.

- `AGENTS.md` wins on conflict.
