package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{version: 1, name: "initial", sql: migrationV1},
}

// ensureMigrationsTable creates schema_migrations. Mutation path only —
// never call from OpenReadOnly / checkSchemaReadable.
func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY NOT NULL,
  applied_at TEXT NOT NULL
);`)
	return err
}

func currentDBVersion(db *sql.DB) (int, error) {
	var ver sql.NullInt64
	err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&ver)
	if err != nil {
		return 0, err
	}
	if !ver.Valid {
		return 0, nil
	}
	return int(ver.Int64), nil
}

func applyMigrations(db *sql.DB, now time.Time) error {
	if err := ensureMigrationsTable(db); err != nil {
		return fmt.Errorf("memory: ensure migrations table: %w", err)
	}
	ver, err := currentDBVersion(db)
	if err != nil {
		return fmt.Errorf("memory: read schema version: %w", err)
	}
	if ver > CurrentSchemaVersion {
		return fmt.Errorf("memory: unsupported newer schema version %d (max %d)", ver, CurrentSchemaVersion)
	}
	for _, m := range migrations {
		if m.version <= ver {
			continue
		}
		if m.version != ver+1 {
			return fmt.Errorf("memory: non-monotonic migration gap: have %d, next %d", ver, m.version)
		}
		if err := applyOneMigration(db, m, now); err != nil {
			return err
		}
		ver = m.version
	}
	return nil
}

func applyOneMigration(db *sql.DB, m migration, now time.Time) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin migration %d: %w", m.version, err)
	}
	committed := false
	defer func() {
		if !committed {
			err = rollbackTx(tx, err)
		}
	}()

	for _, stmt := range splitSQL(m.sql) {
		if _, err = tx.Exec(stmt); err != nil {
			err = fmt.Errorf("memory: migration %d (%s): %w", m.version, m.name, err)
			return err
		}
	}
	if _, err = tx.Exec(
		`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
		m.version, now.UTC().Format(time.RFC3339Nano),
	); err != nil {
		err = fmt.Errorf("memory: record migration %d: %w", m.version, err)
		return err
	}
	if err = tx.Commit(); err != nil {
		err = fmt.Errorf("memory: commit migration %d: %w", m.version, err)
		return err
	}
	committed = true
	return nil
}

func splitSQL(script string) []string {
	var out []string
	for _, part := range strings.Split(script, ";") {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}

// checkSchemaReadable is SELECT-only introspection for OpenReadOnly.
// It never CREATE/INSERT/UPDATE/DELETE and never calls ensureMigrationsTable.
func checkSchemaReadable(db *sql.DB) error {
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return fmt.Errorf("memory: schema_migrations missing")
	}
	if err != nil {
		return fmt.Errorf("memory: inspect schema: %w", err)
	}

	ver, err := currentDBVersion(db)
	if err != nil {
		return fmt.Errorf("memory: read schema version: %w", err)
	}
	if ver == 0 {
		return fmt.Errorf("memory: database has no applied schema")
	}
	if ver > CurrentSchemaVersion {
		return fmt.Errorf("memory: unsupported newer schema version %d (max %d)", ver, CurrentSchemaVersion)
	}
	if ver < CurrentSchemaVersion {
		return fmt.Errorf("memory: schema version %d requires migration (open read-write)", ver)
	}
	return nil
}
