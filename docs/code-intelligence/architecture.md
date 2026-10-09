# Code Intelligence architecture

Provider-neutral architecture for Atlas Code Intelligence. Provider-specific details (for example CodeGraph) live in separate docs under this directory.

## Architectural ownership

Atlas owns:

- the Code Intelligence capability;
- internal contracts;
- normalized models;
- lifecycle and freshness states;
- integration points with Status, Doctor, and Context Economy.

External tools sit behind adapters. Upper layers (governance, Status, Doctor, Context Economy, agents) depend on Atlas models and services, not on a vendor CLI, database schema, or wire format.

## Internal architecture

Implemented layout:

```
internal/codeintel/
  provider.go      # provider boundary
  models.go
  service.go
  lifecycle.go
  fingerprint.go
  containment.go
  storage.go
  metadata.go
  plan.go
  refresh.go

internal/codeintel/codegraph/
  # CodeGraph CLI adapter (optional external provider)
```

`internal/codeintel/` holds the Atlas-owned surface. Concrete providers live under that package tree as adapters. Swapping a provider must not force rewrites of Status, Doctor, Context Economy, or agents.

## Provider contract

Operations the provider boundary supports:

| Operation | Intent |
| --- | --- |
| **Probe** | Detect executable, version, and capabilities; return Atlas capability state. |
| **Refresh** | Explicit graph/index construction or refresh for a project (initial / incremental / noop / full). |
| **Status** | Read-only readiness / freshness for a project. |

Status and Doctor consume read-only health. Refresh mutations are explicit and gated by Atlas.

## Provider lifecycle

States in use:

- `missing` — no configured / expected provider data yet.
- `available` — executable present and probeable.
- `ready` — installed, compatible, and index/state usable.
- `stale` — present but out of date relative to project sources.
- `incompatible` — executable found but version/capabilities fail policy.
- `unavailable` — not installed, removed, or otherwise not usable.
- `error` — probe/execution failed in a reportable way.

Atlas degrades cleanly when no provider is present: Code Intelligence stays off; the rest of Atlas continues.

## Mutation policy

- **Status** is read-only.
- **Doctor** is read-only.
- **Startup** is read-only.
- **Discovery / Inspect** is read-only.

None of the above may build or refresh a provider index.

Graph/index **refresh** is an explicit mutation under Atlas control. No silent rebuilds. No hidden mutations during diagnostics. Refresh modes include initial, incremental, noop, and full when policy requires it.

## Storage policy

Provider runtime data belongs under Atlas Home, scoped by project identity:

```text
$ATLAS_HOME/projects/<project-id>/<provider>/
```

Example: `…/codegraph/`.

Do **not** store provider databases or caches in the product repository. Portable project files under `.atlas/` must not gain machine-local absolute paths for provider state. Path containment rejects symlink escapes.

## Security

Foundation expectations:

- local-first;
- no remote embeddings;
- no implicit external LLM enrichment;
- no sending project code outside the developer environment as part of this capability;
- safe execution of external processes (explicit argv, cancellation, timeouts, bounded stdout/stderr);
- provider failures mapped to Atlas-owned errors and states;
- symlink-safe containment for Home/project paths.

Structural analysis is evidence, not absolute truth. Atlas continues to rely on compilers/runtimes where applicable, tests, architecture rules, framework knowledge, and human review.

## Context Economy relationship

Code Intelligence **complements** Context Economy; it does not replace it.

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

Atlas Context Graph remains a separate deferred capability. Product language in upper layers should say “Code Intelligence,” not promote a specific provider name as a domain concept, and must not treat provider graphs as the Atlas Context Graph.

## Replaceability

CodeGraph is the first optional external provider, not a permanent core dependency.

Architecture allows later providers without rewriting governance, Status, Doctor, Context Economy, or agents. Adapters absorb vendor churn; Atlas models stay stable.
