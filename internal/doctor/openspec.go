package doctor

import (
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

// evaluateRequiredOpenSpec reports OpenSpec readiness when SDD+openspec is required.
//
// Current/temporary: CLI absence is INFO only (PATH probe; Atlas does not execute
// OpenSpec CLI today). Block B will require a compatible OpenSpec CLI/provider
// for governed SDD (missing/incompatible → FAIL/BLOCKED). Do not treat today's
// optional-CLI stance as the target architecture.
func evaluateRequiredOpenSpec(result inspect.Inspection) []Check {
	if !result.Runtime.ConfigLoads || !result.Runtime.DependsOnSDDContract {
		return nil
	}

	var checks []Check
	cli := toolAvailable(result.Tools, "openspec")
	if cli {
		checks = append(checks, info("openspec cli", "available (CLI execution unused today; provider hardening pending Block B)"))
	} else {
		checks = append(checks, info("openspec cli", "not on PATH (currently optional; Block B will require OpenSpec CLI for governed SDD)"))
	}

	root := strings.TrimSpace(result.RootPath)
	if root != "" && root != "." {
		assess, err := openspec.AssessLayout(root)
		if err != nil {
			checks = append(checks, fail("openspec layout", "inspection failed: "+err.Error(), RepairManual,
				"Resolve filesystem access for openspec/ then re-run Doctor"))
			return checks
		}
		switch {
		case assess.Valid:
			checks = append(checks, pass("openspec layout", "openspec/ and openspec/changes/ present"))
		case assess.Absent:
			checks = append(checks, fail("openspec layout", "required OpenSpec tree absent", RepairConfirm,
				"Run Runtime Repair to create openspec/ + openspec/changes/"))
		default:
			reason := strings.TrimSpace(assess.Reason)
			if reason == "" {
				reason = "incomplete or incompatible OpenSpec tree"
			}
			checks = append(checks, fail("openspec layout", reason, RepairManual,
				"Do not auto-repair; restore or recreate the OpenSpec tree manually"))
		}
	}

	switch {
	case result.Runtime.SDDContractRenderError != "":
		checks = append(checks, fail("sdd openspec contract", "canonical render failed", RepairManual, ""))
	case !result.Runtime.SDDContractPresent:
		checks = append(checks, fail("sdd openspec contract", config.FileSDDOpenSpecContract+" missing", RepairAtlas,
			"Run Runtime Repair to rematerialize the SDD/OpenSpec contract"))
	case !result.Runtime.SDDContractMatches:
		checks = append(checks, fail("sdd openspec contract", "content drifted", RepairAtlas,
			"Run Runtime Repair to restore the Atlas-owned contract"))
	default:
		checks = append(checks, pass("sdd openspec contract", config.FileSDDOpenSpecContract+" present"))
	}
	return checks
}
