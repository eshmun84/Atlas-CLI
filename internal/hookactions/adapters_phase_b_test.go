package hookactions_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	adapteropencode "github.com/eshmun84/Atlas-CLI/internal/adapters/opencode"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func TestPhaseB_CursorBridgeDeclaresSessionStart(t *testing.T) {
	b := adaptercursor.NewHooksBridge()
	found := false
	for _, et := range b.DeclaredEvents() {
		if et == hooks.EventSessionStart {
			found = true
		}
		if et == hooks.EventSessionResume {
			t.Fatal("Cursor must not claim session.resume until native mapping exists")
		}
	}
	if !found {
		t.Fatal("Cursor bridge must declare session.start")
	}
	now := time.Now().UTC()
	d, ok, err := adaptercursor.TranslateToCanonical(adaptercursor.NativePayload{
		HookEventName: adaptercursor.NativeSessionStart,
		SessionID:     "s1",
	}, now)
	if err != nil || !ok || d.Event.Type != hooks.EventSessionStart {
		t.Fatalf("translate=%#v ok=%v err=%v", d, ok, err)
	}
}

func TestPhaseB_OpenCodeBridgeDeclaresSessionStart(t *testing.T) {
	b := adapteropencode.NewHooksBridge()
	found := false
	for _, et := range b.DeclaredEvents() {
		if et == hooks.EventSessionStart {
			found = true
		}
		if et == hooks.EventSessionResume {
			t.Fatal("OpenCode must not claim session.resume until native mapping exists")
		}
	}
	if !found {
		t.Fatal("OpenCode bridge must declare session.start")
	}
	now := time.Now().UTC()
	d, ok, err := adapteropencode.TranslateToCanonical(adapteropencode.NativePayload{
		HookEventName: adapteropencode.NativeSessionCreated,
		SessionID:     "s1",
	}, now)
	if err != nil || !ok || d.Event.Type != hooks.EventSessionStart {
		t.Fatalf("translate=%#v ok=%v err=%v", d, ok, err)
	}
}

func TestPhaseB_AdapterTransitionSameProject(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
	})

	ref := seedInitializedProject(t, "pb-transition")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	src := filepath.Join(ref.Root, "keep.go")
	write(t, src, "package keep\n")

	for i, ad := range []adapters.ID{adapters.Cursor, adapters.OpenCode} {
		hookactions.ResetAdmitStateForTest()
		cap := sessionCapableFor(ad, hooks.EventSessionStart)
		eng := hooksengine.Engine{
			Registry:           hooksengine.DefaultRegistry(),
			Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
			CapabilityOverride: &cap,
		}
		resp := eng.Dispatch(nil, hooksengine.Request{
			SchemaVersion: 1,
			Adapter:       string(ad),
			Event: hooksengine.EventInput{
				ID: "tr-" + string(ad), Type: string(hooks.EventSessionStart),
				OccurredAt: time.Now().UTC().Format(time.RFC3339),
				Payload: hooksengine.PayloadInput{
					ProjectID: ref.ProjectID, SessionID: "s",
				},
			},
		})
		if resp.Status != hooksengine.StatusSuccess {
			t.Fatalf("adapter %s i=%d: %#v", ad, i, resp)
		}
	}
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
	got, err := os.ReadFile(src)
	if err != nil || string(got) != "package keep\n" {
		t.Fatal("user source must be preserved across adapter transition")
	}
}
