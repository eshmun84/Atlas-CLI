// Package specengine is the Spec Engine adapter layer.
//
// Neutral SDD types and the Engine interface live in internal/sdd.
// Provider adapters (for example openspec) live in subpackages here and
// import internal/sdd. internal/sdd must never import these adapters.
package specengine
