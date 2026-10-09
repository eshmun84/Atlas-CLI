# Code Intelligence

Code Intelligence is Atlas’s capability for structural understanding of a project’s codebase.

It answers questions that file listing alone cannot: which symbols exist, how they call each other, what depends on what, and what a change is likely to touch. That evidence helps Atlas select relevant context for agents and avoid broad, low-signal repository reads.

## Status

Code Intelligence is implemented as an optional external provider boundary under `internal/codeintel/`. The first adapter is CodeGraph (`internal/codeintel/codegraph/`). Atlas runs normally with no Code Intelligence provider installed.

There is **no** CodeGraph MCP integration. Atlas Context Graph remains deferred and separate.

## Capabilities

- **Provider probe** — detect executable, version, and compatibility.
- **Lifecycle / freshness** — missing, available, ready, stale, incompatible, unavailable, error.
- **Explicit refresh** — initial, incremental, noop, or full refresh under Atlas control.
- **Metadata + fingerprint** — Home-scoped project metadata and source fingerprinting.
- **Containment** — symlink-safe paths; provider data under Atlas Home, not the product repo.
- **Status / Doctor** — read-only visibility into Code Intelligence health.

## Ownership and providers

Atlas owns the **Code Intelligence** abstraction: contracts, normalized models, lifecycle, and how Status, Doctor, and Context Economy consume results.

Providers are optional backends behind that abstraction. **CodeGraph** is the first external provider; it is installed outside Atlas and is not part of the Atlas Go core.

## Relationships

- **Complements Context Economy.** Context Economy v0 remains the file-based context surface. Code Intelligence adds structural evidence; it does not replace Context Economy.
- **Not Atlas Context Graph.** Atlas Context Graph is a separate deferred capability.
- **Not MCP.** CodeGraph is not projected as an MCP server.

## Further reading

- [architecture.md](architecture.md) — provider-neutral architecture, lifecycle, storage, and security.
- [codegraph-provider.md](codegraph-provider.md) — CodeGraph-specific responsibility and integration boundaries.
