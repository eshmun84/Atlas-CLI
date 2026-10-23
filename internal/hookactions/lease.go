package hookactions

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// AdmitLeaseTTL bounds how long an "admitted" lease blocks a second spawn after
// the parent releases maintain.lock and before the worker re-acquires it.
// Must cover spawn + worker lock acquire without approaching AdmissionTimeout.
const AdmitLeaseTTL = 5 * time.Second

const (
	maintainLeaseName          = "maintain.lease"
	maintainLeaseInvalidPrefix = "maintain.lease.invalid."

	leaseStateAdmitted = "admitted"
	leaseStateRunning  = "running"
)

var errInvalidPersistedAdmissionID = errors.New("hookactions: invalid persisted admission id")

// maintainLease is a short-lived cross-process fence closing the
// release-before-spawn TOCTOU window on maintain.lock.
type maintainLease struct {
	SchemaVersion int    `yaml:"schema_version"`
	State         string `yaml:"state"`
	PID           int    `yaml:"pid"`
	At            string `yaml:"at"`
	AdmissionID   string `yaml:"admission_id,omitempty"`
}

func maintainLeaseRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, maintainLeaseName))
}

func writeMaintainLease(homePath, projectID, state, admissionID string) error {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("hookactions: maintain lease requires home and project id")
	}
	if admissionID != "" && !validAdmissionID(admissionID) {
		return fmt.Errorf("hookactions: invalid admission id")
	}
	doc := maintainLease{
		SchemaVersion: 1,
		State:         state,
		PID:           os.Getpid(),
		At:            time.Now().UTC().Format(time.RFC3339Nano),
		AdmissionID:   admissionID,
	}
	return writeMaintainLeaseDocument(homePath, projectID, doc)
}

func writeMaintainLeaseDocument(homePath, projectID string, doc maintainLease) error {
	if err := guardLeaseReplace(homePath, projectID); err != nil {
		return err
	}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("hookactions: marshal maintain lease: %w", err)
	}
	rel := maintainLeaseRel(projectID)
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, home.DirPermHome, ".atlas-lease-*.tmp"); err != nil {
		return fmt.Errorf("hookactions: write maintain lease: %w", err)
	}
	return nil
}

func readMaintainLease(homePath, projectID string) (maintainLease, bool, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return maintainLease{}, false, fmt.Errorf("hookactions: maintain lease requires home and project id")
	}
	rel := maintainLeaseRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return maintainLease{}, false, nil
		}
		return maintainLease{}, false, err
	}
	return classifyLeaseBytes(data)
}

func clearMaintainLeaseOwned(homePath, projectID, admissionID string) {
	_ = withPendingLock(homePath, projectID, func() error {
		clearMaintainLeaseOwnedLocked(homePath, projectID, admissionID)
		return nil
	})
}

// clearMaintainLeaseOwnedLocked removes only the lease owned by admissionID.
// The caller must hold pending.lock. Empty admission IDs only match legacy
// leases, so a new worker can never claim or clear legacy/foreign state.
func clearMaintainLeaseOwnedLocked(homePath, projectID, admissionID string) bool {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return false
	}
	doc, ok, err := readMaintainLease(homePath, projectID)
	if err != nil || !ok || !leaseOwnedBy(doc, admissionID) {
		return false
	}
	rel := maintainLeaseRel(projectID)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	_ = os.Remove(abs)
	return true
}

func leaseOwnedBy(doc maintainLease, admissionID string) bool {
	if admissionID == "" {
		return doc.AdmissionID == ""
	}
	return validAdmissionID(admissionID) &&
		validAdmissionID(doc.AdmissionID) &&
		doc.AdmissionID == admissionID
}

// invalidLeaseRecoverable reports whether maintain.lock proves an invalid-ID
// lease is stale enough to quarantine. Fresh admitted leases and states whose
// age cannot be established remain untouched and fail closed.
func invalidLeaseRecoverable(doc maintainLease, now time.Time) bool {
	switch strings.TrimSpace(doc.State) {
	case leaseStateRunning:
		// Caller holds maintain.lock, so no running worker owns the flock.
		return true
	case leaseStateAdmitted:
		at, ok := parseLeaseTime(doc.At)
		if !ok {
			return false
		}
		age := now.Sub(at)
		if age < 0 {
			age = 0
		}
		return age > AdmitLeaseTTL
	default:
		return false
	}
}

// quarantineInvalidMaintainLeaseLocked preserves exact primary bytes before
// removing an expired, semantically invalid lease. Caller holds pending.lock
// (and maintain.lock by the only production callsites).
func quarantineInvalidMaintainLeaseLocked(homePath, projectID string) error {
	rel := maintainLeaseRel(projectID)
	original, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		return err
	}
	if err := fsafety.SafeMkdirAll(homePath, pendingStateDirRel(projectID), home.DirPermHome); err != nil {
		return fmt.Errorf("hookactions: invalid lease evidence dir: %w", err)
	}

	var (
		evidenceAbs     string
		evidenceWritten bool
	)
	for attempt := 0; attempt < 16; attempt++ {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return fmt.Errorf("hookactions: invalid lease evidence id: %w", err)
		}
		name := maintainLeaseInvalidPrefix +
			time.Now().UTC().Format("20060102T150405") + "-" +
			hex.EncodeToString(suffix[:]) + ".yaml"
		evidenceRel := filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, name))
		evidenceAbs, err = fsafety.ContainedJoin(homePath, evidenceRel)
		if err != nil {
			return err
		}
		if err := publishExclusiveDurable(evidenceAbs, original); err != nil {
			if os.IsExist(err) || errors.Is(err, fsafety.ErrSymlink) {
				continue
			}
			return fmt.Errorf("hookactions: invalid lease evidence create: %w", err)
		}
		evidenceWritten = true
		break
	}
	if !evidenceWritten {
		return fmt.Errorf("hookactions: invalid lease evidence create: exhausted unique names")
	}

	current, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("hookactions: invalid lease changed during quarantine")
	}
	primaryAbs, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(primaryAbs); err != nil {
		return fmt.Errorf("hookactions: remove invalid lease after evidence: %w", err)
	}
	if err := syncParentDir(primaryAbs); err != nil {
		return fmt.Errorf("hookactions: sync dir after invalid lease remove: %w", err)
	}
	return nil
}

func parseLeaseTime(raw string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		at, err = time.Parse(time.RFC3339, strings.TrimSpace(raw))
		if err != nil {
			return time.Time{}, false
		}
	}
	return at.UTC(), true
}

// leaseBlocksAdmission reports whether an existing lease should prevent a new
// spawn while the caller holds maintain.lock (worker not yet holding it).
func leaseBlocksAdmission(doc maintainLease, now time.Time) bool {
	at, ok := parseLeaseTime(doc.At)
	if !ok {
		return false
	}
	age := now.Sub(at)
	if age < 0 {
		age = 0
	}
	switch strings.TrimSpace(doc.State) {
	case leaseStateAdmitted:
		return age <= AdmitLeaseTTL
	case leaseStateRunning:
		// Holding maintain.lock means no live worker owns the flock; a "running"
		// lease here is stale (crash). Do not block.
		return false
	default:
		return false
	}
}
