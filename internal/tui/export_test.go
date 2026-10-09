package tui

import (
	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
)

// BuildCodeIntelPlanForTest exposes buildCodeIntelPlan.
func BuildCodeIntelPlanForTest(disc inspect.Inspection, forceFull bool) codeintel.RefreshPlan {
	return buildCodeIntelPlan(disc, forceFull)
}

// SetCodeIntelProjectIDForTest installs a test-only ProjectID seam.
func SetCodeIntelProjectIDForTest(fn func(root, projectName string) (string, error)) {
	if fn == nil {
		codeIntelProjectIDFn = home.ProjectID
		return
	}
	codeIntelProjectIDFn = fn
}

// SetCodeIntelFingerprintForTest installs a test-only fingerprint seam.
func SetCodeIntelFingerprintForTest(fn func(root string) (codeintel.SourceFingerprint, error)) {
	if fn == nil {
		codeIntelFingerprintFn = codeintel.ComputeSourceFingerprint
		return
	}
	codeIntelFingerprintFn = fn
}
