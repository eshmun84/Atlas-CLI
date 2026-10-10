package memory

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const testProjectID = "demo-abcdef0123456789"

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	home := t.TempDir()
	s, err := Open(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, home
}

func openTimedStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	home := t.TempDir()
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	s, err := OpenWithOptions(home, testProjectID, Options{
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, &now
}

func TestFTS5DriverAvailable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(title, content)`); err != nil {
		t.Fatalf("FTS5 unavailable with modernc.org/sqlite: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(title, content) VALUES ('CodeGraph', 'provider')`); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM t WHERE t MATCH ?`, "CodeGraph").Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "CodeGraph" {
		t.Fatalf("got %q", title)
	}
}

func TestOpen_FreshMigrationsAndReopen(t *testing.T) {
	s, home := openTestStore(t)
	ver, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if ver != CurrentSchemaVersion {
		t.Fatalf("version=%d", ver)
	}
	fk, err := s.ForeignKeysEnabled()
	if err != nil || !fk {
		t.Fatalf("foreign_keys=%v err=%v", fk, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	ver2, err := s2.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if ver2 != ver {
		t.Fatalf("reopen mutated version %d -> %d", ver, ver2)
	}
}

func TestOpen_NewerSchemaFails(t *testing.T) {
	s, home := openTestStore(t)
	db := s.DBForTest()
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES (999, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if _, err := Open(home, testProjectID); err == nil || !strings.Contains(err.Error(), "unsupported newer") {
		t.Fatalf("expected newer schema fail, got %v", err)
	}
	if _, err := OpenReadOnly(home, testProjectID); err == nil || !strings.Contains(err.Error(), "unsupported newer") {
		t.Fatalf("readonly expected newer schema fail, got %v", err)
	}
}

func TestExists_DoesNotCreate(t *testing.T) {
	home := t.TempDir()
	ok, err := Exists(home, testProjectID)
	if err != nil || ok {
		t.Fatalf("exists=%v err=%v", ok, err)
	}
	rel, err := MemoryDirRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("memory dir must not be created by Exists: %v", err)
	}
}

func TestOpenReadOnly_MissingFails(t *testing.T) {
	home := t.TempDir()
	if _, err := OpenReadOnly(home, testProjectID); err == nil {
		t.Fatal("expected missing db error")
	}
	ok, _ := Exists(home, testProjectID)
	if ok {
		t.Fatal("read-only miss must not create db")
	}
	rel, _ := MemoryDirRel(testProjectID)
	if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly miss must not create memory dir: %v", err)
	}
}

func TestOpenReadOnly_ValidDB(t *testing.T) {
	s, home := openTestStore(t)
	_, err := s.SaveObservation(Observation{Type: TypeFact, Title: "t", Content: "c"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	ro, err := OpenReadOnly(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	list, err := ro.ListObservations(ListFilter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := ro.SaveObservation(Observation{Type: TypeFact, Title: "x", Content: "y"}); err == nil {
		t.Fatal("SaveObservation must fail on read-only store")
	}
}

func TestOpenReadOnly_NoSchemaDoesNotCreateMigrations(t *testing.T) {
	home := t.TempDir()
	rel, err := DatabaseRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	dirRel, err := MemoryDirRel(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(dirRel)), 0o700); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(home, filepath.FromSlash(rel))
	// Empty SQLite file with no schema_migrations.
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated(x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	before := fileFingerprint(t, abs)

	_, err = OpenReadOnly(home, testProjectID)
	if err == nil || !strings.Contains(err.Error(), "schema_migrations missing") {
		t.Fatalf("expected missing schema error, got %v", err)
	}
	after := fileFingerprint(t, abs)
	if before != after {
		t.Fatalf("OpenReadOnly mutated DB file: before=%s after=%s", before, after)
	}

	// Confirm schema_migrations was not created.
	db2, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	var n int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='schema_migrations'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("schema_migrations must not be created by OpenReadOnly")
	}
}

func TestOpenReadOnly_WriteSQLFailsAndNoMutation(t *testing.T) {
	s, home := openTestStore(t)
	_ = s.Close()
	dbRel, _ := DatabaseRel(testProjectID)
	abs := filepath.Join(home, filepath.FromSlash(dbRel))
	before := fileFingerprint(t, abs)

	ro, err := OpenReadOnly(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()

	if _, err := ro.DBForTest().Exec(`INSERT INTO memory_observations(
		id, project_id, type, scope, title, content, content_hash, duplicate_count, revision_count,
		first_seen_at, last_seen_at, created_at, updated_at
	) VALUES ('x','p','fact','project','t','c','h',1,0,'t','t','t','t')`); err == nil {
		t.Fatal("raw INSERT through read-only connection must fail")
	}
	if _, err := ro.BeginSession(); err == nil {
		t.Fatal("BeginSession must fail read-only")
	}

	after := fileFingerprint(t, abs)
	if before != after {
		t.Fatalf("DB mutated under OpenReadOnly: before=%s after=%s", before, after)
	}
}

func fileFingerprint(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d:%x", info.Size(), data)
}

func TestObservation_InsertGetValidate(t *testing.T) {
	s, _ := openTimedStore(t)
	res, err := s.SaveObservation(Observation{
		Type:    TypeFact,
		Title:   "CodeGraph",
		Content: "CodeGraph is the Code Intelligence provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != SaveCreated {
		t.Fatalf("action=%s", res.Action)
	}
	got, err := s.GetObservation(res.Observation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "CodeGraph" || got.Scope != ScopeProject || got.DuplicateCount != 1 {
		t.Fatalf("%#v", got)
	}
	if _, err := s.SaveObservation(Observation{Type: "nope", Title: "t", Content: "c"}); err == nil {
		t.Fatal("invalid type")
	}
	if _, err := s.SaveObservation(Observation{Type: TypeFact, Scope: "personal", Title: "t", Content: "c"}); err == nil {
		t.Fatal("invalid scope")
	}
	if _, err := s.SaveObservation(Observation{Type: TypeFact, Scope: "global", Title: "t", Content: "c"}); err == nil {
		t.Fatal("global scope deferred / rejected in V1")
	}
	if _, err := s.SaveObservation(Observation{Type: TypeFact, Title: "", Content: "c"}); err == nil {
		t.Fatal("empty title")
	}
}

func TestDedupe_Exact(t *testing.T) {
	s, now := openTimedStore(t)
	in := Observation{
		Type: TypeDiscovery, Title: "T", Content: "same body",
	}
	a, err := s.SaveObservation(in)
	if err != nil {
		t.Fatal(err)
	}
	first := a.Observation.FirstSeenAt
	*now = now.Add(time.Minute)
	b, err := s.SaveObservation(in)
	if err != nil {
		t.Fatal(err)
	}
	if b.Action != SaveDuplicate || b.Observation.ID != a.Observation.ID {
		t.Fatalf("%#v", b)
	}
	if b.Observation.DuplicateCount != 2 {
		t.Fatalf("dup=%d", b.Observation.DuplicateCount)
	}
	if !b.Observation.FirstSeenAt.Equal(first) {
		t.Fatal("first_seen mutated")
	}
	if !b.Observation.LastSeenAt.After(first) {
		t.Fatal("last_seen must advance")
	}
	list, err := s.ListObservations(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("rows=%d", len(list))
	}
}

func TestTopic_UpsertAndIsolation(t *testing.T) {
	s, now := openTimedStore(t)
	a, err := s.SaveObservation(Observation{
		Type: TypeArchitecture, Title: "Skills v1", Content: "skills are Home-canonical",
		TopicKey: "architecture/skills",
	})
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	b, err := s.SaveObservation(Observation{
		Type: TypeArchitecture, Title: "Skills v2", Content: "skills pins are exact",
		TopicKey: "architecture/skills",
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Action != SaveUpdated || b.Observation.ID != a.Observation.ID {
		t.Fatalf("%#v", b)
	}
	if b.Observation.RevisionCount != 1 {
		t.Fatalf("rev=%d", b.Observation.RevisionCount)
	}
	*now = now.Add(time.Minute)
	c, err := s.SaveObservation(Observation{
		Type: TypeArchitecture, Title: "Skills v2", Content: "skills pins are exact",
		TopicKey: "architecture/skills",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Action != SaveDuplicate {
		t.Fatalf("action=%s", c.Action)
	}

	// Different topic must not collide.
	d, err := s.SaveObservation(Observation{
		Type: TypeArchitecture, Title: "Other", Content: "other note",
		TopicKey: "architecture/other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Observation.ID == a.Observation.ID {
		t.Fatal("topic collision")
	}
}

func TestSessions(t *testing.T) {
	s, _ := openTimedStore(t)
	sess, err := s.BeginSession()
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.SaveObservation(Observation{
		Type: TypeFact, Title: "linked", Content: "with session", SessionID: sess.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Observation.SessionID != sess.ID {
		t.Fatal("session not associated")
	}
	ended, err := s.EndSession(sess.ID, "optional summary")
	if err != nil {
		t.Fatal(err)
	}
	if ended.EndedAt == nil || ended.Summary != "optional summary" {
		t.Fatalf("%#v", ended)
	}
	if _, err := s.EndSession("missing", ""); err == nil {
		t.Fatal("missing session")
	}
	// Already closed: explicit no-op success.
	if _, err := s.EndSession(sess.ID, ""); err != nil {
		t.Fatal(err)
	}
}

func TestFTS_SearchAndUpdate(t *testing.T) {
	s, now := openTimedStore(t)
	_, err := s.SaveObservation(Observation{
		Type: TypeFact, Title: "Provider", Content: "CodeGraph is the Code Intelligence provider",
		TopicKey: "architecture/code-intelligence",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(SearchQuery{Query: "CodeGraph"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	*now = now.Add(time.Minute)
	_, err = s.SaveObservation(Observation{
		Type: TypeFact, Title: "Provider", Content: "replaced body without prior token",
		TopicKey: "architecture/code-intelligence",
	})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.Search(SearchQuery{Query: "CodeGraph"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("stale FTS hits: %#v", stale)
	}
	fresh, err := s.Search(SearchQuery{Query: "replaced"})
	if err != nil || len(fresh) != 1 {
		t.Fatalf("fresh=%v err=%v", fresh, err)
	}
	if _, err := s.Search(SearchQuery{Query: `"`}); err == nil {
		t.Fatal("malformed query should error cleanly")
	}
}

