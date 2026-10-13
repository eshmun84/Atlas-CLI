package memoryhealth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Known diagnostic check IDs (Slice 36D).
const (
	CheckConfigEnabled         = "memory.config.enabled"
	CheckConfigPersistenceMode = "memory.config.persistence_mode"
	CheckStorageExists         = "memory.storage.exists"
	CheckStorageRegular        = "memory.storage.regular"
	CheckStorageSymlinkSafe    = "memory.storage.symlink_safe"
	CheckStorageReadable       = "memory.storage.readable"
	CheckStorageWritable       = "memory.storage.writable_if_required"
	CheckSchemaCompatible      = "memory.schema.compatible"
	CheckSchemaMigrations      = "memory.schema.migrations_readable"
	CheckRetrievalReadonlyOpen = "memory.retrieval.readonly_open"
	CheckRetrievalBasicQuery   = "memory.retrieval.basic_query"
	CheckPersistenceAvailable  = "memory.persistence.available"
	CheckDataObservationCount  = "memory.data.observation_count"
	CheckDataRequiredFields    = "memory.data.required_fields"
	CheckDataScopeValidity     = "memory.data.scope_validity"
	CheckDataDuplicateAnomaly  = "memory.data.duplicate_anomaly"
)

// Inspect gathers read-only Memory diagnostics. It never creates DB files,
// directories, migrations, or observation writes.
func Inspect(in Input) Diagnostics {
	d := Diagnostics{
		Status:  StatusUnavailable,
		Checks:  nil,
		Limits:  defaultLimits(),
		Enabled: in.MemoryEnabled,
	}

	mode := strings.TrimSpace(in.PersistenceMode)
	if mode == "" {
		mode = config.MemoryPersistenceGoverned
	}
	d.PersistenceMode = mode
	d.ProjectID = strings.TrimSpace(in.ProjectID)

	if !in.Initialized {
		d.Status = StatusUnavailable
		d.StorageLabel = "n/a"
		d.SchemaLabel = "n/a"
		d.RetrievalLabel = "n/a"
		d.PersistenceLabel = "n/a"
		d.QualityLabel = "n/a"
		d.add(Check{
			ID:       CheckConfigEnabled,
			Severity: SeverityInfo,
			Status:   "unavailable",
			Message:  "Memory inactive (project not initialized)",
		})
		return d
	}

	if !in.ConfigLoads {
		d.Status = StatusFailed
		d.StorageLabel = "unknown"
		d.SchemaLabel = "unknown"
		d.RetrievalLabel = "unknown"
		d.PersistenceLabel = "unknown"
		d.QualityLabel = "unknown"
		d.add(Check{
			ID:              CheckConfigEnabled,
			Severity:        SeverityFail,
			Status:          "failed",
			Message:         "Atlas config failed to load; Memory diagnostics unavailable",
			RemediationHint: "Fix .atlas/config.yaml then re-run atlas doctor",
		})
		return d
	}

	// --- Configuration ---
	if !in.MemoryEnabled {
		d.Enabled = false
		d.Status = StatusUnavailable
		d.StorageLabel = "n/a"
		d.SchemaLabel = "n/a"
		d.RetrievalLabel = "n/a"
		d.PersistenceLabel = "off"
		d.QualityLabel = "n/a"
		d.add(Check{
			ID:       CheckConfigEnabled,
			Severity: SeverityInfo,
			Status:   "unavailable",
			Message:  "Memory is disabled in configuration",
		})
		d.add(Check{
			ID:       CheckConfigPersistenceMode,
			Severity: SeverityInfo,
			Status:   "off",
			Message:  "Persistence mode forced off while Memory is disabled",
		})
		return d
	}

	d.add(Check{
		ID:       CheckConfigEnabled,
		Severity: SeverityPass,
		Status:   "enabled",
		Message:  "Memory is enabled",
	})

	switch mode {
	case config.MemoryPersistenceGoverned:
		d.add(Check{
			ID:       CheckConfigPersistenceMode,
			Severity: SeverityPass,
			Status:   "governed",
			Message:  "Persistence mode is governed",
		})
		d.PersistenceLabel = "governed"
	case config.MemoryPersistenceOff:
		d.add(Check{
			ID:       CheckConfigPersistenceMode,
			Severity: SeverityInfo,
			Status:   "off",
			Message:  "Persistence mode is off (retrieval-only)",
		})
		d.PersistenceLabel = "off"
	default:
		d.add(Check{
			ID:              CheckConfigPersistenceMode,
			Severity:        SeverityFail,
			Status:          "invalid",
			Message:         "Persistence mode is invalid",
			RemediationHint: "Set memory.persistence.mode to governed or off",
			Metadata:        checkMeta("mode", mode),
		})
		d.PersistenceLabel = "invalid"
		d.Status = StatusFailed
		d.StorageLabel = "unchecked"
		d.SchemaLabel = "unchecked"
		d.RetrievalLabel = "unchecked"
		d.QualityLabel = "unchecked"
		return finalize(d)
	}

	homePath := strings.TrimSpace(in.HomePath)
	projectID := strings.TrimSpace(in.ProjectID)
	if homePath == "" || projectID == "" {
		d.add(Check{
			ID:       CheckStorageExists,
			Severity: SeverityWarn,
			Status:   "unresolved",
			Message:  "Memory storage path unresolved (Atlas Home or project id missing)",
		})
		d.Status = StatusUnavailable
		d.StorageLabel = "unresolved"
		d.SchemaLabel = "n/a"
		d.RetrievalLabel = "n/a"
		d.QualityLabel = "n/a"
		if mode == config.MemoryPersistenceGoverned {
			d.Status = StatusDegraded
			d.add(Check{
				ID:       CheckPersistenceAvailable,
				Severity: SeverityWarn,
				Status:   "unavailable",
				Message:  "Governed persistence cannot resolve storage path",
			})
		}
		return finalize(d)
	}

	if err := memory.ValidateProjectID(projectID); err != nil {
		d.add(Check{
			ID:       CheckStorageExists,
			Severity: SeverityFail,
			Status:   "invalid_scope",
			Message:  "Project scope identity is invalid",
		})
		d.Status = StatusFailed
		d.StorageLabel = "invalid"
		d.SchemaLabel = "n/a"
		d.RetrievalLabel = "n/a"
		d.QualityLabel = "n/a"
		return finalize(d)
	}

	rel, relErr := memory.DatabaseRel(projectID)
	if relErr != nil {
		d.add(Check{
			ID:       CheckStorageExists,
			Severity: SeverityFail,
			Status:   "path_error",
			Message:  "Memory storage path could not be derived",
		})
		d.Status = StatusFailed
		d.StorageLabel = "invalid"
		return finalize(d)
	}
	d.StorageRel = rel

	abs, absErr := fsafety.ContainedJoin(homePath, rel)
	if absErr != nil {
		if errors.Is(absErr, fsafety.ErrSymlink) {
			return finalize(failSymlinkStorage(d, mode))
		}
		d.add(Check{
			ID:       CheckStorageExists,
			Severity: SeverityFail,
			Status:   "unsafe",
			Message:  "Memory storage path is unsafe",
		})
		d.Status = StatusFailed
		d.StorageLabel = "unsafe"
		d.SchemaLabel = "n/a"
		d.RetrievalLabel = "n/a"
		d.QualityLabel = "n/a"
		return finalize(d)
	}

	// --- Storage presence / symlink / regular ---
	exists, existsErr := memory.Exists(homePath, projectID)
	switch {
	case errors.Is(existsErr, memory.ErrSymlinkDatabase),
		errors.Is(existsErr, fsafety.ErrSymlink):
		return finalize(failSymlinkStorage(d, mode))

	case errors.Is(existsErr, memory.ErrNotRegularDatabase):
		d.add(Check{
			ID:       CheckStorageRegular,
			Severity: SeverityFail,
			Status:   "not_regular",
			Message:  "Memory database path is not a regular file",
		})
		d.add(Check{
			ID:       CheckStorageSymlinkSafe,
			Severity: SeverityPass,
			Status:   "ok",
			Message:  "Memory database path is not a symlink",
		})
		d.Status = StatusFailed
		d.StorageLabel = "invalid"
		d.SchemaLabel = "unchecked"
		d.RetrievalLabel = "blocked"
		d.QualityLabel = "unchecked"
		return finalize(d)

	case existsErr != nil:
		d.add(Check{
			ID:       CheckStorageExists,
			Severity: SeverityFail,
			Status:   "error",
			Message:  sanitizeMessage("Memory storage inspect failed: "+existsErr.Error(), homePath),
		})
		d.Status = StatusFailed
		d.StorageLabel = "error"
		d.SchemaLabel = "unchecked"
		d.RetrievalLabel = "blocked"
		d.QualityLabel = "unchecked"
		return finalize(d)

	case !exists:
		d.add(Check{
			ID:       CheckStorageExists,
			Severity: SeverityInfo,
			Status:   "missing",
			Message:  "Memory database is not present yet",
		})
		d.add(Check{
			ID:       CheckStorageSymlinkSafe,
			Severity: SeverityPass,
			Status:   "ok",
			Message:  "No Memory database symlink present",
		})
		d.add(Check{
			ID:       CheckStorageRegular,
			Severity: SeverityInfo,
			Status:   "missing",
			Message:  "Memory database file not created yet",
		})
		d.StorageLabel = "missing"
		d.SchemaLabel = "n/a"
		d.RetrievalLabel = "unavailable"
		d.QualityLabel = "0 anomalies"
		if mode == config.MemoryPersistenceGoverned {
			// Persistence is expected to create on first accept — missing DB is
			// unavailable (informational), not failed.
			d.Status = StatusUnavailable
			parentRel, _ := memory.MemoryDirRel(projectID)
			parentAbs, parentErr := fsafety.ContainedJoin(homePath, parentRel)
			writable := false
			if parentErr == nil {
				if info, err := os.Lstat(parentAbs); err == nil && info.IsDir() {
					writable = pathWritable(parentAbs)
				} else if homeInfo, err := os.Lstat(homePath); err == nil && homeInfo.IsDir() {
					// Parent may not exist yet; Home writability gates future create.
					writable = pathWritable(homePath)
				}
			}
			if writable {
				d.add(Check{
					ID:       CheckStorageWritable,
					Severity: SeverityPass,
					Status:   "ready",
					Message:  "Storage location is writable for governed persistence",
				})
				d.add(Check{
					ID:       CheckPersistenceAvailable,
					Severity: SeverityInfo,
					Status:   "ready",
					Message:  "Governed persistence enabled; database will be created on first accepted propose",
				})
				d.PersistenceLabel = "governed (no db yet)"
			} else {
				d.add(Check{
					ID:              CheckStorageWritable,
					Severity:        SeverityWarn,
					Status:          "not_writable",
					Message:         "Persistence is governed but Memory storage location is not writable",
					RemediationHint: "Ensure Atlas Home project directory is writable",
				})
				d.add(Check{
					ID:       CheckPersistenceAvailable,
					Severity: SeverityWarn,
					Status:   "unavailable",
					Message:  "Governed persistence cannot write Storage",
				})
				d.Status = StatusDegraded
				d.PersistenceLabel = "governed (not writable)"
			}
		} else {
			d.Status = StatusUnavailable
			d.add(Check{
				ID:       CheckPersistenceAvailable,
				Severity: SeverityInfo,
				Status:   "off",
				Message:  "Persistence is off; missing Memory database is expected",
			})
			d.PersistenceLabel = "off"
		}
		d.add(Check{
			ID:       CheckDataObservationCount,
			Severity: SeverityInfo,
			Status:   "0",
			Message:  "Observation count: 0 (no database)",
		})
		return finalize(d)
	}

	// DB exists and is a regular non-symlink file.
	d.add(Check{
		ID:       CheckStorageExists,
		Severity: SeverityPass,
		Status:   "present",
		Message:  "Memory database is present",
	})
	d.add(Check{
		ID:       CheckStorageSymlinkSafe,
		Severity: SeverityPass,
		Status:   "ok",
		Message:  "Memory database is not a symlink",
	})
	d.add(Check{
		ID:       CheckStorageRegular,
		Severity: SeverityPass,
		Status:   "regular",
		Message:  "Memory database is a regular file",
	})

	// Readable via OpenReadOnly (also validates schema).
	store, openErr := memory.OpenReadOnly(homePath, projectID)
	if openErr != nil {
		d.add(Check{
			ID:       CheckStorageReadable,
			Severity: SeverityFail,
			Status:   "unreadable",
			Message:  sanitizeMessage("Memory database is not readable: "+classifyOpenErr(openErr), homePath),
		})
		d.add(Check{
			ID:       CheckRetrievalReadonlyOpen,
			Severity: SeverityFail,
			Status:   "failed",
			Message:  "Read-only open failed",
		})
		switch {
		case errors.Is(openErr, memory.ErrSchemaIncompatible):
			d.add(Check{
				ID:              CheckSchemaCompatible,
				Severity:        SeverityFail,
				Status:          "incompatible",
				Message:         "Memory schema is incompatible with this Atlas version",
				RemediationHint: "Upgrade Atlas or recreate Memory via governed persistence after backup",
			})
			d.add(Check{
				ID:       CheckSchemaMigrations,
				Severity: SeverityFail,
				Status:   "unreadable",
				Message:  "Schema migrations are not readable or incomplete",
			})
			d.SchemaLabel = "incompatible"
		default:
			d.SchemaLabel = "unreadable"
		}
		d.Status = StatusFailed
		d.StorageLabel = "unreadable"
		d.RetrievalLabel = "failed"
		d.QualityLabel = "unchecked"
		if mode == config.MemoryPersistenceGoverned {
			d.add(Check{
				ID:       CheckPersistenceAvailable,
				Severity: SeverityFail,
				Status:   "blocked",
				Message:  "Governed persistence blocked by unreadable storage",
			})
		}
		return finalize(d)
	}
	defer func() { _ = store.Close() }()

	d.add(Check{
		ID:       CheckStorageReadable,
		Severity: SeverityPass,
		Status:   "readable",
		Message:  "Memory database is readable",
	})
	d.add(Check{
		ID:       CheckRetrievalReadonlyOpen,
		Severity: SeverityPass,
		Status:   "ok",
		Message:  "Read-only open succeeded",
	})
	d.StorageLabel = "ready"

	ver, verErr := store.SchemaVersion()
	if verErr != nil {
		d.add(Check{
			ID:       CheckSchemaMigrations,
			Severity: SeverityFail,
			Status:   "unreadable",
			Message:  "Schema migration state is unreadable",
		})
		d.add(Check{
			ID:       CheckSchemaCompatible,
			Severity: SeverityFail,
			Status:   "unknown",
			Message:  "Memory schema compatibility could not be verified",
		})
		d.Status = StatusFailed
		d.SchemaLabel = "unreadable"
		d.RetrievalLabel = "unchecked"
		d.QualityLabel = "unchecked"
		return finalize(d)
	}
	if ver != memory.CurrentSchemaVersion {
		d.add(Check{
			ID:              CheckSchemaCompatible,
			Severity:        SeverityFail,
			Status:          "incompatible",
			Message:         fmt.Sprintf("Memory schema version %d is incompatible (expected %d)", ver, memory.CurrentSchemaVersion),
			RemediationHint: "Open Memory with a matching Atlas version to migrate, or recreate storage",
			Metadata:        checkMeta("version", fmt.Sprintf("%d", ver)),
		})
		d.add(Check{
			ID:       CheckSchemaMigrations,
			Severity: SeverityWarn,
			Status:   "stale",
			Message:  "Migration table readable but schema version mismatched",
		})
		d.Status = StatusFailed
		d.SchemaLabel = "incompatible"
		d.RetrievalLabel = "blocked"
		d.QualityLabel = "unchecked"
		return finalize(d)
	}
	d.add(Check{
		ID:       CheckSchemaCompatible,
		Severity: SeverityPass,
		Status:   "compatible",
		Message:  "Memory schema is compatible",
		Metadata: checkMeta("version", fmt.Sprintf("%d", ver)),
	})
	d.add(Check{
		ID:       CheckSchemaMigrations,
		Severity: SeverityPass,
		Status:   "readable",
		Message:  "Schema migrations are readable",
	})
	d.SchemaLabel = "compatible"

	// Writability (permission-only; never open RW).
	dbWritable := pathWritable(abs)
	dirWritable := pathWritable(filepath.Dir(abs))
	writableOK := dbWritable && dirWritable
	if mode == config.MemoryPersistenceGoverned {
		if writableOK {
			d.add(Check{
				ID:       CheckStorageWritable,
				Severity: SeverityPass,
				Status:   "writable",
				Message:  "Memory storage is writable for governed persistence",
			})
			d.add(Check{
				ID:       CheckPersistenceAvailable,
				Severity: SeverityPass,
				Status:   "available",
				Message:  "Governed persistence write path is available",
			})
			d.PersistenceLabel = "governed"
		} else {
			d.add(Check{
				ID:              CheckStorageWritable,
				Severity:        SeverityWarn,
				Status:          "not_writable",
				Message:         "Persistence is governed but Memory storage is not writable",
				RemediationHint: "Restore write permission on memory.db and its parent directory",
			})
			d.add(Check{
				ID:       CheckPersistenceAvailable,
				Severity: SeverityWarn,
				Status:   "unavailable",
				Message:  "Governed persistence cannot write to Memory storage",
			})
			d.PersistenceLabel = "governed (read-only)"
			d.markDegraded()
		}
	} else {
		d.add(Check{
			ID:       CheckStorageWritable,
			Severity: SeverityInfo,
			Status:   "not_required",
			Message:  "Persistence is off; writability not required",
		})
		d.add(Check{
			ID:       CheckPersistenceAvailable,
			Severity: SeverityInfo,
			Status:   "off",
			Message:  "Persistence is disabled",
		})
		d.PersistenceLabel = "off"
	}

	// Retrieval probe.
	stats, statsErr := store.QualityStats()
	if statsErr != nil {
		d.add(Check{
			ID:       CheckRetrievalBasicQuery,
			Severity: SeverityFail,
			Status:   "failed",
			Message:  sanitizeMessage("Memory quality/query probe failed: "+statsErr.Error(), homePath),
		})
		d.RetrievalLabel = "failed"
		d.QualityLabel = "unchecked"
		d.Status = StatusFailed
		return finalize(d)
	}

	if !stats.FTSPresent {
		d.add(Check{
			ID:              CheckRetrievalBasicQuery,
			Severity:        SeverityFail,
			Status:          "fts_missing",
			Message:         "Memory FTS structures are missing",
			RemediationHint: "Recreate Memory storage via governed persistence after backup",
		})
		d.RetrievalLabel = "failed"
		d.Status = StatusFailed
		return finalize(d)
	}

	if probeErr := store.ProbeRetrieval(); probeErr != nil {
		d.add(Check{
			ID:       CheckRetrievalBasicQuery,
			Severity: SeverityFail,
			Status:   "failed",
			Message:  sanitizeMessage("Basic Memory retrieval probe failed: "+probeErr.Error(), homePath),
		})
		d.RetrievalLabel = "failed"
		d.Status = StatusFailed
		return finalize(d)
	}
	d.add(Check{
		ID:       CheckRetrievalBasicQuery,
		Severity: SeverityPass,
		Status:   "ready",
		Message:  "Basic Memory retrieval probe succeeded",
	})
	d.RetrievalLabel = "ready"

	// --- Quality metrics ---
	d.Metrics = Metrics{
		ObservationCount:      stats.ObservationCount,
		ProjectScopeCount:     stats.ProjectScopeCount,
		DecisionCount:         stats.DecisionCount,
		ConstraintCount:       stats.ConstraintCount,
		PatternCount:          stats.PatternCount,
		DuplicateExactCount:   stats.DuplicateBumpCount,
		DuplicateAnomalyCount: stats.DuplicateAnomalyCount,
		MalformedCount:        stats.MalformedCount,
		OversizedCount:        stats.OversizedCount,
		InvalidScopeCount:     stats.InvalidScopeCount,
		InvalidTypeCount:      stats.InvalidTypeCount,
	}
	anomaly := stats.MalformedCount + stats.OversizedCount + stats.InvalidScopeCount +
		stats.InvalidTypeCount + stats.DuplicateAnomalyCount
	d.Metrics.AnomalyCount = anomaly

	d.add(Check{
		ID:       CheckDataObservationCount,
		Severity: SeverityInfo,
		Status:   fmt.Sprintf("%d", stats.ObservationCount),
		Message:  fmt.Sprintf("Observation count: %d", stats.ObservationCount),
		Metadata: checkMeta("count", fmt.Sprintf("%d", stats.ObservationCount)),
	})

	if stats.MalformedCount > 0 || stats.OversizedCount > 0 {
		d.add(Check{
			ID:       CheckDataRequiredFields,
			Severity: SeverityWarn,
			Status:   "anomalies",
			Message: fmt.Sprintf(
				"Observations with empty or oversized required fields: malformed=%d oversized=%d",
				stats.MalformedCount, stats.OversizedCount,
			),
		})
		d.markDegraded()
	} else {
		d.add(Check{
			ID:       CheckDataRequiredFields,
			Severity: SeverityPass,
			Status:   "ok",
			Message:  "Required observation fields look valid",
		})
	}

	if stats.InvalidScopeCount > 0 || stats.InvalidTypeCount > 0 {
		d.add(Check{
			ID:       CheckDataScopeValidity,
			Severity: SeverityWarn,
			Status:   "anomalies",
			Message: fmt.Sprintf(
				"Unsupported scope/type anomalies: invalid_scope=%d invalid_type=%d",
				stats.InvalidScopeCount, stats.InvalidTypeCount,
			),
		})
		d.markDegraded()
	} else {
		d.add(Check{
			ID:       CheckDataScopeValidity,
			Severity: SeverityPass,
			Status:   "ok",
			Message:  "Observation scopes and types are within V1 taxonomy",
		})
	}

	if stats.DuplicateAnomalyCount > 0 {
		d.add(Check{
			ID:       CheckDataDuplicateAnomaly,
			Severity: SeverityWarn,
			Status:   "anomalies",
			Message:  fmt.Sprintf("Exact duplicate hash anomalies detected: %d", stats.DuplicateAnomalyCount),
		})
		d.markDegraded()
	} else {
		sev := SeverityPass
		msg := "No exact duplicate hash anomalies"
		if stats.DuplicateBumpCount > 0 {
			sev = SeverityInfo
			msg = fmt.Sprintf("No hash anomalies; %d observation(s) have intentional duplicate bumps", stats.DuplicateBumpCount)
		}
		d.add(Check{
			ID:       CheckDataDuplicateAnomaly,
			Severity: sev,
			Status:   "ok",
			Message:  msg,
		})
	}

	if anomaly == 0 {
		d.QualityLabel = "0 anomalies"
	} else {
		d.QualityLabel = fmt.Sprintf("%d anomalies", anomaly)
	}

	if d.Status == "" || d.Status == StatusUnavailable {
		d.Status = StatusHealthy
	}
	if d.Status != StatusFailed && d.Status != StatusDegraded {
		d.Status = StatusHealthy
	}
	return finalize(d)
}

