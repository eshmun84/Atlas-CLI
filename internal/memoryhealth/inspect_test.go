package memoryhealth_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryhealth"

	_ "modernc.org/sqlite"
)

const testProjectID = "demo-abcdef0123456789"

func baseInput(home string) memoryhealth.Input {
	return memoryhealth.Input{
		Root:            "/tmp/project",
		HomePath:        home,
		ProjectID:       testProjectID,
		Initialized:     true,
		ConfigLoads:     true,
		MemoryEnabled:   true,
		PersistenceMode: config.MemoryPersistenceGoverned,
	}
}

func openFreshDB(t *testing.T) (home string, store *memory.Store) {
	t.Helper()
	home = t.TempDir()
	store, err := memory.Open(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return home, store
}

func saveObs(t *testing.T, s *memory.Store, title, content string, typ memory.ObservationType) {
	t.Helper()
	_, err := s.SaveObservation(memory.Observation{
		ProjectID: testProjectID,
		Type:      typ,
		Scope:     memory.ScopeProject,
		Title:     title,
		Content:   content,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkByID(d memoryhealth.Diagnostics, id string) (memoryhealth.Check, bool) {
	for _, c := range d.Checks {
		if c.ID == id {
			return c, true
		}
	}
	return memoryhealth.Check{}, false
}

func TestInspect_Disabled(t *testing.T) {
	d := memoryhealth.Inspect(memoryhealth.Input{
		Initialized:   true,
		ConfigLoads:   true,
		MemoryEnabled: false,
	})
	if d.Status != memoryhealth.StatusUnavailable {
		t.Fatalf("status=%s", d.Status)
	}
	c, ok := checkByID(d, memoryhealth.CheckConfigEnabled)
	if !ok || c.Severity != memoryhealth.SeverityInfo {
		t.Fatalf("enabled check=%+v ok=%v", c, ok)
	}
}

func TestInspect_NotInitialized(t *testing.T) {
	d := memoryhealth.Inspect(memoryhealth.Input{Initialized: false})
	if d.Status != memoryhealth.StatusUnavailable {
		t.Fatalf("status=%s", d.Status)
	}
}

func TestInspect_InvalidPersistenceMode(t *testing.T) {
	d := memoryhealth.Inspect(memoryhealth.Input{
		Initialized:     true,
		ConfigLoads:     true,
		MemoryEnabled:   true,
		PersistenceMode: "aggressive",
	})
	if d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s", d.Status)
	}
	c, ok := checkByID(d, memoryhealth.CheckConfigPersistenceMode)
	if !ok || c.Severity != memoryhealth.SeverityFail {
		t.Fatalf("mode check=%+v", c)
	}
}

func TestInspect_GovernedMissingDB(t *testing.T) {
	home := t.TempDir()
	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusUnavailable && d.Status != memoryhealth.StatusDegraded {
		t.Fatalf("status=%s", d.Status)
	}
	c, ok := checkByID(d, memoryhealth.CheckStorageExists)
	if !ok || c.Status != "missing" {
		t.Fatalf("exists=%+v", c)
	}
	if d.Metrics.ObservationCount != 0 {
		t.Fatalf("count=%d", d.Metrics.ObservationCount)
	}
}

func TestInspect_PersistenceOffMissingDB(t *testing.T) {
	home := t.TempDir()
	in := baseInput(home)
	in.PersistenceMode = config.MemoryPersistenceOff
	d := memoryhealth.Inspect(in)
	if d.Status != memoryhealth.StatusUnavailable {
		t.Fatalf("status=%s", d.Status)
	}
	if d.PersistenceLabel != "off" {
		t.Fatalf("persistence=%s", d.PersistenceLabel)
	}
}

func TestInspect_HealthyGoverned(t *testing.T) {
	home, store := openFreshDB(t)
	saveObs(t, store, "Prefer SQLite Memory", "Use governed persistence for durable decisions.", memory.TypeDecision)
	_ = store.Close()

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusHealthy {
		t.Fatalf("status=%s checks=%+v", d.Status, d.Checks)
	}
	if d.StorageLabel != "ready" || d.SchemaLabel != "compatible" || d.RetrievalLabel != "ready" {
		t.Fatalf("labels storage=%s schema=%s retrieval=%s", d.StorageLabel, d.SchemaLabel, d.RetrievalLabel)
	}
	if d.Metrics.ObservationCount != 1 || d.Metrics.DecisionCount != 1 {
		t.Fatalf("metrics=%+v", d.Metrics)
	}
	if d.Metrics.AnomalyCount != 0 || d.QualityLabel != "0 anomalies" {
		t.Fatalf("quality=%s anomalies=%d", d.QualityLabel, d.Metrics.AnomalyCount)
	}
	if !strings.Contains(d.PersistenceLabel, "governed") {
		t.Fatalf("persistence=%s", d.PersistenceLabel)
	}
}

func TestInspect_SymlinkDBFailed(t *testing.T) {
	home, store := openFreshDB(t)
	rel, err := memory.DatabaseRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(home, filepath.FromSlash(rel))
	_ = store.Close()
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "outside.db")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, abs); err != nil {
		t.Fatal(err)
	}

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s", d.Status)
	}
	c, ok := checkByID(d, memoryhealth.CheckStorageSymlinkSafe)
	if !ok || c.Severity != memoryhealth.SeverityFail {
		t.Fatalf("symlink check=%+v", c)
	}
}

