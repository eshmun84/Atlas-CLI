package hookactions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// Maintain-last record: minimal structured worker observability under Atlas Home.
// Never stores secrets, source contents, or absolute user file bodies.

const (
	maintainLastName = "maintain-last.yaml"

	MaintainStatusRunning    = "running"
	MaintainStatusComplete   = "complete"
	MaintainStatusIncomplete = "incomplete"

	MaintainOutcomeOK          = "ok"
	MaintainOutcomeSkipped     = "skipped"
	MaintainOutcomeFailed      = "failed"
	MaintainOutcomeTimeout     = "timeout"
	MaintainOutcomeCancelled   = "cancelled"
	MaintainOutcomeUnavailable = "unavailable"
	MaintainOutcomePending     = "pending"

	MaintainErrNone              = "none"
	MaintainErrCEFailed          = "ce_failed"
	MaintainErrCIFailed          = "ci_failed"
	MaintainErrWorkerInterrupted = "worker_interrupted"
	MaintainErrTimeout           = "timeout"
	MaintainErrCancelled         = "cancelled"
	MaintainErrResolveFailed     = "resolve_failed"
	MaintainErrLockFailed        = "lock_failed"
)

// MaintainLastRecord is the persisted worker result snapshot.
type MaintainLastRecord struct {
	SchemaVersion int    `yaml:"schema_version"`
	Status        string `yaml:"status"`
	StartedAt     string `yaml:"started_at"`
	FinishedAt    string `yaml:"finished_at,omitempty"`
	DurationMS    int64  `yaml:"duration_ms"`
	CEOutcome     string `yaml:"ce_outcome"`
	CIOutcome     string `yaml:"ci_outcome"`
	ErrorCode     string `yaml:"error_code"`
	PID           int    `yaml:"pid,omitempty"`
	SessionID     string `yaml:"session_id,omitempty"`
	Trigger       string `yaml:"trigger,omitempty"`
	Generation    int64  `yaml:"generation,omitempty"`
}

type maintainLastOpts struct {
	SessionID  string
	Trigger    string
	Generation int64
}

func maintainLastRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, maintainLastName))
}

// MaintainLastPath returns the absolute Home path for tests.
func MaintainLastPath(homePath, projectID string) string {
	return filepath.Join(strings.TrimSpace(homePath), filepath.FromSlash(maintainLastRel(projectID)))
}

func writeMaintainLast(homePath, projectID string, rec MaintainLastRecord) error {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("hookactions: maintain-last requires home and project id")
	}
	rec.SchemaVersion = 1
	if rec.ErrorCode == "" {
		rec.ErrorCode = MaintainErrNone
	}
	data, err := yaml.Marshal(&rec)
	if err != nil {
		return fmt.Errorf("hookactions: marshal maintain-last: %w", err)
	}
	rel := maintainLastRel(projectID)
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, home.DirPermHome, ".atlas-maintain-*.tmp"); err != nil {
		return fmt.Errorf("hookactions: write maintain-last: %w", err)
	}
	return nil
}

// LoadMaintainLast reads the last worker record (tests / diagnostics).
func LoadMaintainLast(homePath, projectID string) (MaintainLastRecord, bool, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return MaintainLastRecord{}, false, fmt.Errorf("hookactions: maintain-last requires home and project id")
	}
	rel := maintainLastRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return MaintainLastRecord{}, false, nil
		}
		return MaintainLastRecord{}, false, err
	}
	var rec MaintainLastRecord
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return MaintainLastRecord{}, false, fmt.Errorf("hookactions: parse maintain-last: %w", err)
	}
	return rec, true, nil
}

// markPriorIncompleteIfNeeded rewrites a prior running record as incomplete
// (e.g. after SIGKILL) before starting a new worker run. Observability only —
// never blocks admission. Clears the prior session fence so recovery is allowed.
func markPriorIncompleteIfNeeded(homePath, projectID string) {
	rec, ok, err := LoadMaintainLast(homePath, projectID)
	if err != nil || !ok {
		return
	}
	if rec.Status == MaintainStatusComplete && strings.TrimSpace(rec.FinishedAt) != "" {
		return
	}
	if rec.Status == MaintainStatusIncomplete {
		return
	}
	// Unfinished: status running or missing finished_at.
	if rec.Status != MaintainStatusRunning && strings.TrimSpace(rec.FinishedAt) != "" {
		return
	}
	now := time.Now().UTC()
	started, perr := time.Parse(time.RFC3339Nano, rec.StartedAt)
	if perr != nil {
		started, _ = time.Parse(time.RFC3339, rec.StartedAt)
	}
	rec.Status = MaintainStatusIncomplete
	rec.FinishedAt = now.Format(time.RFC3339Nano)
	if !started.IsZero() {
		rec.DurationMS = now.Sub(started.UTC()).Milliseconds()
		if rec.DurationMS < 0 {
			rec.DurationMS = 0
		}
	}
	if rec.ErrorCode == "" || rec.ErrorCode == MaintainErrNone {
		rec.ErrorCode = MaintainErrWorkerInterrupted
	}
	if rec.CEOutcome == "" || rec.CEOutcome == MaintainOutcomePending {
		rec.CEOutcome = MaintainOutcomeFailed
	}
	if rec.CIOutcome == "" || rec.CIOutcome == MaintainOutcomePending {
		rec.CIOutcome = MaintainOutcomeFailed
	}
	_ = writeMaintainLast(homePath, projectID, rec)
	// Incomplete ≠ permanently maintained: allow controlled retry for that session.
	clearSessionMaintain(homePath, projectID, rec.SessionID)
}

