---
name: documentation
description: "Trigger: docs, documentation, readme, explain. Write accurate project docs; keep Atlas runtime identity out by default."
triggers:
  - docs
  - documentation
  - readme
  - explain
---

# Documentation

## Activation Contract

Load when writing or updating project documentation, operator notes, or product-facing prose.

## Hard Rules

- Prefer truth over aspiration; do not invent features.
- Preserve project terminology.
- Update only docs affected by the change.
- Distinguish project docs from Atlas-owned governance docs.
- **Project identity boundary:** project-facing docs describe the project, not Atlas.
  MUST NOT introduce by default: Atlas/Cursor/OpenCode identity, workflow, spec engine, Atlas Home, runtime paths, Memory/Context/CodeGraph internals, hooks, governance state, delivery config, or agent metadata.
  Exception: the human explicitly asks to document those surfaces.

## Decision Gates

| Question | Action |
|----------|--------|
| Audience + decision? | Write only what that decision needs |
| Already documented elsewhere? | Link; do not duplicate |
| Claim verified in codebase? | Keep; else mark unknown |
| Atlas runtime detail? | Omit unless explicitly requested |

## Execution Steps

1. Identify audience and required decision.
2. Draft the minimal accurate section.
3. Cross-check against live code/config.
4. Remove speculative or out-of-scope Atlas leakage.

## Output Contract

```markdown
## DocumentationResult
- audience:
- purpose:
- updated_paths:
- unknowns:
```

## References

None required for v1.
