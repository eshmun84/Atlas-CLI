// Package composeapp is the application orchestration layer for Context
// Composition (Slice 36B.6D / 36B.6C).
//
// It retrieves Memory, Context Economy, and CodeGraph candidates through the
// existing source adapters, coordinates budgets and source diversity, and
// calls contextcompose.Compose. The package is independent of Cobra, the TUI,
// and adapter projections.
package composeapp
