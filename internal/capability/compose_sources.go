package capability

import "github.com/eshmun84/Atlas-CLI/internal/contextcompose"

// ComposeSourcesFromPlan maps ContextNeeds / RetrievalActions to contextcompose
// source identifiers for atlas context compose. OpenSpec is never a compose source.
func ComposeSourcesFromPlan(plan CapabilityPlan) []string {
	want := map[string]bool{}
	for _, a := range plan.RetrievalActions {
		switch a.Source {
		case RetrievalMemory:
			want[contextcompose.SourceMemory] = true
		case RetrievalCE:
			want[contextcompose.SourceContextEconomy] = true
		case RetrievalCodeGraph:
			want[contextcompose.SourceCodeGraph] = true
		}
	}
	for _, c := range plan.ContextNeeds {
		switch c.ID {
		case ContextMemory:
			want[contextcompose.SourceMemory] = true
		case ContextEconomy:
			want[contextcompose.SourceContextEconomy] = true
		case ContextCodeIntelligence:
			want[contextcompose.SourceCodeGraph] = true
		}
	}
	order := []string{
		contextcompose.SourceMemory,
		contextcompose.SourceContextEconomy,
		contextcompose.SourceCodeGraph,
	}
	var out []string
	for _, s := range order {
		if want[s] {
			out = append(out, s)
		}
	}
	return out
}
