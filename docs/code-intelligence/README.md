# Code Intelligence

Code Intelligence is Atlas’s capability for structural understanding of a project’s codebase.

It answers questions that file listing alone cannot: which symbols exist, how they call each other, what depends on what, and what a change is likely to touch. That evidence helps Atlas select relevant context for agents and avoid broad, low-signal repository reads.

## Capabilities

- **Symbol discovery** — locate definitions and references by name or pattern.
- **Callers / callees** — follow call relationships around a symbol.
- **Dependency relationships** — map module and package edges.
- **Impact analysis** — estimate blast radius for a symbol or pending diff.
- **Context selection** — narrow the working set of files and symbols for a task.

## Ownership and providers

Atlas owns the **Code Intelligence** abstraction: contracts, normalized models, lifecycle, and how Status, Doctor, and Context Economy consume results.

Providers are optional backends behind that abstraction. **CodeGraph** is the first external provider Atlas evaluates; it is not part of the Atlas core and must not become a global product term in upper layers.

Atlas runs normally with **no** Code Intelligence provider installed or activated.

## Relationships

- **Complements Context Economy.** Context Economy v0 remains the file-based context surface. Code Intelligence adds structural evidence that can improve what gets packed into context; it does not replace Context Economy.
- **Not Atlas Context Graph.** Atlas Context Graph is a separate future capability. Do not conflate provider-backed Code Intelligence with that concept.

## Further reading

- [architecture.md](architecture.md) — provider-neutral architecture, lifecycle, storage, and security.
- [codegraph-provider.md](codegraph-provider.md) — CodeGraph-specific responsibility and integration boundaries.
