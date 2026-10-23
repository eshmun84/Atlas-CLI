package hookactions_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
)

func TestSlice02_MaintainLast_LaterWriterReplacesEarlier(t *testing.T) {
	homePath := t.TempDir()
	projectID := "slice02-maintain"

	_, started := hookactions.BeginMaintainLastForTest(homePath, projectID, "sess-a", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hookactions.FinishMaintainLastForTest(homePath, projectID, "sess-a", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", started,
		hookactions.MaintainOutcomeFailed, hookactions.MaintainOutcomeSkipped, hookactions.MaintainErrCEFailed)

	first, ok, err := hookactions.LoadMaintainLast(homePath, projectID)
	if err != nil || !ok {
		t.Fatalf("first record ok=%v err=%v", ok, err)
	}
	if first.SessionID != "sess-a" || first.ErrorCode != hookactions.MaintainErrCEFailed || first.Status != hookactions.MaintainStatusComplete {
		t.Fatalf("first record = %#v", first)
	}

	_, started = hookactions.BeginMaintainLastForTest(homePath, projectID, "sess-b", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	hookactions.FinishMaintainLastForTest(homePath, projectID, "sess-b", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", started,
		hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone)

	second, ok, err := hookactions.LoadMaintainLast(homePath, projectID)
	if err != nil || !ok {
		t.Fatalf("second record ok=%v err=%v", ok, err)
	}
	if second.SessionID != "sess-b" || second.AdmissionID != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("later writer did not replace the snapshot: %#v", second)
	}
	if second.ErrorCode != hookactions.MaintainErrNone || second.Status != hookactions.MaintainStatusComplete {
		t.Fatalf("later result = %#v", second)
	}
	if second.SchemaVersion != 1 {
		t.Fatalf("schema = %d", second.SchemaVersion)
	}
}
