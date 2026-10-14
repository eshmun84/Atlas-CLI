package config

import "strings"

// NeedsOpenSpecBootstrap reports whether Init Apply must ensure openspec/ layout.
func NeedsOpenSpecBootstrap(doc ProjectDocument) bool {
	workflow := strings.TrimSpace(doc.Governance.Workflow)
	engine := strings.TrimSpace(doc.Governance.SpecEngine)
	return workflow == WorkflowSDD && engine == SpecProviderOpenSpec
}

// NeedsGitBootstrapMode reports whether source_control implies local Git.
func NeedsGitBootstrapMode(doc ProjectDocument) bool {
	mode := strings.TrimSpace(doc.SourceControl.Mode)
	return mode == SourceControlGitLocal || mode == SourceControlGitGitHub
}
