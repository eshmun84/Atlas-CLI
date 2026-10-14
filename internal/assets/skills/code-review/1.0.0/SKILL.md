---
name: code-review
description: "Trigger: review, code-review, pull request, diff. Review for correctness and regressions; evidence not approval."
triggers:
  - review
  - code-review
  - pull request
  - diff
---

# Code Review

## Activation Contract

Load when reviewing a diff, PR, or proposed change for correctness and maintainability.

## Hard Rules

- Critique the change, not the author.
- Prefer real defects over style noise.
- Do not invent unsupported defects.
- Review evidence is not delivery authorization.

## Decision Gates

| Question | Focus |
|----------|-------|
| Intended behavior clear? | Restate before judging |
| Correctness/regression risk? | Blocking findings first |
| Contract/error/state gaps? | Prefer concrete locations |
| Tests/evidence aligned? | Call out missing proof |

## Execution Steps

1. Summarize intended behavior.
2. Inspect correctness, regressions, contracts, error/state handling, concurrency when relevant, maintainability, tests.
3. List findings with the shape below (blocking first).
4. Stop with an evidence verdict — never delivery approval.

## Output Contract

```markdown
## CodeReviewResult
- intent_summary:
- findings:
  - severity:
    location:
    issue:
    impact:
    evidence:
    recommended_correction:
- verdict_as_evidence: approve-as-evidence | request-changes | blocked
```

## References

None required for v1.
