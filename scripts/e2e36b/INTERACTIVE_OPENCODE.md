# OpenCode Interactive E2E — 36B.7 — PASS

Observed via real `opencode run` against the isolated 36B.7 Project A
workspace. Full transcript retained outside the repository under:

`Atlas-E2E/36b7-interactive-transcripts/opencode-interactive.jsonl`

Model used for certification run: `opencode/ling-3.1-flash-free`
(default `exo-free` was deprecated).

## Acceptance

| Criterion | Result |
|-----------|--------|
| Atlas instructions/skill discovered | PASS |
| Atlas retrieval invoked (`atlas context compose` / `atlas memory …`) | PASS |
| Memory canary `CANARY-MEM-36B7-ALPHA` recovered | PASS |
| CE canary `CANARY-CE-36B7-ALPHA.md` recovered | PASS |
| CodeGraph `ProcessPaymentAlpha` recovered | PASS |
| Provenance identified | PASS |
| No BETA leakage | PASS |
| No file mutation / no Git operations | PASS |
| Explicit retrieval; injection not claimed as automatic | PASS |

## Notes

- Automatic harness interactive row remains `SKIPPED` (does not fabricate PASS).
- This file records the authoritative OpenCode Interactive E2E **PASS** for the
  final certification matrix.
