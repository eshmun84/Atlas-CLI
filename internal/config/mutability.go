package config

import "strings"

// Mutability classifies how a config key may change after project init.
type Mutability string

const (
	Immutable         Mutability = "immutable"
	Mutable           Mutability = "mutable"
	MigrationRequired Mutability = "migration_required"
	Unknown           Mutability = "unknown"
)

// ClassifyKey returns the mutability class for a dotted config key path.
func ClassifyKey(path string) Mutability {
	switch path {
	case "project.name", "project.mode", "atlas.version":
		return Immutable
	case "governance.storage_mode", "spec_engine.provider", "memory.provider", "orchestration.default_workflow":
		return MigrationRequired
	}

	for _, root := range []string{
		"adapters",
		"source_control",
		"assets",
		"registry",
		"external_context_providers",
		"materialization",
	} {
		if path == root || strings.HasPrefix(path, root+".") {
			return Mutable
		}
	}

	return Unknown
}
