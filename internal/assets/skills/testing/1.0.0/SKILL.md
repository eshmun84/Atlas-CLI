---
name: testing
description: Plan and execute bounded tests for Atlas-governed changes. Use when verifying behavior, writing tests, or judging test sufficiency.
triggers:
  - test
  - testing
  - coverage
  - verify
---

# Testing

## Activation Contract

Use when the task requires verifying behavior with tests, designing a minimal test plan, or judging whether existing tests cover the change.

## Hard Rules

- Prefer the project's existing test runner and layout.
- Do not invent coverage numbers or test results.
- Keep the suite bounded to the change under review.
- Fail closed when required fixtures or runners are missing.

## Decision Gates

1. Is there an existing test command and convention?
2. What is the smallest set of cases that proves the change?
3. Are flaky or environment-dependent tests avoided?

## Execution Steps

1. Identify the changed surface and risk.
2. Select or author the minimal tests.
3. Run the relevant suite when the environment supports it.
4. Report pass/fail evidence without inventing results.

## Output Contract

- Test plan (what and why)
- Commands run (or explicit reason not run)
- Results summary with concrete evidence

## References

None required for v1.
