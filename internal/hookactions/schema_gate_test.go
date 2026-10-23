package hookactions_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
	"gopkg.in/yaml.v3"
)

func TestAUD01_FutureSchemaSessionAdmissionHasNoSideEffects(t *testing.T) {
	ref := seedInitializedProject(t, "aud01-session")
	setProjectSchemaVersion(t, ref.Root, config.CurrentProjectSchemaVersion+1)

	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	var spawned atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	before := hashTree(t, ref.Home)
	resp := dispatchSession(t, ref, hooks.EventSessionStart, "aud01-session-event")

	if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
		t.Fatalf("native hook must fail open, got %#v", resp)
	}
	for _, result := range resp.Results {
		if result.Outcome != string(hooks.OutcomeSkipped) {
			t.Fatalf("incompatible schema must skip mutations, got %#v", resp.Results)
		}
	}
	if spawned.Load() != 0 {
		t.Fatalf("spawned=%d want 0", spawned.Load())
	}
	if after := hashTree(t, ref.Home); after != before {
		t.Fatal("session admission mutated Atlas Home before schema validation")
	}
}

func TestAUD01_FutureSchemaPostCodeHasNoSideEffects(t *testing.T) {
	ref := seedInitializedProject(t, "aud01-postcode")
	setProjectSchemaVersion(t, ref.Root, config.CurrentProjectSchemaVersion+1)

	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error {
		spawned.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

	capability := sessionCapable(hooks.EventAfterToolExecution)
	engine := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &capability,
	}
	before := hashTree(t, ref.Home)
	resp := engine.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID:         "aud01-postcode-event",
			Type:       string(hooks.EventAfterToolExecution),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID,
				SessionID: "aud01-session",
				ToolName:  "Write",
				Status:    "success",
			},
		},
	})

	if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
		t.Fatalf("native hook must fail open, got %#v", resp)
	}
	if len(resp.Results) != 1 || resp.Results[0].Outcome != string(hooks.OutcomeSkipped) {
		t.Fatalf("incompatible schema must skip post-code mutation, got %#v", resp.Results)
	}
	if spawned.Load() != 0 {
		t.Fatalf("scheduler spawned=%d want 0", spawned.Load())
	}
	if _, err := os.Stat(hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatalf("pending must remain absent, err=%v", err)
	}
	if after := hashTree(t, ref.Home); after != before {
		t.Fatal("post-code mutated pending, lease, fence, CE, or CI state")
	}
}

func TestAUD01_FutureSchemaDirectMaintenanceRejectedBeforeWrites(t *testing.T) {
	ref := seedInitializedProject(t, "aud01-direct-maintain")
	write(t, filepath.Join(ref.Root, "go.mod"), "module aud01\n")
	setProjectSchemaVersion(t, ref.Root, config.CurrentProjectSchemaVersion+1)
	beforeRoot := hashTree(t, ref.Root)
	beforeHome := hashTree(t, ref.Home)

	if _, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home); !errors.Is(err, config.ErrProjectSchemaNewer) {
		t.Fatalf("Context Economy error=%v want ErrProjectSchemaNewer", err)
	}
	if _, err := hookactions.MaintainCodeIntel(context.Background(), ref.Root, ref.Home); !errors.Is(err, config.ErrProjectSchemaNewer) {
		t.Fatalf("Code Intelligence error=%v want ErrProjectSchemaNewer", err)
	}
	if after := hashTree(t, ref.Root); after != beforeRoot {
		t.Fatal("direct maintenance modified the project")
	}
	if after := hashTree(t, ref.Home); after != beforeHome {
		t.Fatal("direct maintenance modified Atlas Home")
	}
}

func TestAUD01_ResolveAtSchemaErrorsAreRecognizable(t *testing.T) {
	tests := []struct {
		name    string
		version int
		want    error
	}{
		{name: "future", version: config.CurrentProjectSchemaVersion + 1, want: config.ErrProjectSchemaNewer},
		{name: "malformed", version: -1, want: config.ErrProjectSchemaMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := seedInitializedProject(t, "aud01-"+test.name)
			setProjectSchemaVersion(t, ref.Root, test.version)
			_, err := hookactions.ResolveAt(ref.Root, ref.Home)
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want %v", err, test.want)
			}
		})
	}
}

func TestAUD01_ResolveAtLegacySchemaRemainsCompatible(t *testing.T) {
	ref := seedInitializedProject(t, "aud01-legacy")
	setProjectSchemaVersion(t, ref.Root, 0)

	resolved, err := hookactions.ResolveAt(ref.Root, ref.Home)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProjectID != ref.ProjectID || resolved.Name != ref.Name {
		t.Fatalf("resolved=%#v want project_id=%q name=%q", resolved, ref.ProjectID, ref.Name)
	}
}

func setProjectSchemaVersion(t *testing.T, root string, version int) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(config.FileConfig))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc config.ProjectDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc.SchemaVersion = version
	raw, err = yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