func TestSafety_SymlinksAndTraversal(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	pid := testProjectID

	// Symlink memory dir.
	proj := filepath.Join(home, "projects", pid)
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(proj, "memory")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(home, pid); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("memory dir symlink: %v", err)
	}

	home2 := t.TempDir()
	rel, _ := MemoryDirRel(pid)
	if err := os.MkdirAll(filepath.Join(home2, filepath.FromSlash(rel)), 0o700); err != nil {
		t.Fatal(err)
	}
	dbRel, _ := DatabaseRel(pid)
	if err := os.Symlink(filepath.Join(outside, "x.db"), filepath.Join(home2, filepath.FromSlash(dbRel))); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(home2, pid); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("db symlink: %v", err)
	}

	if _, err := DatabaseRel("../escape"); err == nil {
		t.Fatal("traversal project id")
	}
	if _, err := Open(home2, "x/../y"); err == nil {
		t.Fatal("traversal open")
	}
}

func TestMigrationFailureDoesNotAdvanceVersion(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "t.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := EnsureMigrationsTableForTest(db); err != nil {
		t.Fatal(err)
	}
	err = ApplyOneMigrationForTest(db, 1, "bad", `CREATE TABLE ok(x INT); CREATE TABLE !!!;`, time.Now().UTC())
	if err == nil {
		t.Fatal("expected migration failure")
	}
	ver, err := CurrentDBVersionForTest(db)
	if err != nil {
		t.Fatal(err)
	}
	if ver != 0 {
		t.Fatalf("version advanced to %d after failure", ver)
	}
}

