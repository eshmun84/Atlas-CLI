package memory

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"

	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMS = 5000
	defaultLimit  = 50
	maxLimit      = 500
)

// Options configures Store open behavior.
type Options struct {
	Clock Clock
	// ReadOnly opens an existing DB without creating directories, creating the
	// file, or applying migrations. Missing DB → error.
	ReadOnly bool
}

// Store is the SQLite-backed Memory Core.
type Store struct {
	db        *sql.DB
	homePath  string
	projectID string
	clock     Clock
	readOnly  bool
}

// Open opens or creates the project Memory database under Atlas Home and
// applies migrations. Mutating: may create memory/ and memory.db.
func Open(homePath, projectID string) (*Store, error) {
	return OpenWithOptions(homePath, projectID, Options{})
}

// OpenWithOptions is Open with explicit options (clock, read-only).
func OpenWithOptions(homePath, projectID string, opts Options) (*Store, error) {
	return openStore(homePath, projectID, opts)
}

// OpenReadOnly opens an existing Memory database without creating paths or
// applying migrations. Does not mutate the filesystem layout.
func OpenReadOnly(homePath, projectID string) (*Store, error) {
	return openStore(homePath, projectID, Options{ReadOnly: true})
}

func openStore(homePath, projectID string, opts Options) (*Store, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if err := ValidateProjectID(projectID); err != nil {
		return nil, err
	}
	abs, rel, err := resolveDBPath(homePath, projectID)
	if err != nil {
		return nil, err
	}

	clock := opts.Clock
	if clock == nil {
		clock = defaultClock
	}

	if opts.ReadOnly {
		ok, err := Exists(homePath, projectID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("memory: database not found")
		}
		// Explicit SQLite URI read-only open — no CREATE, no migrate, no chmod.
		db, err := sql.Open("sqlite", sqliteReadOnlyDSN(abs))
		if err != nil {
			return nil, fmt.Errorf("memory: open read-only: %w", err)
		}
		s := &Store{db: db, homePath: homePath, projectID: projectID, clock: clock, readOnly: true}
		if err := s.configure(); err != nil {
			_ = db.Close()
			return nil, err
		}
		if err := checkSchemaReadable(db); err != nil {
			_ = db.Close()
			return nil, err
		}
		return s, nil
	}

	if _, err := ensureMemoryDir(homePath, projectID); err != nil {
		return nil, err
	}
	// Re-validate after mkdir (symlink race fail-closed).
	if _, err := fsafety.ContainedJoin(homePath, rel); err != nil {
		return nil, fmt.Errorf("memory: database path unsafe after mkdir: %w", err)
	}
	info, lerr := fsafety.LstatContained(homePath, rel)
	if lerr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("memory: refusing symlink database")
	}

	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, fmt.Errorf("memory: open: %w", err)
	}
	s := &Store{db: db, homePath: homePath, projectID: projectID, clock: clock}
	if err := s.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := applyMigrations(db, clock()); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Tighten DB file permissions to match Atlas Home secret-style files (0o600).
	if err := os.Chmod(abs, 0o600); err != nil && !os.IsNotExist(err) {
		_ = db.Close()
		return nil, fmt.Errorf("memory: chmod database: %w", err)
	}
	return s, nil
}

// sqliteReadOnlyDSN builds a SQLite file: URI that opens the DB in mode=ro.
// Plain "path?mode=ro" is NOT a URI: modernc.org/sqlite only forwards mode=ro
// (and other SQLite URI params) when the DSN starts with "file:". Paths are
// escaped via net/url so spaces, '#', '?' and similar cannot corrupt the query.
func sqliteReadOnlyDSN(abs string) string {
	u := url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(abs),
		RawQuery: "mode=ro",
	}
	return u.String()
}

func (s *Store) configure() error {
	// Single connection avoids SQLITE_BUSY from database/sql pooling against
	// default rollback-journal SQLite (WAL is intentionally not auto-enabled).
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	if _, err := s.db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMS)); err != nil {
		return fmt.Errorf("memory: busy_timeout: %w", err)
	}
	if s.readOnly {
		// Defense in depth beyond mode=ro (connection-local; not a schema mutation).
		if _, err := s.db.Exec(`PRAGMA query_only = ON`); err != nil {
			return fmt.Errorf("memory: query_only: %w", err)
		}
	}
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("memory: foreign_keys: %w", err)
	}
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return fmt.Errorf("memory: verify foreign_keys: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("memory: foreign_keys not enabled")
	}
	return nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// ProjectID returns the store's project identity.
func (s *Store) ProjectID() string { return s.projectID }

func (s *Store) now() time.Time {
	if s.clock == nil {
		return defaultClock()
	}
	return s.clock().UTC()
}

func (s *Store) requireWrite() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("memory: store is closed")
	}
	if s.readOnly {
		return fmt.Errorf("memory: store is read-only")
	}
	return nil
}

func clampLimit(n int) int {
	if n <= 0 {
		return defaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}