func TestInspect_SchemaIncompatibleFailed(t *testing.T) {
	home, store := openFreshDB(t)
	dbPath := mustDBPath(t, home)
	_ = store.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO schema_migrations(version, applied_at) VALUES (999, ?)`,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s", d.Status)
	}
	c, ok := checkByID(d, memoryhealth.CheckSchemaCompatible)
	if !ok || c.Severity != memoryhealth.SeverityFail {
		t.Fatalf("schema check=%+v ok=%v status=%s", c, ok, d.Status)
	}
}

func TestInspect_MissingMigrationTableFailed(t *testing.T) {
	home := t.TempDir()
	rel, err := memory.MemoryDirRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, memory.FileMemoryDB)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated(id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s", d.Status)
	}
}

func TestInspect_CorruptDBFailed(t *testing.T) {
	home := t.TempDir()
	rel, err := memory.MemoryDirRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, memory.FileMemoryDB)
	if err := os.WriteFile(dbPath, []byte("not-a-sqlite-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s", d.Status)
	}
}

func TestInspect_ReadOnlyDBDegradedWhenGoverned(t *testing.T) {
	home, store := openFreshDB(t)
	saveObs(t, store, "Read only case", "Durable fact for diagnostics.", memory.TypeFact)
	rel, err := memory.DatabaseRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(home, filepath.FromSlash(rel))
	_ = store.Close()
	if err := os.Chmod(abs, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(abs, 0o600) })

	d := memoryhealth.Inspect(baseInput(home))
	// Read-only file may still open RO successfully; writability should degrade.
	if d.Status != memoryhealth.StatusDegraded && d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s want degraded|failed checks=%+v", d.Status, d.Checks)
	}
	if d.Status == memoryhealth.StatusDegraded {
		c, ok := checkByID(d, memoryhealth.CheckStorageWritable)
		if !ok || c.Severity != memoryhealth.SeverityWarn {
			t.Fatalf("writable check=%+v", c)
		}
	}
}

func TestInspect_UnreadableDBFailed(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	home, store := openFreshDB(t)
	rel, err := memory.DatabaseRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(home, filepath.FromSlash(rel))
	_ = store.Close()
	if err := os.Chmod(abs, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(abs, 0o600) })

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusFailed {
		t.Fatalf("status=%s", d.Status)
	}
}

func TestInspect_MalformedQualityDegraded(t *testing.T) {
	home, store := openFreshDB(t)
	saveObs(t, store, "Valid seed", "Keep schema healthy before injecting bad row.", memory.TypeFact)
	dbPath := mustDBPath(t, home)
	_ = store.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
INSERT INTO memory_observations(
  id, project_id, type, scope, title, content, content_hash,
  duplicate_count, revision_count, first_seen_at, last_seen_at, created_at, updated_at
) VALUES (?, ?, 'fact', 'project', '', 'body', 'hash-malformed-1', 1, 0, ?, ?, ?, ?)`,
		"obs_malformed_1", testProjectID, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusDegraded {
		t.Fatalf("status=%s metrics=%+v", d.Status, d.Metrics)
	}
	if d.Metrics.MalformedCount < 1 {
		t.Fatalf("malformed=%d", d.Metrics.MalformedCount)
	}
	c, ok := checkByID(d, memoryhealth.CheckDataRequiredFields)
	if !ok || c.Severity != memoryhealth.SeverityWarn {
		t.Fatalf("required fields=%+v", c)
	}
}

func TestInspect_InvalidScopeDegraded(t *testing.T) {
	home, store := openFreshDB(t)
	dbPath := mustDBPath(t, home)
	_ = store.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`
INSERT INTO memory_observations(
  id, project_id, type, scope, title, content, content_hash,
  duplicate_count, revision_count, first_seen_at, last_seen_at, created_at, updated_at
) VALUES (?, ?, 'fact', 'global', 'Bad scope', 'Should not exist in V1', 'hash-scope-1', 1, 0, ?, ?, ?, ?)`,
		"obs_scope_1", testProjectID, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusDegraded {
		t.Fatalf("status=%s", d.Status)
	}
	if d.Metrics.InvalidScopeCount < 1 {
		t.Fatalf("invalid_scope=%d", d.Metrics.InvalidScopeCount)
	}
}

