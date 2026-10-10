package memory

import (
	"database/sql"
	"time"
)

// DBForTest exposes the underlying *sql.DB for white-box tests.
func (s *Store) DBForTest() *sql.DB { return s.db }

// CurrentDBVersionForTest returns schema version.
func CurrentDBVersionForTest(db *sql.DB) (int, error) { return currentDBVersion(db) }

// EnsureMigrationsTableForTest creates the migrations table.
func EnsureMigrationsTableForTest(db *sql.DB) error { return ensureMigrationsTable(db) }

// ApplyOneMigrationForTest applies a single migration inside a transaction.
func ApplyOneMigrationForTest(db *sql.DB, version int, name, sqlText string, now time.Time) error {
	return applyOneMigration(db, migration{version: version, name: name, sql: sqlText}, now)
}
