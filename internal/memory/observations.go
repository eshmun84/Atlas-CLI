package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SaveObservation inserts, exact-dedupes, or topic-updates an observation.
// Observation + FTS mutations are atomic in one transaction.
func (s *Store) SaveObservation(in Observation) (res SaveResult, err error) {
	if err := s.requireWrite(); err != nil {
		return SaveResult{}, err
	}
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	if in.ProjectID == "" {
		in.ProjectID = s.projectID
	}
	if in.ProjectID != s.projectID {
		return SaveResult{}, fmt.Errorf("memory: observation project id mismatch")
	}
	if in.Scope == "" {
		in.Scope = ScopeProject
	}
	if err := ValidateObservation(in); err != nil {
		return SaveResult{}, err
	}
	topic, err := NormalizeTopicKey(in.TopicKey)
	if err != nil {
		return SaveResult{}, err
	}
	in.TopicKey = topic
	in.Title = strings.TrimSpace(in.Title)
	in.Content = strings.TrimSpace(in.Content)
	in.SourceKind = strings.TrimSpace(in.SourceKind)
	in.SourceRef = strings.TrimSpace(in.SourceRef)
	in.SessionID = strings.TrimSpace(in.SessionID)

	hash := durableHash(in.ProjectID, in.Scope, in.Type, in.Title, in.Content, in.TopicKey, in.SourceKind, in.SourceRef)
	now := s.now()

	tx, err := s.db.Begin()
	if err != nil {
		return SaveResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			err = rollbackTx(tx, err)
		}
	}()

	if in.SessionID != "" {
		var sess Session
		sess, err = getSessionTx(tx, in.SessionID)
		if err != nil {
			return SaveResult{}, err
		}
		if sess.ProjectID != s.projectID {
			err = fmt.Errorf("memory: session belongs to another project")
			return SaveResult{}, err
		}
	}

	// Exact duplicate by durable hash.
	if existing, ok, getErr := getByHashTx(tx, s.projectID, hash); getErr != nil {
		err = getErr
		return SaveResult{}, err
	} else if ok {
		var updated Observation
		updated, err = bumpDuplicateTx(tx, existing, now)
		if err != nil {
			return SaveResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return SaveResult{}, err
		}
		committed = true
		return SaveResult{Action: SaveDuplicate, Observation: updated}, nil
	}

	// Topic upsert when topic_key is set.
	if topic != "" {
		if existing, ok, getErr := getByTopicTx(tx, s.projectID, in.Scope, topic); getErr != nil {
			err = getErr
			return SaveResult{}, err
		} else if ok {
			if existing.ContentHash == hash {
				var updated Observation
				updated, err = bumpDuplicateTx(tx, existing, now)
				if err != nil {
					return SaveResult{}, err
				}
				if err = tx.Commit(); err != nil {
					return SaveResult{}, err
				}
				committed = true
				return SaveResult{Action: SaveDuplicate, Observation: updated}, nil
			}
			var updated Observation
			updated, err = reviseTopicTx(tx, existing, in, hash, now)
			if err != nil {
				return SaveResult{}, err
			}
			if err = syncFTSTx(tx, updated); err != nil {
				return SaveResult{}, err
			}
			if err = tx.Commit(); err != nil {
				return SaveResult{}, err
			}
			committed = true
			return SaveResult{Action: SaveUpdated, Observation: updated}, nil
		}
	}

	var created Observation
	created, err = insertObservationTx(tx, in, hash, now)
	if err != nil {
		return SaveResult{}, err
	}
	if err = syncFTSTx(tx, created); err != nil {
		return SaveResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return SaveResult{}, err
	}
	committed = true
	return SaveResult{Action: SaveCreated, Observation: created}, nil
}

// GetObservation loads an observation by ID for this project.
func (s *Store) GetObservation(id string) (Observation, error) {
	if s == nil || s.db == nil {
		return Observation{}, fmt.Errorf("memory: store is closed")
	}
	obs, err := getByIDTx(s.db, strings.TrimSpace(id))
	if err != nil {
		return Observation{}, err
	}
	if obs.ProjectID != s.projectID {
		return Observation{}, fmt.Errorf("memory: observation not found")
	}
	return obs, nil
}

