// Package contextcompose is the provider-neutral Context Composer Core (Slice 36B.6A).
//
// It accepts bounded composition candidates, validates them, applies deterministic
// ordering and identity deduplication, enforces global and per-source rune budgets,
// and emits a versioned structured context pack with provenance and explicit
// omissions.
//
// This package is pure Go: it performs no filesystem, database, network, or
// subprocess I/O and never persists state. Upstream slices supply candidates;
// downstream slices deliver packs to agents.
//
// Budget unit: UTF-8 runes (Unicode code points), not bytes and not tokenizer
// tokens. Invariant: used_chars <= budget_chars.
package contextcompose
