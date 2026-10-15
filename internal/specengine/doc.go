// Package specengine is the Spec Engine adapter layer.
//
// Neutral SDD types and the Engine interface live in internal/sdd.
// Provider adapters (currently openspec only) live in subpackages here and
// import internal/sdd. internal/sdd must never import these adapters.
//
// Direction: next Atlas version supports OpenSpec only. Atlas Native Spec
// Engine is deferred; additional engines are future. Keep this layer thin —
// not a generic provider plugin platform. Current OpenSpec adapter still
// owns temporary filesystem/bootstrap heuristics pending Block B hardening.
package specengine
