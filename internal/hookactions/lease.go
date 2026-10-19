package hookactions

import (
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
	maintainLeaseName = "maintain.lease"

	leaseStateAdmitted = "admitted"
	leaseStateRunning  = "running"
)

// maintainLease is a short-lived cross-process fence closing the
// release-before-spawn TOCTOU window on maintain.lock.
type maintainLease struct {
	SchemaVersion int    `yaml:"schema_version"`
	State         string `yaml:"state"`
	PID           int    `yaml:"pid"`
	At            string `yaml:"at"`
}

func maintainLeaseRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, maintainLeaseName))
}

func writeMaintainLease(homePath, projectID, state string) error {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("hookactions: maintain lease requires home and project id")
	}
	doc := maintainLease{
		SchemaVersion: 1,
		State:         state,
		PID:           os.Getpid(),
		At:            time.Now().UTC().Format(time.RFC3339Nano),
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
	var doc maintainLease
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Corrupt lease → treat as absent (fail-open reclaim).
		return maintainLease{}, false, nil
	}
	if strings.TrimSpace(doc.State) == "" {
		return maintainLease{}, false, nil
	}
	return doc, true, nil
}

func clearMaintainLease(homePath, projectID string) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return
	}
	rel := maintainLeaseRel(projectID)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	_ = os.Remove(abs)
}

// leaseBlocksAdmission reports whether an existing lease should prevent a new
// spawn while the caller holds maintain.lock (worker not yet holding it).
func leaseBlocksAdmission(doc maintainLease, now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(doc.At))
	if err != nil {
		at, err = time.Parse(time.RFC3339, strings.TrimSpace(doc.At))
		if err != nil {
			return false
		}
	}
	age := now.Sub(at.UTC())
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
