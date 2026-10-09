package runtime_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func mustRenderAgentsMD(t *testing.T, projectName string, contextGraphEnabled bool, selected []string, existing []byte) string {
	t.Helper()
	out, err := config.RenderAgentsMD(projectName, contextGraphEnabled, selected, existing)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustRenderCursorAtlasMDC(t *testing.T, projectName string) string {
	t.Helper()
	out, err := config.RenderCursorAtlasMDC(projectName)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustRenderOpenCodeAtlas(t *testing.T, projectName string) string {
	t.Helper()
	out, err := config.RenderOpenCodeAtlas(projectName)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
