// Package memorysource is the thin, read-only Memory source adapter for
// Context Composer (Slice 36B.6B).
//
// It resolves project identity through memoryretrieve.Env, invokes
// memoryretrieve.Service.Context, and maps ContextItems into
// contextcompose.Candidates with Composer-compatible SourceStatus.
//
// This package does not open SQLite independently, reimplement FTS/ranking,
// persist composed packs, mutate Memory Core, or write Atlas Home.
package memorysource
