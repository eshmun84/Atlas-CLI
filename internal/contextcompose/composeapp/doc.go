// Package composeapp is the application orchestration layer for Context
// Composition (Slice 36B.6D).
//
// It retrieves Memory and Context Economy candidates through the existing
// source adapters, coordinates budgets, and calls contextcompose.Compose.
// The package is independent of Cobra, the TUI, and adapter projections.
package composeapp