func TestConcurrentSaves(t *testing.T) {
	s, _ := openTestStore(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.SaveObservation(Observation{
				Type:    TypeFact,
				Title:   fmt.Sprintf("t-%d", i),
				Content: fmt.Sprintf("body-%d", i),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListObservations(ListFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 20 {
		t.Fatalf("got %d", len(list))
	}
}

func TestTransaction_FTSFailureRollsBackObservation(t *testing.T) {
	s, _ := openTimedStore(t)
	db := s.DBForTest()
	// Break FTS so insert sync fails after observation write attempt in same tx.
	if _, err := db.Exec(`DROP TABLE memory_observations_fts`); err != nil {
		t.Fatal(err)
	}
	_, err := s.SaveObservation(Observation{
		Type: TypeFact, Title: "x", Content: "y",
	})
	if err == nil {
		t.Fatal("expected fts failure")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memory_observations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("observation leaked without FTS: %d", n)
	}
}

func TestSqliteReadOnlyDSN_FileURI(t *testing.T) {
	abs := "/var/db/memory.db"
	dsn := sqliteReadOnlyDSN(abs)
	if !strings.HasPrefix(dsn, "file:") {
		t.Fatalf("expected file: URI, got %q", dsn)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "file" {
		t.Fatalf("scheme=%q", u.Scheme)
	}
	if u.Path != abs {
		t.Fatalf("path=%q want %q", u.Path, abs)
	}
	if u.RawQuery != "mode=ro" || u.Query().Get("mode") != "ro" {
		t.Fatalf("query=%q", u.RawQuery)
	}
	if want := "file:///var/db/memory.db?mode=ro"; dsn != want {
		t.Fatalf("dsn=%q want %q", dsn, want)
	}
}

func TestSqliteReadOnlyDSN_SpecialPathCharsPreserveQuery(t *testing.T) {
	abs := "/tmp/my home/#proj/q?dir/memory.db"
	dsn := sqliteReadOnlyDSN(abs)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "file" {
		t.Fatalf("scheme=%q", u.Scheme)
	}
	if u.Path != abs {
		t.Fatalf("path=%q want %q (fragment/query must stay in path)", u.Path, abs)
	}
	if u.Fragment != "" {
		t.Fatalf("fragment leaked: %q", u.Fragment)
	}
	if u.RawQuery != "mode=ro" || u.Query().Get("mode") != "ro" {
		t.Fatalf("mode=ro corrupted: raw=%q", u.RawQuery)
	}
	if !strings.Contains(dsn, "%20") || !strings.Contains(dsn, "%23") || !strings.Contains(dsn, "%3F") {
		t.Fatalf("expected escaped spaces/#/?: %q", dsn)
	}
	if strings.Count(dsn, "?") != 1 {
		t.Fatalf("exactly one query delimiter expected: %q", dsn)
	}
}

func TestOpenReadOnly_PathWithSpaces(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "atlas home with spaces")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveObservation(Observation{Type: TypeFact, Title: "t", Content: "c"}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	ro, err := OpenReadOnly(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	list, err := ro.ListObservations(ListFilter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
}

func TestOpenReadOnly_PathWithHash(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home#hash")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	ro, err := OpenReadOnly(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	dsn := sqliteReadOnlyDSN(mustDBAbs(t, home, testProjectID))
	if !strings.Contains(dsn, "%23") {
		t.Fatalf("expected # escaped in DSN: %q", dsn)
	}
}

func TestOpenReadOnly_ModeROAndQueryOnly(t *testing.T) {
	s, home := openTestStore(t)
	_ = s.Close()
	dbRel, _ := DatabaseRel(testProjectID)
	abs := filepath.Join(home, filepath.FromSlash(dbRel))
	before := fileFingerprint(t, abs)

	ro, err := OpenReadOnly(home, testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()

	db := ro.DBForTest()
	var qo int
	if err := db.QueryRow(`PRAGMA query_only`).Scan(&qo); err != nil {
		t.Fatal(err)
	}
	if qo != 1 {
		t.Fatalf("query_only=%d want 1", qo)
	}

	// Bypass Store.requireWrite: raw SQL must still fail under mode=ro / query_only.
	_, err = db.Exec(`CREATE TABLE should_not_exist(x INTEGER)`)
	if err == nil {
		t.Fatal("mode=ro/query_only must reject CREATE")
	}
	_, err = db.Exec(`INSERT INTO memory_sessions(id, project_id, started_at, created_at, updated_at)
		VALUES ('s','p','t','t','t')`)
	if err == nil {
		t.Fatal("mode=ro/query_only must reject INSERT")
	}

	after := fileFingerprint(t, abs)
	if before != after {
		t.Fatalf("DB mutated under read-only: before=%s after=%s", before, after)
	}

	// Missing DB still must not be created (Exists / OpenReadOnly contract).
	missingHome := filepath.Join(t.TempDir(), "empty home")
	if err := os.MkdirAll(missingHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReadOnly(missingHome, testProjectID); err == nil {
		t.Fatal("missing db must fail")
	}
	ok, err := Exists(missingHome, testProjectID)
	if err != nil || ok {
		t.Fatalf("exists=%v err=%v", ok, err)
	}
	rel, _ := MemoryDirRel(testProjectID)
	if _, err := os.Stat(filepath.Join(missingHome, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("must not create memory dir: %v", err)
	}
}

func TestJoinRollbackError_Observable(t *testing.T) {
	opErr := errors.New("operation failed")
	rbErr := errors.New("rollback boom")

	joined := joinRollbackError(opErr, rbErr)
	if joined == nil {
		t.Fatal("expected joined error")
	}
	if !errors.Is(joined, opErr) {
		t.Fatalf("missing original: %v", joined)
	}
	if !errors.Is(joined, rbErr) {
		t.Fatalf("missing rollback: %v", joined)
	}
	if !strings.Contains(joined.Error(), "memory: rollback:") {
		t.Fatalf("rollback prefix missing: %v", joined)
	}

	if got := joinRollbackError(opErr, nil); !errors.Is(got, opErr) || got != opErr {
		t.Fatalf("nil rollback should preserve op err: %v", got)
	}
	if got := joinRollbackError(opErr, sql.ErrTxDone); !errors.Is(got, opErr) || got != opErr {
		t.Fatalf("ErrTxDone must not be reported: %v", got)
	}
	if got := joinRollbackError(nil, rbErr); !errors.Is(got, rbErr) {
		t.Fatalf("rollback-only: %v", got)
	}
	if got := joinRollbackError(nil, sql.ErrTxDone); got != nil {
		t.Fatalf("ErrTxDone alone must be nil: %v", got)
	}
}

func mustDBAbs(t *testing.T, home, projectID string) string {
	t.Helper()
	abs, _, err := resolveDBPath(home, projectID)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
