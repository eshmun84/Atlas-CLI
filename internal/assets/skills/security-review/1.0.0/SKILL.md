---
name: security-review
description: "Trigger: security, auth, secrets, vulnerability, trust boundary. Review trust boundaries without inventing vulns."
triggers:
  - security
  - auth
  - secrets
  - vulnerability
  - trust boundary
---

# Security Review

## Activation Contract

Load when assessing security risk in code, config, or operational changes.

## Hard Rules

- Prefer fail-closed guidance at trust boundaries.
- Never request or invent live secrets.
- Do not provide exploit PoCs or attack reproduction steps.
- Do not invent vulnerabilities without evidence.
- Classify each issue: confirmed | likely | needs-verification.

## Decision Gates

| Surface | Check |
|---------|-------|
| Authn/authz | Enforced on the trusted boundary, not UI-only |
| Input | Validated/parameterized before sinks |
| Secrets | Not hardcoded or logged |
| Filesystem | Path containment; no traversal |
| Process/network | Least privilege; no unexpected egress |
| Persistence | Safe defaults; no forged state |

## Execution Steps

1. Map assets and trust boundaries touched.
2. List material findings with severity and evidence class.
3. Recommend mitigations without exploit detail.
4. Stop if authorization/evidence is insufficient.

## Output Contract

```markdown
## SecurityReviewResult
- trust_boundaries:
- findings:
  - severity:
    location:
    issue:
    impact:
    evidence:
    evidence_class: confirmed | likely | needs-verification
    recommended_correction:
- blockers:
```

## References

None required for v1.
