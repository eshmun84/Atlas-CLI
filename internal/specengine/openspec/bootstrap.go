package openspec

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// BootstrapResult summarizes legacy EnsureBootstrap (removed from Init/Repair paths).
// Retained type for transitional call sites/tests only.
type BootstrapResult struct {
	Created   bool
	Paths     []string
	Footprint fsafety.TransactionFootprint
}

// LayoutAssessment is a read-only classification of openspec/ under root.
type LayoutAssessment struct {
	Absent       bool
	Valid        bool
	Incompatible bool
	Reason       string
}

// EnsureBootstrap is REMOVED from product Init/Repair paths.
// It remains as a hard error so accidental callers fail closed instead of
// recreating the obsolete mkdir-only fake OpenSpec tree.
//
// Real initialization: openspec.RunInit after human-approved Atlas Init.
func EnsureBootstrap(root string) (BootstrapResult, error) {
	return BootstrapResult{}, fmt.Errorf("openspec: EnsureBootstrap removed; use real openspec init (Atlas Init / confirm-repair)")
}
