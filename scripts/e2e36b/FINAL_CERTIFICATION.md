# Atlas 36B.7 — Final Certification Matrix

**Baseline:** `develop @ 4b1f25e8a68635541ec5abe80b8df075f78013a7`

**Slice:** 36B.7 Global Integration E2E

**Authority:** This document is the authoritative final certification matrix.
It combines automated harness evidence with externally observed Cursor and
OpenCode interactive transcripts. The automated harness continues to mark
IDE interactive rows as `SKIPPED` when it cannot observe those sessions;
those harness rows are **not** fabricated PASS.

## Final Matrix

| Component | Result |
|-----------|--------|
| Hooks Foundation | **PASS** |
| Hooks Engine | **PASS** |
| Cursor Hooks | **PASS** |
| OpenCode Hooks | **PASS** |
| Memory Skill | **PASS** |
| Composer Core | **PASS** |
| Memory Source | **PASS** |
| Context Economy Source | **PASS** |
| CodeGraph Source | **PASS** |
| Context Compose | **PASS** |
| Project Isolation | **PASS** |
| Partial Failure | **PASS** |
| Read-only | **PASS** |
| Cursor Interactive E2E | **PASS** |
| OpenCode Interactive E2E | **PASS** |
| Security Regression | **PASS** |

## Documented limitations (not PASS/FAIL)

Path-scoped storage byte-substitution probes under a trusted project path may
be recorded as `UNVERIFIED` by the automated harness when Atlas does not fail
closed and also does not return peer canaries. These are trust-boundary
limitations of path-scoped Memory/CE/CodeGraph storage identity — they do not
override `isolation.ab` PASS (no peer leakage under normal A/B compose).

## Evidence sources

| Layer | Source |
|-------|--------|
| Automated | `go run ./scripts/e2e36b` under isolated `Atlas-E2E/36b7-integration-*` |
| Cursor interactive | External real IDE Agent session — see `INTERACTIVE_CURSOR.md` |
| OpenCode interactive | External real `opencode run` transcript — see `INTERACTIVE_OPENCODE.md` |

## Semantics preserved

- Explicit retrieval only (`atlas context compose` / Memory skill).
- No claim of native automatic context injection.
- No Memory persistence automation.
- No graph auto-build during composition.
- No production feature changes in this slice (harness + certification only).
