package hookactions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// Per-session maintenance fence (Phase B.8).
// Distinguishes admission from worker completion; allows retry after failure.
// Stored under Atlas Home project state — not in the product repository.

const (
	sessionMaintainName = "session-maintain.yaml"

	sessionStateAdmitted  = "admitted"
	sessionStateComplete  = "complete"
	sessionStateRetryable = "retryable"

	admissionErrSpawnFailed       = "spawn_failed"
	admissionErrLockBusy          = "lock_busy"
	admissionErrLockFailed        = "lock_failed"
	admissionErrResolveFailed     = "resolve_failed"
	admissionErrCancelled         = "cancelled"
	admissionErrTimeout           = "timeout"
	admissionErrOwnershipLost     = "ownership_lost"
	admissionErrWorkerInterrupted = "worker_interrupted"

	// SessionAdmittedTTL bounds how long a successful spawn suppresses re-admit
	// before the worker reports completion. Covers WorkerTimeout + grace so a
	// crashed worker does not permanently suppress the session.
	SessionAdmittedTTL = WorkerTimeout + 30*time.Second

	// SessionRecordTTL bounds how long a completed session suppresses re-admit
	// and when stale entries are pruned.
	SessionRecordTTL = 24 * time.Hour

	// MaxSessionMaintainEntries caps map growth; oldest entries are pruned.
	MaxSessionMaintainEntries = 256
)

type sessionMaintainFile struct {
	SchemaVersion int                             `yaml:"schema_version"`
	Sessions      map[string]sessionMaintainEntry `yaml:"sessions"`
}

type sessionMaintainEntry struct {
	State       string `yaml:"state"`
	At          string `yaml:"at"`
	PID         int    `yaml:"pid,omitempty"`
	AdmissionID string `yaml:"admission_id,omitempty"`
	ErrorCode   string `yaml:"error_code,omitempty"`
}

func sessionMaintainRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, sessionMaintainName))
}

// SessionMaintainPath returns the absolute Home path (tests/diagnostics).
func SessionMaintainPath(homePath, projectID string) string {
	return filepath.Join(strings.TrimSpace(homePath), filepath.FromSlash(sessionMaintainRel(projectID)))
}

func readSessionMaintain(homePath, projectID string) (sessionMaintainFile, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return sessionMaintainFile{}, fmt.Errorf("hookactions: session-maintain requires home and project id")
	}
	rel := sessionMaintainRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return sessionMaintainFile{SchemaVersion: 1, Sessions: map[string]sessionMaintainEntry{}}, nil
		}
		return sessionMaintainFile{}, err
	}
	return classifySessionBytes(data)
}

func writeSessionMaintain(homePath, projectID string, doc sessionMaintainFile) error {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("hookactions: session-maintain requires home and project id")
	}
	if err := guardSessionReplace(homePath, projectID); err != nil {
		return err
	}
	return writeSessionMaintainRaw(homePath, projectID, doc)
}

func writeSessionMaintainRaw(homePath, projectID string, doc sessionMaintainFile) error {
	doc.SchemaVersion = 1
	if doc.Sessions == nil {
		doc.Sessions = map[string]sessionMaintainEntry{}
	}
	pruneSessionMaintain(&doc, time.Now().UTC())
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("hookactions: marshal session-maintain: %w", err)
	}
	rel := sessionMaintainRel(projectID)
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, home.DirPermHome, ".atlas-session-*.tmp"); err != nil {
		return fmt.Errorf("hookactions: write session-maintain: %w", err)
	}
	return nil
}

func pruneSessionMaintain(doc *sessionMaintainFile, now time.Time) {
	if doc == nil || doc.Sessions == nil {
		return
	}
	for sid, ent := range doc.Sessions {
		// A non-cooperative identity is evidence, not an expired owner. Leave
		// it for admission recovery, which quarantines the document first.
		if !sessionIdentityCooperative(ent.AdmissionID) {
			continue
		}
		at, ok := parseSessionAt(ent.At)
		if !ok || now.Sub(at) > SessionRecordTTL {
			delete(doc.Sessions, sid)
		}
	}
	cooperative := 0
	for _, ent := range doc.Sessions {
		if sessionIdentityCooperative(ent.AdmissionID) {
			cooperative++
		}
	}
	if cooperative <= MaxSessionMaintainEntries {
		return
	}
	type pair struct {
		sid string
		at  time.Time
	}
	items := make([]pair, 0, len(doc.Sessions))
	for sid, ent := range doc.Sessions {
		if !sessionIdentityCooperative(ent.AdmissionID) {
			continue
		}
		at, ok := parseSessionAt(ent.At)
		if !ok {
			delete(doc.Sessions, sid)
			cooperative--
			continue
		}
		items = append(items, pair{sid: sid, at: at})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	for cooperative > MaxSessionMaintainEntries && len(items) > 0 {
		delete(doc.Sessions, items[0].sid)
		items = items[1:]
		cooperative--
	}
}

func parseSessionAt(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		at, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, false
		}
	}
	return at.UTC(), true
}

