package cursor

import (
	"sync"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

const (
	hooksJSONRel      = ".cursor/hooks.json"
	hooksWrapperRel   = ".cursor/hooks/atlas/dispatch"
	hooksManagedCmd   = ".cursor/hooks/atlas/dispatch"
	hooksWrapperAsset = "adapter-files/cursor/hooks/dispatch"
)

// HooksProjector implements hookproj.Projector for Cursor.
type HooksProjector struct{}

// NewHooksProjector returns a Cursor hooks projector.
func NewHooksProjector() HooksProjector { return HooksProjector{} }

func (HooksProjector) ID() adapters.ID      { return adapters.Cursor }
func (HooksProjector) SupportsHooks() bool  { return true }
func (HooksProjector) HooksJSONRel() string { return hooksJSONRel }
func (HooksProjector) WrapperRel() string   { return hooksWrapperRel }
func (HooksProjector) ManagedCommand() string {
	return hooksManagedCmd
}

func (HooksProjector) ManagedEvents() []string {
	out := make([]string, len(SupportedNativeEvents))
	copy(out, SupportedNativeEvents)
	return out
}

var (
	wrapperOnce sync.Once
	wrapperBody []byte
)

func (HooksProjector) WrapperBytes() []byte {
	wrapperOnce.Do(func() {
		data, err := assets.Content.ReadFile(hooksWrapperAsset)
		if err != nil {
			// Fail closed at reconcile time via empty digest mismatch — keep empty.
			wrapperBody = nil
			return
		}
		wrapperBody = append([]byte(nil), data...)
	})
	return append([]byte(nil), wrapperBody...)
}

var _ hookproj.Projector = HooksProjector{}
