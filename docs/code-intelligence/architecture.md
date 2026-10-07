# Code Intelligence architecture

Provider-neutral architecture for Atlas Code Intelligence. Provider-specific details (for example CodeGraph) live in separate docs under this directory.

## Architectural ownership

Atlas owns:

- the Code Intelligence capability;
- internal contracts;
- normalized models;
- lifecycle and conceptual states;
- integration points with Status, Doctor, and Context Economy.

External tools sit behind adapters. Upper layers (governance, Status, Doctor, Context Economy, agents) depend on Atlas models and services, not on a vendor CLI, database schema, or wire format.

## Proposed internal architecture

Conceptual layout (not implemented yet):

```
internal/codeintel/
  provider.go
  models.go
  service.go

internal/codeintel/<provider>/
  # e.g. codegraph/
```

`internal/codeintel/` holds the Atlas-owned surface. Concrete providers live under that package tree as adapters. Slice work that implements this layout must preserve replaceability: swapping a provider must not force rewrites of Status, Doctor, Context Economy, or agents.

## Provider contract

Conceptual operations every provider adapter should support over time:

| Operation | Intent |
| --- | --- |
| **Probe** | Detect executable, version, and capabilities; return Atlas capability state. |
| **Build** | Explicit graph/index construction or refresh for a project. |
| **Status** | Read-only readiness / freshness for a project. |
| **FindSymbol** | Symbol discovery against the provider index. |
| **Context** | Structural neighborhood for a symbol (callers, callees, related files). |
| **Impact** | Impact estimate for a symbol. |
| **DiffImpact** | Impact estimate for current project changes. |

This table is architectural guidance. A definitive Go `Provider` interface is deferred until implementation; names and signatures may tighten then without changing ownership boundaries.

## Provider lifecycle

Conceptual states:

- `missing` — no configured / expected provider data yet.
- `available` — executable present and probeable.
- `ready` — installed, compatible, and index/state usable.
- `stale` — present but out of date relative to project sources.
- `incompatible` — executable found but version/capabilities fail policy.
- `unavailable` — not installed, removed, or otherwise not usable.
- `error` — probe/execution failed in a reportable way.

Atlas must degrade cleanly when no provider is present: Code Intelligence stays off; the rest of Atlas continues.

## Mutation policy

- **Status** is read-only.
- **Doctor** is read-only.
- **Startup** is read-only.
- **Discovery** is read-only.

None of the above may build or refresh a provider index.

Graph/index **build** and **update** are explicit mutations under Atlas control. No silent rebuilds. No hidden mutations during diagnostics. If a provider supports incremental update, Atlas still decides *when* that mutation runs.

## Storage policy

Provider runtime data belongs under Atlas Home, scoped by project identity:

```text
$ATLAS_HOME/projects/<project-id>/<provider>/
```

Examples: `…/codegraph/`, or a future `…/<other-provider>/`.

Do **not** store provider databases or caches in the product repository unless a future explicit decision says otherwise. Portable project files under `.atlas/` must not gain machine-local absolute paths for provider state.

## Security

Foundation expectations:

- local-first;
- no remote embeddings;
- no implicit external LLM enrichment;
- no sending project code outside the developer environment as part of this capability;
- safe execution of external processes (explicit argv, cancellation, timeouts, bounded stdout/stderr);
- provider failures mapped to Atlas-owned errors and states.

Structural analysis is evidence, not absolute truth. Atlas continues to rely on compilers/runtimes where applicable, tests, architecture rules, framework knowledge, and human review.

## Context Economy relationship

Code Intelligence **complements** Context Economy; it does not replace it.

Conceptual flow:

```text
Task
  → Context Economy
  → Code Intelligence
  → relevant symbols / files / dependencies
  → agent
```

Context Economy remains the explicit file-based update surface. Code Intelligence may inform what is relevant; packing and Home-backed context artifacts stay under Context Economy policy.

## Atlas Context Graph relationship

**Code Intelligence ≠ Atlas Context Graph.**

Atlas Context Graph remains a separate future capability. Product language in upper layers should say “Code Intelligence,” not promote a specific provider name as a domain concept, and must not treat provider graphs as the Atlas Context Graph.

## Replaceability

The first provider may be CodeGraph, but it is not a permanent core dependency.

Architecture must allow later providers (for example SCIP-based tooling, Sourcegraph-compatible intelligence, or an Atlas-native implementation) without rewriting governance, Status, Doctor, Context Economy, or agents. Adapters absorb vendor churn; Atlas models stay stable.
