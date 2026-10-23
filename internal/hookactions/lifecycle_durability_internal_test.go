package hookactions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestLifecycle_PendingFileSyncFailurePreservesPrimary(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("schema_version: 1\npending_generation: nope\n")
	writeStab(t, home, project, maintainPendingName, raw)
	fsafety.SetDurableSyncForTest(func(*os.File) error { return errors.New("injected file sync") }, nil)
	t.Cleanup(func() { fsafety.SetDurableSyncForTest(nil, nil) })

	_, err := writeCorruptEvidenceExclusiveProd(home, project, raw)
	if err == nil {
		t.Fatal("file sync failure returned nil")
	}
	if got := readStab(t, home, project, maintainPendingName); !bytes.Equal(got, raw) {
		t.Fatalf("primary=%q", got)
	}
	if n := len(stabEvidence(t, home, project, maintainPendingCorruptPrefix)); n != 0 {
		t.Fatalf("partial evidence files=%d", n)
	}
}

func TestLifecycle_PendingDirSyncFailurePreservesPrimary(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("schema_version: 1\npending_generation: nope\n")
	writeStab(t, home, project, maintainPendingName, raw)
	fsafety.SetDurableSyncForTest(nil, func(string) error { return errors.New("injected dir sync") })
	t.Cleanup(func() { fsafety.SetDurableSyncForTest(nil, nil) })

	_, err := writeCorruptEvidenceExclusiveProd(home, project, raw)
	if err == nil {
		t.Fatal("dir sync failure returned nil")
	}
	if got := readStab(t, home, project, maintainPendingName); !bytes.Equal(got, raw) {
		t.Fatalf("primary=%q", got)
	}
	if n := len(stabEvidence(t, home, project, maintainPendingCorruptPrefix)); n != 0 {
		t.Fatalf("partial evidence files=%d", n)
	}
}

func TestLifecycle_ResetFailureAfterEvidenceKeepsCorruptPrimary(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("schema_version: 1\npending_generation: nope\n")
	writeStab(t, home, project, maintainPendingName, raw)
	writeRecoveryPendingPrimary = func(string, string, []byte) error {
		return errors.New("injected reset failure")
	}
	t.Cleanup(func() { writeRecoveryPendingPrimary = writeRecoveryPendingPrimaryProd })

	err := QuarantineAndResetCorruptPending(home, project)
	if err == nil {
		t.Fatal("reset failure returned nil")
	}
	if got := readStab(t, home, project, maintainPendingName); !bytes.Equal(got, raw) {
		t.Fatalf("primary=%q", got)
	}
	if preserved := stabEvidenceBytes(t, home, project, maintainPendingCorruptPrefix); !bytes.Equal(preserved, raw) {
		t.Fatal("evidence lost the original bytes")
	}
}

func TestLifecycle_DurablePendingRecoveryPreservesEvidence(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("schema_version: 1\npending_generation: nope\n")
	writeStab(t, home, project, maintainPendingName, raw)
	if err := QuarantineAndResetCorruptPending(home, project); err != nil {
		t.Fatal(err)
	}
	if preserved := stabEvidenceBytes(t, home, project, maintainPendingCorruptPrefix); !bytes.Equal(preserved, raw) {
		t.Fatal("durable recovery did not keep the original bytes")
	}
	doc, err := loadPending(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("epoch=%d/%d", doc.PendingGeneration, doc.ProcessedGeneration)
	}
	if _, err := os.Lstat(filepath.Join(home, "projects", project, "state", maintainPendingName)); err != nil {
		t.Fatal(err)
	}
}
