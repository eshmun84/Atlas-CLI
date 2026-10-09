package mcp

import (
	"fmt"
	"os"
	"os/exec"
)

// EvaluateHealth builds a read-only MCP health snapshot. It never writes,
// authenticates, installs packages, or opens network connections.
func EvaluateHealth(root, homePath, projectID string, desired DesiredState, adapters []AdapterID, projectors map[AdapterID]Projector) Health {
	health := Health{
		SelectedCount: len(desired.Selected()),
		Ready:         true,
	}
	if err := ValidateDesiredState(desired); err != nil {
		health.DefinitionErr = append(health.DefinitionErr, err.Error())
		health.Ready = false
	}

	ownership, err := LoadOwnership(homePath, projectID)
	if err != nil {
		health.DefinitionErr = append(health.DefinitionErr, err.Error())
		health.Ready = false
	}

	desiredMat := desired.SelectedMaterializable()
	for _, adapter := range adapters {
		ah := AdapterHealth{
			Adapter:     adapter,
			SupportsMCP: false,
			Selected:    len(desiredMat),
		}
		proj, ok := projectors[adapter]
		if !ok || proj == nil || !proj.SupportsMCP() {
			ah.Status = ProjectionUnsupported
			ah.Issues = append(ah.Issues, "adapter does not support MCP")
			health.Ready = false
			health.Adapters = append(health.Adapters, ah)
			continue
		}
		ah.SupportsMCP = true
		ah.NativePath = proj.ConfigRelPath()

		snap, err := proj.InspectMCPProjection(root)
		if err != nil {
			ah.Status = ProjectionBlocked
			ah.Issues = append(ah.Issues, err.Error())
			health.Ready = false
			health.Adapters = append(health.Adapters, ah)
			continue
		}
		ah.NativeExists = snap.Exists
		ah.Malformed = snap.Malformed
		ah.MalformError = snap.MalformError
		if snap.Malformed {
			ah.Status = ProjectionMalformed
			ah.Issues = append(ah.Issues, snap.MalformError)
			health.Ready = false
			health.Adapters = append(health.Adapters, ah)
			continue
		}

		owned := ownership.AdapterEntries(adapter)
		ownedSet := map[string]struct{}{}
		for _, e := range owned {
			ownedSet[e.NativeKey] = struct{}{}
		}

		if len(desiredMat) == 0 {
			ah.Status = ProjectionEmpty
			health.Adapters = append(health.Adapters, ah)
			continue
		}

		materialized := 0
		for _, def := range desiredMat {
			key := NativeKeyFor(def)
			entry, err := proj.BuildMCPProjection(root, def)
			if err != nil {
				ah.Issues = append(ah.Issues, err.Error())
				health.Ready = false
				continue
			}
			existing, exists := snap.Servers[key]
			if !exists {
				ah.MissingKeys = append(ah.MissingKeys, key)
				continue
			}
			if _, isOwned := ownedSet[key]; !isOwned {
				// Desired key present but not Atlas-owned: conflict, never materialized.
				ah.OwnershipConflict = append(ah.OwnershipConflict, key)
				continue
			}
			if mapsEqual(existing, entry.Payload) {
				materialized++
			} else {
				ah.DriftedKeys = append(ah.DriftedKeys, key)
			}
		}
		ah.Materialized = materialized

		switch {
		case len(ah.OwnershipConflict) > 0:
			ah.Status = ProjectionBlocked
			health.Ready = false
		case len(ah.MissingKeys) == 0 && len(ah.DriftedKeys) == 0 && materialized == len(desiredMat):
			ah.Status = ProjectionMaterialized
		case len(ah.DriftedKeys) > 0 || (materialized > 0 && len(ah.MissingKeys) > 0):
			ah.Status = ProjectionDrifted
			health.Ready = false
		case materialized == 0:
			ah.Status = ProjectionMissing
			health.Ready = false
		default:
			ah.Status = ProjectionDrifted
			health.Ready = false
		}

		for _, def := range desiredMat {
			ah.Warnings = append(ah.Warnings, definitionWarnings(def)...)
		}
		health.Adapters = append(health.Adapters, ah)
	}

	// Non-materializable selected builtins are warnings, not hard failures.
	for _, def := range desired.Selected() {
		if !def.Materializable {
			health.DefinitionErr = append(health.DefinitionErr,
				fmt.Sprintf("%s selected but not materializable: %s", def.ID, def.CompatibilityNote))
		}
	}
	return health
}

func definitionWarnings(def Definition) []string {
	var out []string
	if def.PrerequisiteCommand != "" {
		if _, err := exec.LookPath(def.PrerequisiteCommand); err != nil {
			out = append(out, fmt.Sprintf("missing prerequisite command %q for %s", def.PrerequisiteCommand, def.ID))
		}
	}
	if def.AuthRequirement == AuthEnvironmentReference {
		for _, name := range ReferencedEnvNames(def) {
			if os.Getenv(name) == "" {
				// Optional for builtins that document anonymous fallback (e.g. Context7).
				if def.ID == BuiltinContext7 {
					out = append(out, fmt.Sprintf("optional env reference %s not set for %s (anonymous mode may apply)", name, def.ID))
					continue
				}
				out = append(out, fmt.Sprintf("env reference %s not set for %s", name, def.ID))
			}
		}
	}
	if def.AuthRequirement == AuthOAuthExternal {
		out = append(out, fmt.Sprintf("%s requires external OAuth/auth in the agent (Atlas does not authenticate)", def.ID))
	}
	if def.Transport == TransportSSE {
		out = append(out, fmt.Sprintf("%s uses legacy sse transport", def.ID))
	}
	return out
}

// ReferencedEnvNames returns deduplicated environment variable names referenced
// by EnvRefs and HeaderRefs. Secrets are never returned — names only.
func ReferencedEnvNames(def Definition) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		name = EnvRefName(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, ref := range def.EnvRefs {
		add(ref)
	}
	for _, ref := range def.HeaderRefs {
		add(ref.Env)
	}
	return out
}
