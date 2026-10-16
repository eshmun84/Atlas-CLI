package doctor

import (
	"context"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

// evaluateRequiredOpenSpec reports OpenSpec provider health when SDD+openspec is required.
// Status/Doctor remain read-only: only Probe (version/list/help) is used.
// Prefers the Inspection-shared provider snapshot (one probe per Inspect cycle).
func evaluateRequiredOpenSpec(result inspect.Inspection) []Check {
	if !result.Runtime.ConfigLoads || !result.Runtime.DependsOnSDDContract {
		return nil
	}

	doc := result.Runtime.Document
	pin := strings.TrimSpace(doc.Governance.OpenSpecVersion)
	root := strings.TrimSpace(result.RootPath)
	h := result.OpenSpecProvider
	if !result.OpenSpecProviderProbed {
		// Fallback for unit tests that construct Inspection without Inspect().
		h = openspec.Probe(context.Background(), openspec.ProbeInput{
			Root:       root,
			ProjectPin: pin,
		})
	}

	var checks []Check

	switch h.State {
	case openspec.StateMissing:
		checks = append(checks, fail("openspec provider", h.Message, RepairManual, h.InstallHint))
	case openspec.StateIncompatible:
		msg := h.Message
		switch h.IncompatKind {
		case openspec.IncompatProjectMismatch:
			msg = "OpenSpec project version mismatch: " + h.Message
		case openspec.IncompatUnsupported:
			msg = "OpenSpec unsupported version: " + h.Message
		case openspec.IncompatCapability:
			msg = "OpenSpec missing capability: " + h.Message
		}
		checks = append(checks, fail("openspec provider", msg, RepairManual, h.InstallHint))
	case openspec.StateError:
		checks = append(checks, fail("openspec provider", h.Message, RepairManual,
			"Resolve OpenSpec CLI/project state manually; Atlas will not heuristically repair provider trees"))
	case openspec.StateAvailable:
		switch h.Project {
		case openspec.ProjectAbsent:
			// Runtime Repair refuses OpenSpec bootstrap — Init owns provider init.
			checks = append(checks, fail("openspec provider", "OpenSpec CLI available but project not initialized", RepairManual,
				"Run Atlas Init Apply (reviewed openspec init) or openspec init --tools <adapters>; Runtime Repair will not bootstrap OpenSpec"))
		case openspec.ProjectLegacyMkdir:
			checks = append(checks, fail("openspec provider", h.Message, RepairManual,
				"Run Atlas Init Apply to replace legacy mkdir bootstrap with real openspec init; Runtime Repair will not invent provider state"))
		default:
			if pin == "" {
				checks = append(checks, fail("openspec provider", "OpenSpec project present but governance.openspec_version pin is unset", RepairManual,
					"Run Atlas Init/Configure to adopt the installed OpenSpec version as the project pin"))
			} else {
				checks = append(checks, fail("openspec provider", h.Message, RepairManual, h.InstallHint))
			}
		}
	case openspec.StateReady:
		checks = append(checks, pass("openspec provider",
			"ready (installed "+h.Installed+", project pin "+h.RequiredPin+")"))
		checks = append(checks, evaluateOpenSpecToolProjections(root, doc.Adapters.Selected)...)
	default:
		checks = append(checks, fail("openspec provider", "unknown provider state", RepairManual, ""))
	}

	// Contract remains Atlas-owned.
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

// evaluateOpenSpecToolProjections reports missing opsx/tool projections when the
// provider project is Ready. Remediation is Configure/Init — never Runtime Repair.
func evaluateOpenSpecToolProjections(root string, adapters []string) []Check {
	missing := openspec.MissingToolProjections(root, adapters)
	if len(missing) == 0 {
		return []Check{pass("openspec tool projections", "expected provider tool projections present")}
	}
	return []Check{fail("openspec tool projections",
		"provider tool projections stale/missing for: "+strings.Join(missing, ", "),
		RepairManual,
		"Run Atlas Configure Apply (or Init) to sync OpenSpec tool projections via openspec init --tools / openspec update; Runtime Repair will not invent provider projections")}
}
