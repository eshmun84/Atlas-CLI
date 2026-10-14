---
name: testing
description: "Trigger: test, testing, coverage, verify. Plan and run minimum-sufficient tests with honest evidence."
triggers:
  - test
  - testing
  - coverage
  - verify
---

# Testing

## Activation Contract

Load when verifying behavior, writing tests, or judging whether existing tests cover a change.

## Hard Rules

- Prefer the project's existing runner and layout.
- Prefer focused tests first; expand only when risk/contract requires it.
- Never invent coverage numbers or results.
- Fail closed when required runners/fixtures are missing — say so.
- Distinguish layers: unit / integration / E2E / regression.

## Decision Gates

| Situation | Action |
|-----------|--------|
| Pure logic/parser change | Focused unit tests |
| Boundary across modules/IO | Integration test at the smallest public boundary |
| User-visible multi-component flow | Targeted E2E only if risk warrants |
| Prior bug class | Add/adjust regression coverage |
| Runner unavailable | Report not-run; do not invent PASS |

## Execution Steps

1. Identify changed behavior and risk.
2. Choose the minimum sufficient test layer.
3. Author or select focused cases first.
4. Run the narrow suite; record exact commands.
5. Expand only when residual risk requires it.
6. Report what was tested and what was not.

## Output Contract

```markdown
## TestingResult
- changed_behavior:
- risk:
- layers_used: unit | integration | e2e | regression
- commands:
- results:
- not_tested:
```

## References

None required for v1.
