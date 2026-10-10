package memory

import (
	"fmt"
	"strings"
)

// Search runs an FTS5 query over title/content/topic_key for this project.
// Invalid MATCH syntax returns a clean error (no panic, no SQL interpolation).
func (s *Store) Search(q SearchQuery) ([]Observation, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("memory: store is closed")
	}
	query := strings.TrimSpace(q.Query)
	if query == "" {
		return nil, fmt.Errorf("memory: search query is required")
	}
	limit := clampLimit(q.Limit)

	// Parameterized MATCH — never concatenate user input into SQL text.
	const sqlText = `
SELECT o.id, o.project_id, o.session_id, o.type, o.scope, o.title, o.content, o.topic_key,
	o.source_kind, o.source_ref, o.content_hash, o.duplicate_count, o.revision_count,
	o.first_seen_at, o.last_seen_at, o.created_at, o.updated_at
FROM memory_observations_fts
JOIN memory_observations o ON o.id = memory_observations_fts.id
WHERE o.project_id = ? AND memory_observations_fts MATCH ?
ORDER BY rank
LIMIT ?`

	rows, err := s.db.Query(sqlText, s.projectID, query, limit)
	if err != nil {
		if isFTSSyntaxError(err) {
			return nil, fmt.Errorf("memory: invalid search query")
		}
		return nil, fmt.Errorf("memory: search: %w", err)
	}
	defer rows.Close()

	var out []Observation
	for rows.Next() {
		obs, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, obs)
	}
	if err := rows.Err(); err != nil {
		if isFTSSyntaxError(err) {
			return nil, fmt.Errorf("memory: invalid search query")
		}
		return nil, err
	}
	return out, nil
}

func isFTSSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "fts5") &&
		(strings.Contains(msg, "syntax") || strings.Contains(msg, "malformed") || strings.Contains(msg, "error"))
}

// SchemaVersion returns the applied schema version.
func (s *Store) SchemaVersion() (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("memory: store is closed")
	}
	return currentDBVersion(s.db)
}

// ForeignKeysEnabled reports PRAGMA foreign_keys.
func (s *Store) ForeignKeysEnabled() (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("memory: store is closed")
	}
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return false, err
	}
	return fk == 1, nil
}
