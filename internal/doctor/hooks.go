package doctor

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
)

// evaluateHooks reports compact hooks projection health.
// Repairability matches Runtime Repair: missing/stale → atlas-repair;
// drifted/conflict/malformed → manual. Does not claim Memory injection.
func evaluateHooks(result inspect.Inspection) []Check {
	if !result.Runtime.ConfigLoads {
		return []Check{{
			Severity: SeverityInfo,
			Name:     "hooks",
			Message:  "n/a (Atlas not configured)",
		}}
	}
	h := result.Runtime.Hooks
	if !h.Configured {
		return []Check{{
			Severity: SeverityInfo,
			Name:     "hooks",
			Message:  "not configured (no hooks-capable adapter selected)",
		}}
	}

	counts := h.SummaryCounts()
	repairable := h.RepairableCount()
	manual := h.ManualCount()

	switch {
	case repairable > 0:
		return []Check{fail("hooks projections",
			fmt.Sprintf("not ready (missing=%d stale=%d)", counts[hookproj.StateMissing], counts[hookproj.StateStale]),
			RepairAtlas,
			"Run Runtime Repair to restore Atlas-owned hooks wrappers/plugins")}
	case manual > 0:
		parts := []string{}
		if counts[hookproj.StateDrifted] > 0 {
			parts = append(parts, fmt.Sprintf("drifted=%d", counts[hookproj.StateDrifted]))
		}
		if counts[hookproj.StateConflict] > 0 {
			parts = append(parts, fmt.Sprintf("conflict=%d", counts[hookproj.StateConflict]))
		}
		if counts[hookproj.StateMalformed] > 0 {
			parts = append(parts, fmt.Sprintf("malformed=%d", counts[hookproj.StateMalformed]))
		}
		if counts[hookproj.StateUnsupported] > 0 {
			parts = append(parts, fmt.Sprintf("unsupported=%d", counts[hookproj.StateUnsupported]))
		}
		msg := "not repairable automatically"
		if len(parts) > 0 {
			msg = msg + " (" + joinComma(parts) + ")"
		}
		return []Check{fail("hooks projections", msg, RepairManual,
			"Resolve hooks conflicts/drift/malformed config manually; Runtime Repair does not overwrite digest-drifted owned wrappers")}
	case counts[hookproj.StateReady] > 0:
		return []Check{{
			Severity: SeverityPass,
			Name:     "hooks projections",
			Message:  fmt.Sprintf("%d ready", counts[hookproj.StateReady]),
		}}
	default:
		return []Check{{
			Severity: SeverityInfo,
			Name:     "hooks",
			Message:  "no hooks projections evaluated",
		}}
	}
}

func joinComma(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		out := parts[0]
		for _, p := range parts[1:] {
			out += ", " + p
		}
		return out
	}
}
