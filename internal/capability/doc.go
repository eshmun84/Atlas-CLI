// Package capability implements Task Capability Discovery for Atlas Block C.
//
// DiscoverPlan is a pure, deterministic planner: task + bounded evidence →
// CapabilityPlan (capability needs, context needs, task-active skills,
// governed retrieval actions). It does not query Memory, CodeGraph, or MCP,
// does not mutate the filesystem, and does not bypass AGENTS/SDD gates.
//
// Technology signals are secondary evidence only — never permanent skill
// selection (DOGFOOD-015).
package capability
