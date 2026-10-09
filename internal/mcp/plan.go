package mcp

import "fmt"

// BuildPlan constructs an explicit reconciliation plan for one adapter.
func BuildPlan(adapter AdapterID, desired []Definition, snap NativeSnapshot, owned []OwnedEntry) Plan {
	plan := Plan{}
	if snap.Malformed {
		plan.Blocked = true
		plan.Actions = append(plan.Actions, PlanAction{
			Adapter: adapter,
			Kind:    ActionBlocked,
			Reason:  fmt.Sprintf("native MCP config malformed: %s", snap.MalformError),
		})
		return plan
	}

	ownedKeys := map[string]string{} // nativeKey -> definitionID
	for _, e := range owned {
		ownedKeys[e.NativeKey] = e.DefinitionID
	}

	desiredByKey := map[string]Definition{}
	for _, def := range desired {
		if !def.Enabled || !def.Materializable {
			continue
		}
		key := NativeKeyFor(def)
		desiredByKey[key] = def
	}

	// Creates / updates / unchanged for desired materializable entries.
	for key, def := range desiredByKey {
		_, exists := snap.Servers[key]
		if !exists {
			plan.Actions = append(plan.Actions, PlanAction{
				Adapter:      adapter,
				DefinitionID: def.ID,
				NativeKey:    key,
				Kind:         ActionCreate,
				Reason:       "missing Atlas-managed entry",
			})
			continue
		}
		// Ownership conflict: any pre-existing native key that is not recorded
		// as Atlas-owned blocks. Content equality never establishes ownership.
		if _, isOwned := ownedKeys[key]; !isOwned {
			plan.Blocked = true
			plan.Actions = append(plan.Actions, PlanAction{
				Adapter:      adapter,
				DefinitionID: def.ID,
				NativeKey:    key,
				Kind:         ActionBlocked,
				Reason:       "native key exists and is not Atlas-owned; refusing adopt or overwrite",
			})
			continue
		}
		plan.Actions = append(plan.Actions, PlanAction{
			Adapter:      adapter,
			DefinitionID: def.ID,
			NativeKey:    key,
			Kind:         ActionUpdate, // refined by RefinePlanWithPayloads
			Reason:       "existing entry requires comparison",
		})
	}

	// Removals: owned keys no longer desired.
	for key, defID := range ownedKeys {
		if _, keep := desiredByKey[key]; keep {
			continue
		}
		plan.Actions = append(plan.Actions, PlanAction{
			Adapter:      adapter,
			DefinitionID: defID,
			NativeKey:    key,
			Kind:         ActionRemove,
			Reason:       "Atlas-managed entry no longer desired",
		})
	}

	return plan
}

// RefinePlanWithPayloads upgrades create/update/unchanged using built payloads.
func RefinePlanWithPayloads(plan Plan, built map[string]NativeEntry, snap NativeSnapshot) Plan {
	if plan.Blocked {
		return plan
	}
	out := Plan{Blocked: plan.Blocked}
	for _, action := range plan.Actions {
		if action.Kind == ActionRemove || action.Kind == ActionBlocked {
			out.Actions = append(out.Actions, action)
			continue
		}
		entry, ok := built[action.NativeKey]
		if !ok {
			action.Kind = ActionBlocked
			action.Reason = "missing built projection payload"
			out.Blocked = true
			out.Actions = append(out.Actions, action)
			continue
		}
		existing, exists := snap.Servers[action.NativeKey]
		if !exists {
			action.Kind = ActionCreate
			action.Reason = "create Atlas-managed entry"
			out.Actions = append(out.Actions, action)
			continue
		}
		if mapsEqual(existing, entry.Payload) {
			action.Kind = ActionUnchanged
			action.Reason = "already matches desired projection"
		} else {
			action.Kind = ActionUpdate
			action.Reason = "update Atlas-managed entry"
		}
		out.Actions = append(out.Actions, action)
	}
	return out
}

// OwnedKeys returns native keys from ownership entries.
func OwnedKeys(entries []OwnedEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.NativeKey)
	}
	return out
}

// OwnershipForDesired builds ownership entries for successfully applied desired defs.
func OwnershipForDesired(desired []Definition) []OwnedEntry {
	out := make([]OwnedEntry, 0, len(desired))
	for _, def := range desired {
		if !def.Enabled || !def.Materializable {
			continue
		}
		out = append(out, OwnedEntry{
			DefinitionID: def.ID,
			NativeKey:    NativeKeyFor(def),
		})
	}
	return out
}
