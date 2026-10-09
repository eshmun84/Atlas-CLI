package config

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
)

// Testable seams for InspectMCPHealth resolution paths.
var (
	resolveHomeFn = home.Resolve
	projectIDFn   = home.ProjectID
)

// ReconcileMCPProjections materializes/removes Atlas-owned MCP projections for
// the selected adapters. It does not rematerialize AGENTS.md, rules, agents,
// or run Runtime Repair.
func ReconcileMCPProjections(root string, doc ProjectDocument, draftMCP MCPDraft) (mcp.ReconcileResult, error) {
	root = strings.TrimSpace(root)
	if root == "" || root == "." {
		return mcp.ReconcileResult{}, fmt.Errorf("mcp reconcile: workspace root is required")
	}
	desired, err := draftMCP.DesiredState()
	if err != nil {
		return mcp.ReconcileResult{}, fmt.Errorf("mcp reconcile: desired state: %w", err)
	}

	homePath, err := home.Resolve()
	if err != nil {
		return mcp.ReconcileResult{}, fmt.Errorf("mcp reconcile: atlas home: %w", err)
	}
	projectID, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		return mcp.ReconcileResult{}, fmt.Errorf("mcp reconcile: project id: %w", err)
	}
	if err := home.EnsureProjectLayout(homePath, projectID); err != nil {
		return mcp.ReconcileResult{}, fmt.Errorf("mcp reconcile: %w", err)
	}
	ownership, err := mcp.LoadOwnership(homePath, projectID)
	if err != nil {
		return mcp.ReconcileResult{}, err
	}

	return mcp.Reconcile(mcp.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Desired:    desired,
		Adapters:   SelectedMCPAdapters(doc.Adapters.Selected),
		Projectors: MCPProjectors(),
		Ownership:  ownership,
		DryRun:     false,
	})
}

// InspectMCPHealth returns a read-only MCP health snapshot.
func InspectMCPHealth(root string, doc ProjectDocument) (mcp.Health, error) {
	draft := doc.ToMCPDraft()
	desired, err := draft.DesiredState()
	if err != nil {
		return mcp.Health{
			SelectedCount: draft.SelectedCount(),
			DefinitionErr: []string{err.Error()},
			Ready:         false,
		}, nil
	}
	homePath, err := resolveHomeFn()
	if err != nil {
		return mcp.Health{SelectedCount: draft.SelectedCount(), Ready: false},
			fmt.Errorf("inspect mcp health: resolve Atlas Home: %w", err)
	}
	projectID, err := projectIDFn(root, doc.Project.Name)
	if err != nil {
		return mcp.Health{SelectedCount: draft.SelectedCount(), Ready: false},
			fmt.Errorf("inspect mcp health: project id: %w", err)
	}
	return mcp.EvaluateHealth(root, homePath, projectID, desired, SelectedMCPAdapters(doc.Adapters.Selected), MCPProjectors()), nil
}

// MCPChanged reports whether MCP desired state differs between documents.
func MCPChanged(previous, next ProjectDocument) bool {
	if previous.MCP.Builtins != next.MCP.Builtins {
		return true
	}
	if len(previous.MCP.Custom) != len(next.MCP.Custom) {
		return true
	}
	for i := range previous.MCP.Custom {
		a, b := previous.MCP.Custom[i], next.MCP.Custom[i]
		if a.Name != b.Name ||
			a.Transport != b.Transport ||
			a.CommandOrURL != b.CommandOrURL ||
			a.Enabled != b.Enabled ||
			a.Arguments != b.Arguments ||
			a.EnvironmentReferences != b.EnvironmentReferences ||
			a.AuthRequirement != b.AuthRequirement ||
			!headerRefsEqual(a.HeaderRefs, b.HeaderRefs) {
			return true
		}
	}
	return false
}

func headerRefsEqual(a, b map[string]mcp.HeaderValueRef) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va.Env != vb.Env || va.Prefix != vb.Prefix {
			return false
		}
	}
	return true
}
