package opencode

import (
	"os"
	"sync"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

const (
	hooksPluginRel   = ".opencode/plugins/atlas-hooks/index.ts"
	hooksPluginAsset = "adapter-files/opencode/hooks/atlas-hooks/index.ts"
)

// HooksProjector implements hookproj.Projector for OpenCode V2 plugins.
type HooksProjector struct{}

// NewHooksProjector returns an OpenCode hooks projector.
func NewHooksProjector() HooksProjector { return HooksProjector{} }

func (HooksProjector) ID() adapters.ID               { return adapters.OpenCode }
func (HooksProjector) SupportsHooks() bool           { return true }
func (HooksProjector) Style() hookproj.Style         { return hookproj.StylePluginFile }
func (HooksProjector) ArtifactRel() string           { return hooksPluginRel }
func (HooksProjector) ArtifactFileMode() os.FileMode { return 0o644 }
func (HooksProjector) ConfigMergeRel() string        { return "" }
func (HooksProjector) ManagedCommand() string        { return "" }

func (HooksProjector) ManagedEvents() []string {
	out := make([]string, len(SupportedNativeEvents))
	copy(out, SupportedNativeEvents)
	return out
}

var (
	pluginOnce sync.Once
	pluginBody []byte
)

func (HooksProjector) ArtifactBytes() []byte {
	pluginOnce.Do(func() {
		data, err := assets.Content.ReadFile(hooksPluginAsset)
		if err != nil {
			pluginBody = nil
			return
		}
		pluginBody = append([]byte(nil), data...)
	})
	return append([]byte(nil), pluginBody...)
}

var _ hookproj.Projector = HooksProjector{}
