package codeintel_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestRefresh_RootFingerprintError_PreservesMetadata(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	stub := &refreshStub{cap: codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0",
	}}
	svc := codeintel.NewService(stub)
	fixed := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	out, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{
		Root: root, ProjectName: "demo", Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeMeta, err := os.ReadFile(out.MetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var before codeintel.Metadata
	if err := json.Unmarshal(beforeMeta, &before); err != nil {
		t.Fatal(err)
	}
	if before.ProjectRootIdentity == "" {
		t.Fatal("expected prior known-good root identity")
	}

	mustWrite(t, filepath.Join(root, "a.go"), "package a\nfunc Y(){}\n")
	codeintel.SetRootFingerprintForTest(func(string) (string, error) {
		return "", errors.New("injected root fingerprint failure")
	})
	t.Cleanup(func() { codeintel.SetRootFingerprintForTest(nil) })

	out2, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{
		Root: root, ProjectName: "demo", ForceFull: true,
		Now: func() time.Time { return fixed.Add(time.Minute) },
	})
	if err == nil || !strings.Contains(err.Error(), "project root identity") {
		t.Fatalf("expected root identity error, got %v", err)
	}
	if out2.State != codeintel.StateError {
		t.Fatalf("state=%v want error", out2.State)
	}
	afterMeta, err := os.ReadFile(out.MetadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterMeta) != string(beforeMeta) {
		t.Fatalf("metadata mutated on fingerprint failure\nbefore=%s\nafter=%s", beforeMeta, afterMeta)
	}
	if out2.Metadata.ProjectRootIdentity != before.ProjectRootIdentity {
		t.Fatalf("outcome metadata identity=%q want %q", out2.Metadata.ProjectRootIdentity, before.ProjectRootIdentity)
	}
}
