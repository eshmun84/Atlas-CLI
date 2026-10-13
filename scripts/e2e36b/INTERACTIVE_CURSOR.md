# Cursor Interactive E2E — 36B.7 — PASS

Observed in a real Cursor IDE Agent session against the isolated 36B.7
Project A workspace (external to Atlas-CLI; transcript retained outside the
repository).

## Acceptance

| Criterion | Result |
|-----------|--------|
| Atlas context retrieval executed | PASS |
| Memory canary `CANARY-MEM-36B7-ALPHA` recovered | PASS |
| CE canary `CANARY-CE-36B7-ALPHA.md` recovered | PASS |
| CodeGraph `ProcessPaymentAlpha` at `src/payments.go:4` recovered | PASS |
| Provenance identified (Memory / CE / CodeGraph) | PASS |
| No BETA leakage (`CANARY-*-BETA`, `ProcessPaymentBeta`) | PASS |
| No file mutation | PASS |
| No Git operations | PASS |
| Explicit retrieval only; no automatic injection claim | PASS |

## Notes

- Automatic harness interactive row remains `SKIPPED` (cannot observe IDE).
- This file records the authoritative Cursor Interactive E2E **PASS** for the
  final certification matrix.
