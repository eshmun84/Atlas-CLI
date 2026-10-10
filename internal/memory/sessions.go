package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// BeginSession starts a new Memory session for the store's project.
func (s *Store) BeginSession() (Session, error) {
	if err := s.requireWrite(); err != nil {
		return Session{}, err
	}
	id, err := newSessionID()
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	sess := Session{
		ID:        id,
		ProjectID: s.projectID,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = s.db.Exec(
		`INSERT INTO memory_sessions(id, project_id, started_at, ended_at, summary, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, NULL, ?, ?)`,
		sess.ID, sess.ProjectID,
		sess.StartedAt.Format(time.RFC3339Nano),
		sess.CreatedAt.Format(time.RFC3339Nano),
		sess.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Session{}, fmt.Errorf("memory: begin session: %w", err)
	}
	return sess, nil
}

// EndSession closes a session. Optional summary is persisted as-is (not generated).
// Fails if the session is missing or belongs to another project.
// Ending an already-closed session updates summary when provided and is otherwise a no-op success.
func (s *Store) EndSession(sessionID string, summary string) (sess Session, err error) {
	if err := s.requireWrite(); err != nil {
		return Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Session{}, fmt.Errorf("memory: session id is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Session{}, err
	}
	committed := false
	defer func() {
		if !committed {
			err = rollbackTx(tx, err)
		}
	}()

	sess, err = getSessionTx(tx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if sess.ProjectID != s.projectID {
		err = fmt.Errorf("memory: session %q belongs to another project", sessionID)
		return Session{}, err
	}

	now := s.now()
	sum := strings.TrimSpace(summary)
	if sess.EndedAt != nil {
		// Already closed: allow optional summary refresh only.
		if sum != "" && sum != sess.Summary {
			if _, err = tx.Exec(
				`UPDATE memory_sessions SET summary = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
				sum, now.Format(time.RFC3339Nano), sessionID, s.projectID,
			); err != nil {
				err = fmt.Errorf("memory: update closed session summary: %w", err)
				return Session{}, err
			}
			sess.Summary = sum
			sess.UpdatedAt = now
		}
		if err = tx.Commit(); err != nil {
			return Session{}, err
		}
		committed = true
		return sess, nil
	}

	ended := now
	if _, err = tx.Exec(
		`UPDATE memory_sessions SET ended_at = ?, summary = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
		ended.Format(time.RFC3339Nano), nullIfEmpty(sum), now.Format(time.RFC3339Nano), sessionID, s.projectID,
	); err != nil {
		err = fmt.Errorf("memory: end session: %w", err)
		return Session{}, err
	}
	if err = tx.Commit(); err != nil {
		return Session{}, err
	}
	committed = true
	sess.EndedAt = &ended
	sess.Summary = sum
	sess.UpdatedAt = now
	return sess, nil
}

// GetSession loads a session by ID for this project.
func (s *Store) GetSession(sessionID string) (Session, error) {
	if s == nil || s.db == nil {
		return Session{}, fmt.Errorf("memory: store is closed")
	}
	sess, err := getSessionTx(s.db, sessionID)
	if err != nil {
		return Session{}, err
	}
	if sess.ProjectID != s.projectID {
		return Session{}, fmt.Errorf("memory: session %q belongs to another project", sessionID)
	}
	return sess, nil
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func getSessionTx(q queryRower, sessionID string) (Session, error) {
	var (
		sess    Session
		started string
		ended   sql.NullString
		summary sql.NullString
		created string
		updated string
	)
	err := q.QueryRow(
		`SELECT id, project_id, started_at, ended_at, summary, created_at, updated_at
		 FROM memory_sessions WHERE id = ?`,
		strings.TrimSpace(sessionID),
	).Scan(&sess.ID, &sess.ProjectID, &started, &ended, &summary, &created, &updated)
	if err == sql.ErrNoRows {
		return Session{}, fmt.Errorf("memory: session %q not found", sessionID)
	}
	if err != nil {
		return Session{}, fmt.Errorf("memory: get session: %w", err)
	}
	var err2 error
	if sess.StartedAt, err2 = time.Parse(time.RFC3339Nano, started); err2 != nil {
		return Session{}, fmt.Errorf("memory: parse started_at: %w", err2)
	}
	if ended.Valid && ended.String != "" {
		t, err := time.Parse(time.RFC3339Nano, ended.String)
		if err != nil {
			return Session{}, fmt.Errorf("memory: parse ended_at: %w", err)
		}
		sess.EndedAt = &t
	}
	if summary.Valid {
		sess.Summary = summary.String
	}
	if sess.CreatedAt, err2 = time.Parse(time.RFC3339Nano, created); err2 != nil {
		return Session{}, fmt.Errorf("memory: parse created_at: %w", err2)
	}
	if sess.UpdatedAt, err2 = time.Parse(time.RFC3339Nano, updated); err2 != nil {
		return Session{}, fmt.Errorf("memory: parse updated_at: %w", err2)
	}
	return sess, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
