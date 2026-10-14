---
name: atlas-sdd-implement
description: SDD Implement — mutate only accepted scope; stop on material conflict.
---

# Atlas SDD Implement

## Role

Own the **Implement** phase under `.atlas/contracts/sdd-openspec.md`.

## Preconditions

Before any product mutation, verify:

1. approved `<change-id>` exists under `openspec/changes/<change-id>/`;
2. `proposal.md` and `tasks.md` exist and sufficiently identify intent, scope, acceptance criteria, and tasks;
3. `specs/<domain>/spec.md` exists when domain requirements apply; `design.md` when explicit design was required;
4. human acceptance of the proposal/design is present when required;
5. implementation scope is explicit.

If any precondition fails → STOP (`blocked` / `interaction_required`). Do not implement.

## Procedure

1. Confirm preconditions against the change tree (not a vague “compatible artifacts” claim alone).
2. Implement only accepted scope.
3. Run focused validation for the changed surface.
4. Report files, tests, deviations, and remaining risks.
5. On material conflict with acceptance, stop and ask (do not silently re-scope).

## Output

```markdown
## ImplementResult
- status: completed | blocked | interaction_required
- change_id:
- scope_completed:
- files_changed:
- tests:
- validation:
- deviations:
- remaining_risks:
```

## Hard rules

- MUST NOT expand scope or “while here” refactor.
- MUST NOT invent test results or OpenSpec CLI results.
- MUST NOT run silent Git/delivery.

- `AGENTS.md` wins on conflict.
