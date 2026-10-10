---
name: code-review
description: Review code changes for correctness, clarity, and maintainability. Use for PR review, diff critique, and quality gates.
triggers:
  - review
  - code-review
  - pull request
  - diff
---

# Code Review

## Activation Contract

Use when reviewing a diff, PR, or proposed change for correctness and maintainability.

## Hard Rules

- Critique the change, not the author.
- Prefer concrete, actionable findings with file references.
- Do not approve delivery; review evidence is not authorization.
- Do not invent defects that are not supported by the diff.

## Decision Gates

1. What is the intended behavior of the change?
2. Which risks are material vs nitpicks?
3. Are tests and contracts aligned with the claim?

## Execution Steps

1. Summarize the change intent.
2. List blocking findings, then non-blocking notes.
3. Call out missing tests or contract gaps.
4. Stop with a clear verdict: approve-as-evidence / request changes / blocked.

## Output Contract

- Intent summary
- Blocking findings
- Non-blocking notes
- Explicit non-approval of delivery

## References

None required for v1.
