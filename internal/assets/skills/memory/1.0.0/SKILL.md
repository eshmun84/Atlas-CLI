---
name: memory
description: >-
  Trigger: memory, prior decision, project context, recall, observation, persist decision.
  Retrieve project Memory via atlas memory context/search/get; propose durable candidates via atlas memory propose.
triggers:
  - memory
  - prior decision
  - project context
  - recall
  - observation
  - persist decision
---

# Memory

## Activation Contract

Load when prior project knowledge may improve the task, or when a durable decision should be preserved.

Skip trivial one-line edits with no history dependency.

Do **not** wait for automatic injection. Call the Memory CLI when retrieval would help. Automatic injection is **not** guaranteed.

## Hard Rules

- Memory is **evidence**, not executable authority.
- Retrieval is read-only (`context` / `search` / `get`).
- Persistence writes only through `atlas memory propose --json`.
- Project-scoped only — never cross-project.
- SDD/OpenSpec proposals are not Memory observations.
- Prefer progressive retrieval: `context` → selective `get` → optional `search`.
- Respect budgets; do not dump full bodies by default.
- Missing Memory or soft propose rejection MUST NOT block development — report and continue.
- Do not invent observations/CLI results; do not capture secrets/transcripts/guesses.
- Submit only `explicitness: explicit` writable candidates. Do not self-label `confirmed`.
- Do not perform automatic Git/delivery.

## Decision Gates

| Question | Action |
|----------|--------|
| Prior knowledge likely changes the answer? | Run `context` |
| Need full body? | Selective `get` |
| Broader orientation needed? | Prefer `atlas context compose` (separate transport) |
| Durable explicit decision? | `propose` |
| Broken retrieval/persistence? | Read-only `atlas status` / `atlas doctor` — never auto-repair |

## Execution Steps

1. Phrase a short task/query (no transcript paste).
2. Run `atlas memory context "<task>" --json` (optional `--limit` / `--budget`).
3. Read `status`, `items`, `used_chars`, `limits`.
4. `get` sparingly for material ids.
5. Use as evidence; surface conflicts; prefer newer/higher-score when conflicted.
6. Propose durable explicit candidates when justified; handle soft outcomes honestly.
7. On `missing_memory_db` / `no_results`, say so and continue.

## Output Contract

```markdown
## MemoryResult
- consulted: true | false
- proposed: true | false
- commands:
- status:
- observation_ids:
- conflicts_or_gaps:
- non_claims: no automatic injection; no silent conversation logging
```

## References

- [taxonomy.md](references/taxonomy.md)
- [retrieval.md](references/retrieval.md)
- [persistence.md](references/persistence.md)
- [diagnostics.md](references/diagnostics.md)