func beginMaintainLast(homePath, projectID, sessionID string) (MaintainLastRecord, time.Time) {
	return beginMaintainLastEx(homePath, projectID, maintainLastOpts{SessionID: sessionID})
}

func beginMaintainLastEx(homePath, projectID string, opts maintainLastOpts) (MaintainLastRecord, time.Time) {
	markPriorIncompleteIfNeeded(homePath, projectID)
	started := time.Now().UTC()
	rec := MaintainLastRecord{
		SchemaVersion: 1,
		Status:        MaintainStatusRunning,
		StartedAt:     started.Format(time.RFC3339Nano),
		DurationMS:    0,
		CEOutcome:     MaintainOutcomePending,
		CIOutcome:     MaintainOutcomePending,
		ErrorCode:     MaintainErrNone,
		PID:           os.Getpid(),
		SessionID:     strings.TrimSpace(opts.SessionID),
		Trigger:       strings.TrimSpace(opts.Trigger),
		Generation:    opts.Generation,
	}
	_ = writeMaintainLast(homePath, projectID, rec)
	return rec, started
}

func finishMaintainLast(homePath, projectID, sessionID string, started time.Time, ce, ci, errCode string) {
	finishMaintainLastEx(homePath, projectID, maintainLastOpts{SessionID: sessionID}, started, ce, ci, errCode)
}

func finishMaintainLastEx(homePath, projectID string, opts maintainLastOpts, started time.Time, ce, ci, errCode string) {
	finished := time.Now().UTC()
	dur := finished.Sub(started).Milliseconds()
	if dur < 0 {
		dur = 0
	}
	if errCode == "" {
		errCode = MaintainErrNone
	}
	sid := strings.TrimSpace(opts.SessionID)
	rec := MaintainLastRecord{
		SchemaVersion: 1,
		Status:        MaintainStatusComplete,
		StartedAt:     started.Format(time.RFC3339Nano),
		FinishedAt:    finished.Format(time.RFC3339Nano),
		DurationMS:    dur,
		CEOutcome:     ce,
		CIOutcome:     ci,
		ErrorCode:     errCode,
		PID:           os.Getpid(),
		SessionID:     sid,
		Trigger:       strings.TrimSpace(opts.Trigger),
		Generation:    opts.Generation,
	}
	_ = writeMaintainLast(homePath, projectID, rec)
	finalizeSessionAfterWorker(homePath, projectID, sid, errCode)
}

// finalizeSessionAfterWorker records that the worker finished an attempt for
// this session. Operational CE/CI failures still mark complete so we do not
// re-admit on every subsequent prompt. Timeout/cancel leave the fence cleared
// for controlled retry; incomplete/interrupted is handled in
// markPriorIncompleteIfNeeded (Phase B.8 scenario 5).
func finalizeSessionAfterWorker(homePath, projectID, sessionID, errCode string) {
	if !ValidSessionID(sessionID) {
		return
	}
	switch errCode {
	case MaintainErrTimeout, MaintainErrCancelled:
		clearSessionMaintain(homePath, projectID, sessionID)
	default:
		markSessionComplete(homePath, projectID, sessionID)
	}
}

func mapCEOutcome(r ContextEconomyResult, err error) string {
	if err != nil {
		if errorsIsTimeout(err) {
			return MaintainOutcomeTimeout
		}
		if errorsIsCancel(err) {
			return MaintainOutcomeCancelled
		}
		return MaintainOutcomeFailed
	}
	return outcomeString(r.Outcome)
}

func mapCIOutcome(r CodeIntelResult, err error) string {
	if r.Skipped || r.State == codeintel.StateUnavailable || r.State == codeintel.StateMissing ||
		r.State == codeintel.StateIncompatible {
		return MaintainOutcomeUnavailable
	}
	if err != nil {
		if errorsIsTimeout(err) {
			return MaintainOutcomeTimeout
		}
		if errorsIsCancel(err) {
			return MaintainOutcomeCancelled
		}
		return MaintainOutcomeFailed
	}
	return outcomeString(r.Outcome)
}

func outcomeString(o hooks.Outcome) string {
	switch o {
	case hooks.OutcomeOK:
		return MaintainOutcomeOK
	case hooks.OutcomeSkipped:
		return MaintainOutcomeSkipped
	case hooks.OutcomeFailed:
		return MaintainOutcomeFailed
	case hooks.OutcomeTimeout:
		return MaintainOutcomeTimeout
	case hooks.OutcomeCancelled:
		return MaintainOutcomeCancelled
	default:
		return MaintainOutcomeOK
	}
}

func deriveErrorCode(ce, ci string, ctxErr error) string {
	if ctxErr != nil {
		if errorsIsTimeout(ctxErr) {
			return MaintainErrTimeout
		}
		if errorsIsCancel(ctxErr) {
			return MaintainErrCancelled
		}
	}
	if ce == MaintainOutcomeFailed || ce == MaintainOutcomeTimeout {
		return MaintainErrCEFailed
	}
	if ci == MaintainOutcomeFailed || ci == MaintainOutcomeTimeout {
		return MaintainErrCIFailed
	}
	return MaintainErrNone
}

func errorsIsTimeout(err error) bool {
	return err != nil && errors.Is(err, context.DeadlineExceeded)
}

func errorsIsCancel(err error) bool {
	return err != nil && errors.Is(err, context.Canceled)
}
