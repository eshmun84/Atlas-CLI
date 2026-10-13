// Package codegraphsource is the thin, read-only CodeGraph retrieval adapter
// for Context Composer (Slice 36B.6C).
//
// It resolves project identity through memoryretrieve.Env, uses the existing
// codeintel/codegraph Adapter for Probe/Status availability, executes a single
// bounded `codegraph query --json` against an isolated temporary snapshot of
// the Atlas-owned graph.db, and maps results into contextcompose.Candidates
// with Composer-compatible SourceStatus.
//
// Querying a snapshot prevents the external CodeGraph/SQLite CLI from creating
// WAL/SHM sidecars beside live Atlas Home storage. This package never builds,
// rebuilds, migrates, or mutates live graph state. Query generation is
// deterministic identifier extraction — no LLM rewriting, embeddings, or network.
package codegraphsource
