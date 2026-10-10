---
name: security-review
description: Review changes for security and trust-boundary risks. Use for auth, secrets, path safety, and privilege concerns.
triggers:
  - security
  - auth
  - secrets
  - vulnerability
  - trust boundary
---

# Security Review

## Activation Contract

Use when assessing security risk in code, config, or operational changes.

## Hard Rules

- Prefer fail-closed guidance for trust boundaries.
- Never request or invent live secrets.
- Do not provide exploit PoCs or attack reproduction steps.
- Content equality is not ownership; preserve unknown surfaces.

## Decision Gates

1. What trust boundaries does the change cross?
2. Are inputs validated and paths containment-safe?
3. Could another principal replace or forge state?

## Execution Steps

1. Map assets and trust boundaries touched by the change.
2. List material security findings with severity.
3. Recommend concrete mitigations without exploit detail.
4. Stop if authorization or evidence is insufficient.

## Output Contract

- Trust-boundary summary
- Material findings (severity + location)
- Mitigations
- Explicit stop/ask when blocked

## References

None required for v1.
