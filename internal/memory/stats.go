package memory

import (
	"database/sql"
	"errors"
	"fmt"
)

// QualityStats are bounded aggregate counters for read-only diagnostics.
// All queries are project-scoped and never load observation bodies.
type QualityStats struct {
	ObservationCount      int
	ProjectScopeCount     int
	DecisionCount         int
	ConstraintCount       int
	PatternCount          int
	DuplicateBumpCount    int // observations with duplicate_count > 1 (intentional bumps)
	DuplicateAnomalyCount int // distinct content_hash with >1 row (unique-index violation)
	MalformedCount        int // empty title/content
	OversizedCount        int // title/content beyond validation caps
	InvalidScopeCount     int
	InvalidTypeCount      int
	FTSPresent            bool
}

// QualityStats returns aggregate observation quality metrics for this project.
// Read-only SELECT aggregates only — no row body materialization.
func (s *Store) QualityStats() (QualityStats, error) {
	var out QualityStats
	if s == nil || s.db == nil {
		return out, fmt.Errorf("memory: store is closed")
	}
	pid := s.projectID

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations WHERE project_id = ?`, pid,
	).Scan(&out.ObservationCount); err != nil {
		return out, fmt.Errorf("memory: count observations: %w", err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations WHERE project_id = ? AND scope = ?`,
		pid, string(ScopeProject),
	).Scan(&out.ProjectScopeCount); err != nil {
		return out, fmt.Errorf("memory: count project scope: %w", err)
	}

	typeCounts := []struct {
		typ string
		dst *int
	}{
		{string(TypeDecision), &out.DecisionCount},
		{string(TypeConstraint), &out.ConstraintCount},
		{string(TypePattern), &out.PatternCount},
	}
	for _, tc := range typeCounts {
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM memory_observations WHERE project_id = ? AND type = ?`,
			pid, tc.typ,
		).Scan(tc.dst); err != nil {
			return out, fmt.Errorf("memory: count type %s: %w", tc.typ, err)
		}
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations WHERE project_id = ? AND duplicate_count > 1`,
		pid,
	).Scan(&out.DuplicateBumpCount); err != nil {
		return out, fmt.Errorf("memory: count duplicate bumps: %w", err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM (
			SELECT content_hash FROM memory_observations
			WHERE project_id = ?
			GROUP BY content_hash
			HAVING COUNT(*) > 1
		)`,
		pid,
	).Scan(&out.DuplicateAnomalyCount); err != nil {
		return out, fmt.Errorf("memory: count duplicate anomalies: %w", err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations
		 WHERE project_id = ?
		   AND (TRIM(title) = '' OR TRIM(content) = '')`,
		pid,
	).Scan(&out.MalformedCount); err != nil {
		return out, fmt.Errorf("memory: count malformed: %w", err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations
		 WHERE project_id = ?
		   AND (LENGTH(title) > ? OR LENGTH(content) > ?)`,
		pid, maxTitleLen, maxContentLen,
	).Scan(&out.OversizedCount); err != nil {
		return out, fmt.Errorf("memory: count oversized: %w", err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations
		 WHERE project_id = ? AND scope != ?`,
		pid, string(ScopeProject),
	).Scan(&out.InvalidScopeCount); err != nil {
		return out, fmt.Errorf("memory: count invalid scope: %w", err)
	}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memory_observations
		 WHERE project_id = ?
		   AND type NOT IN (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pid,
		string(TypeDecision), string(TypeArchitecture), string(TypeConstraint),
		string(TypeDiscovery), string(TypeBugfix), string(TypePattern),
		string(TypeConfig), string(TypePreference), string(TypeFact),
	).Scan(&out.InvalidTypeCount); err != nil {
		return out, fmt.Errorf("memory: count invalid type: %w", err)
	}

	var ftsName string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'memory_observations_fts'`,
	).Scan(&ftsName)
	switch {
	case err == nil && ftsName == "memory_observations_fts":
		out.FTSPresent = true
	case errors.Is(err, sql.ErrNoRows):
		out.FTSPresent = false
	case err != nil:
		return out, fmt.Errorf("memory: inspect fts: %w", err)
	}

	return out, nil
}

// ProbeRetrieval runs a bounded FTS probe that must not mutate state.
// Empty results are success; only structural/query failures are errors.
func (s *Store) ProbeRetrieval() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("memory: store is closed")
	}
	_, err := s.Search(SearchQuery{Query: `"atlas_diag_probe"`, Limit: 1})
	return err
}
