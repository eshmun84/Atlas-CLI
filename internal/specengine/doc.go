// Package specengine is the Spec Engine adapter layer.
//
// Neutral SDD types and the Engine interface live in internal/sdd.
// Provider adapters (currently openspec only) live in subpackages here and
// import internal/sdd. internal/sdd must never import these adapters.
//
// Direction: Atlas supports OpenSpec only. Atlas Native Spec Engine is
// deferred; additional engines are future. Keep this layer thin — not a
// generic provider plugin platform. Real OpenSpec CLI is required for
// governed SDD; Atlas does not emulate OpenSpec or mkdir-bootstrap provider
// trees (Init owns openspec init; Runtime Repair refuses provider bootstrap).
package specengine