// sessionBlocksAdmission reports whether sessionID already has a live admit or
// a completed maintenance that should suppress another spawn.
func sessionBlocksAdmission(homePath, projectID, sessionID string, now time.Time) bool {
	if !ValidSessionID(sessionID) {
		return false
	}
	doc, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		return false
	}
	ent, ok := doc.Sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return false
	}
	at, ok := parseSessionAt(ent.At)
	if !ok {
		return false
	}
	age := now.Sub(at)
	if age < 0 {
		age = 0
	}
	switch strings.TrimSpace(ent.State) {
	case sessionStateComplete:
		return age <= SessionRecordTTL
	case sessionStateAdmitted:
		return age <= SessionAdmittedTTL
	default:
		return false
	}
}

func markSessionAdmittedLocked(homePath, projectID, sessionID, admissionID string) error {
	if !ValidSessionID(sessionID) {
		return nil
	}
	if !validAdmissionID(admissionID) {
		return fmt.Errorf("hookactions: invalid admission id")
	}
	doc, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		return err
	}
	doc.Sessions[strings.TrimSpace(sessionID)] = sessionMaintainEntry{
		State:       sessionStateAdmitted,
		At:          time.Now().UTC().Format(time.RFC3339Nano),
		PID:         os.Getpid(),
		AdmissionID: admissionID,
	}
	return writeSessionMaintain(homePath, projectID, doc)
}

func markSessionCompleteOwned(homePath, projectID, sessionID, admissionID string) {
	if !ValidSessionID(sessionID) {
		return
	}
	_ = withPendingLock(homePath, projectID, func() error {
		doc, err := readSessionMaintain(homePath, projectID)
		if err != nil {
			return err
		}
		sid := strings.TrimSpace(sessionID)
		ent, ok := doc.Sessions[sid]
		if !ok || !sessionOwnedBy(ent, admissionID) {
			return nil
		}
		state := strings.TrimSpace(ent.State)
		if state != sessionStateAdmitted &&
			(state != sessionStateRetryable || strings.TrimSpace(ent.ErrorCode) != admissionErrWorkerInterrupted) {
			return nil
		}
		ent.State = sessionStateComplete
		ent.At = time.Now().UTC().Format(time.RFC3339Nano)
		ent.PID = os.Getpid()
		ent.ErrorCode = ""
		doc.Sessions[sid] = ent
		return writeSessionMaintain(homePath, projectID, doc)
	})
}

func markSessionRetryableOwned(homePath, projectID, sessionID, admissionID, errorCode string) {
	if !ValidSessionID(sessionID) {
		return
	}
	_ = withPendingLock(homePath, projectID, func() error {
		return markSessionStateOwnedLocked(homePath, projectID, sessionID, admissionID, sessionStateAdmitted, sessionStateRetryable, errorCode)
	})
}

// markSessionStateOwnedLocked is a CAS over one session entry and its source
// state. The caller must hold pending.lock. A tokenized caller cannot mutate a
// legacy, foreign, or terminal entry.
func markSessionStateOwnedLocked(homePath, projectID, sessionID, admissionID, expectedState, nextState, errorCode string) error {
	doc, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		return err
	}
	sid := strings.TrimSpace(sessionID)
	ent, ok := doc.Sessions[sid]
	if !ok || !sessionOwnedBy(ent, admissionID) || strings.TrimSpace(ent.State) != expectedState {
		return nil
	}
	ent.State = nextState
	ent.At = time.Now().UTC().Format(time.RFC3339Nano)
	ent.PID = os.Getpid()
	ent.ErrorCode = strings.TrimSpace(errorCode)
	doc.Sessions[sid] = ent
	return writeSessionMaintain(homePath, projectID, doc)
}

func sessionOwnedBy(ent sessionMaintainEntry, admissionID string) bool {
	if admissionID == "" {
		return ent.AdmissionID == ""
	}
	return validAdmissionID(admissionID) &&
		validAdmissionID(ent.AdmissionID) &&
		ent.AdmissionID == admissionID
}

func markAdmissionSessionsRetryableLocked(homePath, projectID, admissionID, errorCode string) error {
	if admissionID != "" && !validAdmissionID(admissionID) {
		return fmt.Errorf("hookactions: invalid admission id")
	}
	doc, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		return err
	}
	changed := false
	for sid, ent := range doc.Sessions {
		if !sessionOwnedBy(ent, admissionID) || strings.TrimSpace(ent.State) != sessionStateAdmitted {
			continue
		}
		ent.State = sessionStateRetryable
		ent.At = time.Now().UTC().Format(time.RFC3339Nano)
		ent.PID = os.Getpid()
		ent.ErrorCode = strings.TrimSpace(errorCode)
		doc.Sessions[sid] = ent
		changed = true
	}
	if !changed {
		return nil
	}
	return writeSessionMaintain(homePath, projectID, doc)
}

// LoadSessionMaintainState returns the persisted state for a session (tests).
func LoadSessionMaintainState(homePath, projectID, sessionID string) (state string, ok bool, err error) {
	if !ValidSessionID(sessionID) {
		return "", false, nil
	}
	doc, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		return "", false, err
	}
	ent, ok := doc.Sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return "", false, nil
	}
	return strings.TrimSpace(ent.State), true, nil
}