// GetByTopic loads the observation for project+scope+topic_key.
func (s *Store) GetByTopic(scope Scope, topicKey string) (Observation, error) {
	if s == nil || s.db == nil {
		return Observation{}, fmt.Errorf("memory: store is closed")
	}
	if scope == "" {
		scope = ScopeProject
	}
	topic, err := NormalizeTopicKey(topicKey)
	if err != nil {
		return Observation{}, err
	}
	if topic == "" {
		return Observation{}, fmt.Errorf("memory: topic key is required")
	}
	obs, ok, err := getByTopicTx(s.db, s.projectID, scope, topic)
	if err != nil {
		return Observation{}, err
	}
	if !ok {
		return Observation{}, fmt.Errorf("memory: topic %q not found", topic)
	}
	return obs, nil
}

// ListObservations returns observations matching filter (store project enforced).
func (s *Store) ListObservations(filter ListFilter) ([]Observation, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("memory: store is closed")
	}
	filter.ProjectID = s.projectID
	limit := clampLimit(filter.Limit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	q := `SELECT id, project_id, session_id, type, scope, title, content, topic_key,
		source_kind, source_ref, content_hash, duplicate_count, revision_count,
		first_seen_at, last_seen_at, created_at, updated_at
		FROM memory_observations WHERE project_id = ?`
	args := []any{s.projectID}
	if filter.Scope != "" {
		q += ` AND scope = ?`
		args = append(args, string(filter.Scope))
	}
	if filter.Type != "" {
		q += ` AND type = ?`
		args = append(args, string(filter.Type))
	}
	if tk := strings.TrimSpace(filter.TopicKey); tk != "" {
		topic, err := NormalizeTopicKey(tk)
		if err != nil {
			return nil, err
		}
		q += ` AND topic_key = ?`
		args = append(args, topic)
	}
	q += ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: list: %w", err)
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
	return out, rows.Err()
}