func TestInspect_DuplicateAnomalyDegraded(t *testing.T) {
	home, store := openFreshDB(t)
	dbPath := mustDBPath(t, home)
	_ = store.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Drop unique index so we can insert a true hash collision anomaly.
	if _, err := db.Exec(`DROP INDEX IF EXISTS memory_observations_hash_idx`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, id := range []string{"obs_dup_a", "obs_dup_b"} {
		_, err := db.Exec(`
INSERT INTO memory_observations(
  id, project_id, type, scope, title, content, content_hash,
  duplicate_count, revision_count, first_seen_at, last_seen_at, created_at, updated_at
) VALUES (?, ?, 'fact', 'project', ?, 'same', 'same-hash', 1, 0, ?, ?, ?, ?)`,
			id, testProjectID, "Dup "+id, now, now, now, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusDegraded {
		t.Fatalf("status=%s metrics=%+v", d.Status, d.Metrics)
	}
	if d.Metrics.DuplicateAnomalyCount < 1 {
		t.Fatalf("anomaly=%d", d.Metrics.DuplicateAnomalyCount)
	}
}

func TestInspect_ZeroObservationsHealthy(t *testing.T) {
	home, store := openFreshDB(t)
	_ = store.Close()
	d := memoryhealth.Inspect(baseInput(home))
	if d.Status != memoryhealth.StatusHealthy {
		t.Fatalf("status=%s", d.Status)
	}
	if d.Metrics.ObservationCount != 0 {
		t.Fatalf("count=%d", d.Metrics.ObservationCount)
	}
}

func TestInspect_IsolationProjectANeverSeesB(t *testing.T) {
	home := t.TempDir()
	projectA := "proj-aaaaaaaaaaaaaaaa"
	projectB := "proj-bbbbbbbbbbbbbbbb"

	storeA, err := memory.Open(home, projectA)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storeA.SaveObservation(memory.Observation{
		ProjectID: projectA,
		Type:      memory.TypeDecision,
		Scope:     memory.ScopeProject,
		Title:     "Secret for A",
		Content:   "project A only secret token SUPERSECRET_A",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = storeA.Close()

	storeB, err := memory.Open(home, projectB)
	if err != nil {
		t.Fatal(err)
	}
	_, err = storeB.SaveObservation(memory.Observation{
		ProjectID: projectB,
		Type:      memory.TypeFact,
		Scope:     memory.ScopeProject,
		Title:     "Fact for B",
		Content:   "project B content",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = storeB.Close()

	inA := baseInput(home)
	inA.ProjectID = projectA
	dA := memoryhealth.Inspect(inA)
	if dA.Metrics.ObservationCount != 1 {
		t.Fatalf("A count=%d", dA.Metrics.ObservationCount)
	}

	inB := baseInput(home)
	inB.ProjectID = projectB
	dB := memoryhealth.Inspect(inB)
	if dB.Metrics.ObservationCount != 1 {
		t.Fatalf("B count=%d", dB.Metrics.ObservationCount)
	}
	if dA.Metrics.DecisionCount != 1 || dB.Metrics.DecisionCount != 0 {
		t.Fatalf("cross-project leak A=%+v B=%+v", dA.Metrics, dB.Metrics)
	}
}

func TestInspect_NoSecretLeakageOrHomePaths(t *testing.T) {
	home, store := openFreshDB(t)
	secret := "sk-live-SUPERSECRETTOKEN123"
	saveObs(t, store, "Contains secret", "token="+secret, memory.TypeConfig)
	_ = store.Close()

	d := memoryhealth.Inspect(baseInput(home))
	var b strings.Builder
	b.WriteString(string(d.Status))
	b.WriteString(d.StorageLabel)
	b.WriteString(d.SchemaLabel)
	b.WriteString(d.RetrievalLabel)
	b.WriteString(d.PersistenceLabel)
	b.WriteString(d.QualityLabel)
	b.WriteString(d.StorageRel)
	for _, c := range d.Checks {
		b.WriteString(c.Message)
		b.WriteString(c.RemediationHint)
		for _, v := range c.Metadata {
			b.WriteString(v)
		}
	}
	out := b.String()
	if strings.Contains(out, secret) {
		t.Fatal("diagnostics leaked observation secret content")
	}
	if strings.Contains(out, home) {
		t.Fatalf("diagnostics leaked absolute home path: %s", out)
	}
}

func TestInspect_ReadOnlyDoesNotMutate(t *testing.T) {
	home, store := openFreshDB(t)
	saveObs(t, store, "Fingerprint seed", "Deterministic content for RO certification.", memory.TypeFact)
	_ = store.Close()

	before := fingerprintMemoryTree(t, home)
	_ = memoryhealth.Inspect(baseInput(home))
	_ = memoryhealth.Inspect(baseInput(home))
	after := fingerprintMemoryTree(t, home)
	if before != after {
		t.Fatalf("diagnostics mutated Memory tree\nbefore=%s\nafter=%s", before, after)
	}
}

func mustDBPath(t *testing.T, home string) string {
	t.Helper()
	rel, err := memory.DatabaseRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, filepath.FromSlash(rel))
}

func fingerprintMemoryTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b.WriteString(rel)
		b.WriteByte('|')
		b.WriteString(info.Mode().String())
		b.WriteByte('|')
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.WriteString(strings.ReplaceAll(string(data), "\x00", ""))
		}
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
