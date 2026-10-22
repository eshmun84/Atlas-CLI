package hookactions_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func TestQA04_OpenCodeSessionIsolation(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	hookactions.SetSpawner(func(hookactions.ProjectRef) error { return nil })
	hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error { return nil })
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.SetSchedulerSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	a := seedInitializedProject(t, "iso-a")
	b := seedInitializedProject(t, "iso-b")
	write(t, filepath.Join(b.Home, "sentinel"), "b-home-untouched\n")
	before := hashTree(t, b.Home)

	capB := sessionCapableFor(adapters.OpenCode, hooks.EventSessionStart, hooks.EventAfterToolExecution)
	engB := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: b.Root, Home: b.Home},
		CapabilityOverride: &capB,
	}
	foreign := engB.Dispatch(context.Background(), openCodeSessionRequest(b, "ses_shared", a.Root))
	if foreign.Status != hooksengine.StatusProjectMismatch {
		t.Fatalf("foreign session status=%s %#v", foreign.Status, foreign)
	}
	if hashTree(t, b.Home) != before {
		t.Fatal("foreign session.created wrote the other project Home")
	}

	missing := engB.Dispatch(context.Background(), openCodeSessionRequest(b, "ses_missing", ""))
	if missing.Status != hooksengine.StatusProjectMismatch {
		t.Fatalf("missing directory status=%s", missing.Status)
	}
	if hashTree(t, b.Home) != before {
		t.Fatal("missing session directory wrote the project Home")
	}

	capA := sessionCapableFor(adapters.OpenCode, hooks.EventSessionStart, hooks.EventAfterToolExecution)
	engA := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: a.Root, Home: a.Home},
		CapabilityOverride: &capA,
	}
	owned := engA.Dispatch(context.Background(), openCodeSessionRequest(a, "ses_shared", a.Root))
	if owned.Status != hooksengine.StatusSuccess {
		t.Fatalf("owned session status=%s %#v", owned.Status, owned)
	}
	if _, err := os.Stat(hookactions.SessionMaintainPath(a.Home, a.ProjectID)); err != nil {
		t.Fatalf("owning project did not record the session: %v", err)
	}
	again := engA.Dispatch(context.Background(), openCodeSessionRequest(a, "ses_shared", a.Root))
	if again.Status != hooksengine.StatusSuccess {
		t.Fatalf("second owned session status=%s", again.Status)
	}
	if hashTree(t, b.Home) != before {
		t.Fatal("shared session id was admitted in the other project")
	}

	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(a.Root, link); err != nil {
		t.Fatal(err)
	}
	viaLink := engA.Dispatch(context.Background(), openCodeSessionRequest(a, "ses_link", link))
	if viaLink.Status != hooksengine.StatusSuccess {
		t.Fatalf("symlink directory status=%s %#v", viaLink.Status, viaLink)
	}

	toolForeign := engB.Dispatch(context.Background(), openCodeToolRequest(b, "ses_tool", a.Root))
	if toolForeign.Status != hooksengine.StatusProjectMismatch {
		t.Fatalf("foreign tool status=%s", toolForeign.Status)
	}
	if _, err := os.Stat(hookactions.MaintainPendingPath(b.Home, b.ProjectID)); !os.IsNotExist(err) {
		t.Fatalf("foreign tool.execute.after created pending state: %v", err)
	}
	if hashTree(t, b.Home) != before {
		t.Fatal("foreign Phase C wrote the other project Home")
	}
	toolOwned := engA.Dispatch(context.Background(), openCodeToolRequest(a, "ses_tool", a.Root))
	if toolOwned.Status != hooksengine.StatusSuccess {
		t.Fatalf("owned tool status=%s %#v", toolOwned.Status, toolOwned)
	}
	doc, err := hookactions.LoadMaintainPending(a.Home, a.ProjectID)
	if err != nil || doc.PendingGeneration != 1 {
		t.Fatalf("owned pending=%d err=%v", doc.PendingGeneration, err)
	}

	cursorCap := sessionCapableFor(adapters.Cursor, hooks.EventSessionStart)
	cursorEng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: a.Root, Home: a.Home},
		CapabilityOverride: &cursorCap,
	}
	cursor := cursorEng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: "cursor-session", Type: string(hooks.EventSessionStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload:    hooksengine.PayloadInput{ProjectID: a.ProjectID, SessionID: "cursor-sess"},
		},
	})
	if cursor.Status != hooksengine.StatusSuccess {
		t.Fatalf("cursor without session_directory status=%s %#v", cursor.Status, cursor)
	}
	if hashTree(t, b.Home) != before {
		t.Fatal("cursor dispatch wrote the other project Home")
	}
}

func openCodeSessionRequest(ref projectFixture, sessionID, directory string) hooksengine.Request {
	return hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.OpenCode),
		Event: hooksengine.EventInput{
			ID: "oc-" + sessionID, Type: string(hooks.EventSessionStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID:        ref.ProjectID,
				SessionID:        sessionID,
				SessionDirectory: directory,
			},
		},
	}
}

func openCodeToolRequest(ref projectFixture, sessionID, directory string) hooksengine.Request {
	return hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.OpenCode),
		Event: hooksengine.EventInput{
			ID: "tool-" + sessionID, Type: string(hooks.EventAfterToolExecution),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID:        ref.ProjectID,
				SessionID:        sessionID,
				ToolName:         "write",
				Status:           "success",
				SessionDirectory: directory,
			},
		},
	}
}

func hashTree(t *testing.T, root string) string {
	t.Helper()
	sum := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = sum.Write([]byte(rel))
		_, _ = sum.Write([]byte{0})
		if info.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = sum.Write(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(sum.Sum(nil))
}
