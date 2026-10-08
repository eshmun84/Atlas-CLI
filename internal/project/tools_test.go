package project_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestDiscoverTools_ReturnsRequiredNames(t *testing.T) {
	t.Parallel()

	tools := project.DiscoverTools()
	if len(tools) != len(project.RequiredTools) {
		t.Fatalf("got %d tools, want %d", len(tools), len(project.RequiredTools))
	}

	got := map[string]project.ToolInfo{}
	for _, tool := range tools {
		got[tool.Name] = tool
	}

	for _, name := range project.RequiredTools {
		tool, ok := got[name]
		if !ok {
			t.Fatalf("missing tool entry for %q", name)
		}
		if tool.Available && tool.Path == "" {
			t.Fatalf("available tool %q must include Path", name)
		}
		if !tool.Available && tool.Path != "" {
			t.Fatalf("unavailable tool %q must have empty Path", name)
		}
	}
}