func failSymlinkStorage(d Diagnostics, mode string) Diagnostics {
	d.add(Check{
		ID:              CheckStorageSymlinkSafe,
		Severity:        SeverityFail,
		Status:          "symlink",
		Message:         "Memory database path is a symlink",
		RemediationHint: "Replace the symlink with a regular memory.db under Atlas Home",
	})
	d.add(Check{
		ID:       CheckStorageRegular,
		Severity: SeverityFail,
		Status:   "symlink",
		Message:  "Memory database is not a regular file",
	})
	d.add(Check{
		ID:       CheckStorageExists,
		Severity: SeverityFail,
		Status:   "unsafe",
		Message:  "Memory storage rejected (symlink)",
	})
	d.Status = StatusFailed
	d.StorageLabel = "symlink"
	d.SchemaLabel = "unchecked"
	d.RetrievalLabel = "blocked"
	d.QualityLabel = "unchecked"
	if mode == config.MemoryPersistenceGoverned {
		d.add(Check{
			ID:       CheckPersistenceAvailable,
			Severity: SeverityFail,
			Status:   "blocked",
			Message:  "Governed persistence blocked by unsafe storage",
		})
	}
	return d
}

func (d *Diagnostics) add(c Check) {
	d.Checks = append(d.Checks, c)
}

