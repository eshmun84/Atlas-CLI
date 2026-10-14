package home

import (
	"fmt"
	"strings"
)

// RepairHomeHint is the actionable guidance when Atlas Home is not healthy.
// Init Apply must fail closed and point operators here — never bootstrap Home.
const RepairHomeHint = "Atlas Home is not healthy. Run Runtime Repair to restore Atlas Home, or re-run `make install` to materialize it, then retry init."

// Healthy reports whether Atlas Home is machine-ready for project Init Apply.
// Read-only: never creates or mutates Home.
func Healthy(status Status) bool {
	if !status.Exists {
		return false
	}
	if !status.LayoutComplete {
		return false
	}
	if !status.StateLoads {
		return false
	}
	if len(status.MissingAssets) > 0 || len(status.DriftedAssets) > 0 || len(status.AssetErrors) > 0 {
		return false
	}
	return true
}

// RequireHealthy inspects Atlas Home and returns an actionable error when it is
// missing, incomplete, corrupt, or drifted. Read-only: never creates Home.
func RequireHealthy() (Status, error) {
	status := Inspect()
	if Healthy(status) {
		return status, nil
	}
	return status, fmt.Errorf("%s (%s)", RepairHomeHint, summarizeUnhealthy(status))
}

func summarizeUnhealthy(status Status) string {
	var parts []string
	switch {
	case status.Path == "" && status.StateError != "":
		parts = append(parts, "resolve failed: "+status.StateError)
	case !status.Exists:
		parts = append(parts, "missing at "+displayHomePath(status.Path))
	default:
		parts = append(parts, "path "+displayHomePath(status.Path))
		if !status.LayoutComplete {
			parts = append(parts, "layout incomplete")
		}
		if status.StateError != "" {
			parts = append(parts, "state: "+status.StateError)
		} else if !status.StatePresent {
			parts = append(parts, "state missing")
		} else if !status.StateLoads {
			parts = append(parts, "state unreadable")
		}
		if n := len(status.MissingAssets); n > 0 {
			parts = append(parts, fmt.Sprintf("%d missing assets", n))
		}
		if n := len(status.DriftedAssets); n > 0 {
			parts = append(parts, fmt.Sprintf("%d drifted assets", n))
		}
		if n := len(status.AssetErrors); n > 0 {
			parts = append(parts, fmt.Sprintf("%d asset errors", n))
		}
	}
	return strings.Join(parts, "; ")
}

func displayHomePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "(unresolved)"
	}
	return path
}

// NeedsRepair reports whether Runtime Repair should offer an Atlas Home refresh.
func NeedsRepair(status Status) bool {
	return !Healthy(status)
}