func insertObservationTx(tx *sql.Tx, in Observation, hash string, now time.Time) (Observation, error) {
	id, err := newObservationID()
	if err != nil {
		return Observation{}, err
	}
	obs := Observation{
		ID:             id,
		ProjectID:      in.ProjectID,
		SessionID:      in.SessionID,
		Type:           in.Type,
		Scope:          in.Scope,
		Title:          in.Title,
		Content:        in.Content,
		TopicKey:       in.TopicKey,
		SourceKind:     in.SourceKind,
		SourceRef:      in.SourceRef,
		ContentHash:    hash,
		DuplicateCount: 1,
		RevisionCount:  0,
		FirstSeenAt:    now,
		LastSeenAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = tx.Exec(
		`INSERT INTO memory_observations(
			id, project_id, session_id, type, scope, title, content, topic_key,
			source_kind, source_ref, content_hash, duplicate_count, revision_count,
			first_seen_at, last_seen_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		obs.ID, obs.ProjectID, nullIfEmpty(obs.SessionID), string(obs.Type), string(obs.Scope),
		obs.Title, obs.Content, nullIfEmpty(obs.TopicKey),
		nullIfEmpty(obs.SourceKind), nullIfEmpty(obs.SourceRef), obs.ContentHash,
		obs.DuplicateCount, obs.RevisionCount,
		obs.FirstSeenAt.Format(time.RFC3339Nano), obs.LastSeenAt.Format(time.RFC3339Nano),
		obs.CreatedAt.Format(time.RFC3339Nano), obs.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Observation{}, fmt.Errorf("memory: insert observation: %w", err)
	}
	return obs, nil
}

func bumpDuplicateTx(tx *sql.Tx, existing Observation, now time.Time) (Observation, error) {
	_, err := tx.Exec(
		`UPDATE memory_observations SET duplicate_count = duplicate_count + 1, last_seen_at = ?, updated_at = ?
		 WHERE id = ?`,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), existing.ID,
	)
	if err != nil {
		return Observation{}, fmt.Errorf("memory: bump duplicate: %w", err)
	}
	existing.DuplicateCount++
	existing.LastSeenAt = now
	existing.UpdatedAt = now
	return existing, nil
}

func reviseTopicTx(tx *sql.Tx, existing Observation, in Observation, hash string, now time.Time) (Observation, error) {
	_, err := tx.Exec(
		`UPDATE memory_observations SET
			session_id = ?, type = ?, title = ?, content = ?,
			source_kind = ?, source_ref = ?, content_hash = ?,
			revision_count = revision_count + 1,
			last_seen_at = ?, updated_at = ?
		 WHERE id = ?`,
		nullIfEmpty(in.SessionID), string(in.Type), in.Title, in.Content,
		nullIfEmpty(in.SourceKind), nullIfEmpty(in.SourceRef), hash,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), existing.ID,
	)
	if err != nil {
		return Observation{}, fmt.Errorf("memory: topic revise: %w", err)
	}
	existing.SessionID = in.SessionID
	existing.Type = in.Type
	existing.Title = in.Title
	existing.Content = in.Content
	existing.SourceKind = in.SourceKind
	existing.SourceRef = in.SourceRef
	existing.ContentHash = hash
	existing.RevisionCount++
	existing.LastSeenAt = now
	existing.UpdatedAt = now
	return existing, nil
}

func syncFTSTx(tx *sql.Tx, obs Observation) error {
	if _, err := tx.Exec(`DELETE FROM memory_observations_fts WHERE id = ?`, obs.ID); err != nil {
		return fmt.Errorf("memory: fts delete: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO memory_observations_fts(id, title, content, topic_key) VALUES (?, ?, ?, ?)`,
		obs.ID, obs.Title, obs.Content, obs.TopicKey,
	); err != nil {
		return fmt.Errorf("memory: fts insert: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func getByIDTx(q queryRower, id string) (Observation, error) {
	row := q.QueryRow(
		`SELECT id, project_id, session_id, type, scope, title, content, topic_key,
			source_kind, source_ref, content_hash, duplicate_count, revision_count,
			first_seen_at, last_seen_at, created_at, updated_at
		 FROM memory_observations WHERE id = ?`, id)
	obs, err := scanObservation(row)
	if err == sql.ErrNoRows {
		return Observation{}, fmt.Errorf("memory: observation %q not found", id)
	}
	if err != nil {
		return Observation{}, err
	}
	return obs, nil
}

func getByHashTx(q queryRower, projectID, hash string) (Observation, bool, error) {
	row := q.QueryRow(
		`SELECT id, project_id, session_id, type, scope, title, content, topic_key,
			source_kind, source_ref, content_hash, duplicate_count, revision_count,
			first_seen_at, last_seen_at, created_at, updated_at
		 FROM memory_observations WHERE project_id = ? AND content_hash = ?`,
		projectID, hash)
	obs, err := scanObservation(row)
	if err == sql.ErrNoRows {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	return obs, true, nil
}

func getByTopicTx(q queryRower, projectID string, scope Scope, topic string) (Observation, bool, error) {
	row := q.QueryRow(
		`SELECT id, project_id, session_id, type, scope, title, content, topic_key,
			source_kind, source_ref, content_hash, duplicate_count, revision_count,
			first_seen_at, last_seen_at, created_at, updated_at
		 FROM memory_observations WHERE project_id = ? AND scope = ? AND topic_key = ?`,
		projectID, string(scope), topic)
	obs, err := scanObservation(row)
	if err == sql.ErrNoRows {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	return obs, true, nil
}

func scanObservation(row scanner) (Observation, error) {
	var (
		obs                           Observation
		sessionID, topic, sk, sr      sql.NullString
		typ, scope                    string
		first, last, created, updated string
		dup, rev                      int
	)
	err := row.Scan(
		&obs.ID, &obs.ProjectID, &sessionID, &typ, &scope, &obs.Title, &obs.Content, &topic,
		&sk, &sr, &obs.ContentHash, &dup, &rev,
		&first, &last, &created, &updated,
	)
	if err != nil {
		return Observation{}, err
	}
	obs.Type = ObservationType(typ)
	obs.Scope = Scope(scope)
	if sessionID.Valid {
		obs.SessionID = sessionID.String
	}
	if topic.Valid {
		obs.TopicKey = topic.String
	}
	if sk.Valid {
		obs.SourceKind = sk.String
	}
	if sr.Valid {
		obs.SourceRef = sr.String
	}
	obs.DuplicateCount = dup
	obs.RevisionCount = rev
	var err2 error
	if obs.FirstSeenAt, err2 = time.Parse(time.RFC3339Nano, first); err2 != nil {
		return Observation{}, fmt.Errorf("memory: parse first_seen_at: %w", err2)
	}
	if obs.LastSeenAt, err2 = time.Parse(time.RFC3339Nano, last); err2 != nil {
		return Observation{}, fmt.Errorf("memory: parse last_seen_at: %w", err2)
	}
	if obs.CreatedAt, err2 = time.Parse(time.RFC3339Nano, created); err2 != nil {
		return Observation{}, fmt.Errorf("memory: parse created_at: %w", err2)
	}
	if obs.UpdatedAt, err2 = time.Parse(time.RFC3339Nano, updated); err2 != nil {
		return Observation{}, fmt.Errorf("memory: parse updated_at: %w", err2)
	}
	return obs, nil
}