func (d *Diagnostics) markDegraded() {
	if d.Status != StatusFailed {
		d.Status = StatusDegraded
	}
}

func finalize(d Diagnostics) Diagnostics {
	if d.Status == "" {
		d.Status = StatusUnavailable
	}
	if d.StorageLabel == "" {
		d.StorageLabel = "unknown"
	}
	if d.SchemaLabel == "" {
		d.SchemaLabel = "unknown"
	}
	if d.RetrievalLabel == "" {
		d.RetrievalLabel = "unknown"
	}
	if d.PersistenceLabel == "" {
		d.PersistenceLabel = d.PersistenceMode
	}
	if d.QualityLabel == "" {
		d.QualityLabel = "n/a"
	}
	return d
}

func defaultLimits() []string {
	return []string{
		"diagnostics are read-only (no create/migrate/repair/vacuum/reindex)",
		"no semantic quality scoring or embeddings",
		"convention_count unsupported: V1 has no convention type (pattern_count reported instead)",
		"persistence rejection history is not stored; only current writability/mode is reported",
		"no absolute Atlas Home paths in user-facing messages",
	}
}

func classifyOpenErr(err error) string {
	switch {
	case errors.Is(err, memory.ErrSchemaIncompatible):
		return "schema incompatible"
	case errors.Is(err, memory.ErrDatabaseNotFound):
		return "database not found"
	case errors.Is(err, memory.ErrSymlinkDatabase):
		return "symlink database"
	case errors.Is(err, memory.ErrNotRegularDatabase):
		return "not a regular file"
	case errors.Is(err, memory.ErrUnsafePath):
		return "unsafe path"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	default:
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "permission") {
			return "permission denied"
		}
		if strings.Contains(strings.ToLower(msg), "corrupt") || strings.Contains(strings.ToLower(msg), "malformed") {
			return "corrupt or malformed database"
		}
		return "open failed"
	}
}
