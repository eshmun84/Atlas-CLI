# CodeGraph provider

CodeGraph-specific boundaries for Atlas Code Intelligence. For the provider-neutral design, see [architecture.md](architecture.md).

CodeGraph is an **optional external** backend. It is not part of the Atlas Go core and is not Atlas Context Graph.

## Responsibility boundary

### Developer

- Installs CodeGraph outside Atlas.
- Updates, downgrades, or uninstalls CodeGraph.
- Keeps the executable available on the environment PATH (or configured location).

### Atlas

- Detects the executable.
- Reads version information.
- Verifies required capabilities.
- Applies a compatibility policy.
- Allows activation only when installed and compatible.
- Configures project-scoped storage paths under Atlas Home.
- Executes the provider safely.
- Consumes machine-readable JSON.
- Normalizes results into Atlas models.
- Reports states such as missing, incompatible, stale, unavailable, or error.

### Mandatory rule

**Atlas is not a package manager.**

Atlas does **not**:

- install CodeGraph;
- update CodeGraph;
- uninstall CodeGraph;
- run `npm`, `brew`, `curl`, or other installers to manage CodeGraph.

## Missing executable

If CodeGraph is not installed:

- provider capability = unavailable;
- it cannot be activated;
- Atlas continues to operate without Code Intelligence from this provider.

If it was configured and later disappears:

- Atlas reports WARNING / unavailable;
- Atlas does **not** auto-repair or reinstall;
- Atlas does **not** attempt package-manager recovery.

## Integration boundary

```text
Atlas
  → Code Intelligence abstraction
  → CodeGraph adapter
  → external CodeGraph CLI
  → machine-readable JSON
```

Rules:

- do not import CodeGraph’s JavaScript implementation into the Go core;
- do not read `graph.db` (SQLite) directly;
- do not depend on CodeGraph’s internal schema;
- do not concatenate shell commands or use `sh -c` for provider invocation;
- use `os/exec` with explicit argument vectors;
- honor context cancellation;
- apply timeouts;
- bound and capture stdout/stderr;
- prefer JSON as the machine-readable contract; do not scrape human tables when JSON exists;
- map provider failures to Atlas-owned errors.

## Compatibility

Finding a binary is not enough.

Atlas must:

- locate the executable;
- obtain its version;
- verify required capabilities;
- enforce a compatibility policy;
- refuse activation when version or capabilities fail that policy.

The adapter exists to shield Atlas from upstream CLI and schema churn.

## Storage

Conceptual layout under Atlas Home:

```text
$ATLAS_HOME/projects/<project-id>/codegraph/graph.db
$ATLAS_HOME/projects/<project-id>/codegraph/metadata.json
```

`metadata.json` may record:

- provider identity;
- provider version;
- project identity;
- source revision;
- last successful build/update timestamp;
- freshness state.

Do **not** create a `.codegraph` directory (or equivalent) inside the product repository.

## Evidence model

CodeGraph output is **structural evidence**, not absolute truth.

Treat results carefully when the codebase uses:

- reflection;
- dependency injection;
- dynamic dispatch;
- framework “magic”;
- convention-heavy frameworks (for example Laravel-style indirection);
- runtime-generated relationships.

Atlas should still prefer compilers/runtimes where applicable, tests, architecture rules, framework knowledge, and human review when evidence conflicts.

## Initial scope

First integration target: **CLI + JSON** behind the Code Intelligence abstraction.

Out of scope for this foundation:

- MCP materialization;
- remote embeddings;
- external LLM enrichment;
- Atlas Context Graph;
- package management of CodeGraph.
