package hookactions

import (
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

// Corrupt evidence leaves are uniquely named:
//
//	maintain-pending.corrupt.<utc>-<hex>.yaml
const maintainPendingCorruptPrefix = "maintain-pending.corrupt."

// Phase C.2b: durable post-code maintenance pending state under Atlas Home.
// Generations are independent of Phase B session fences.

const (
	maintainPendingName = "maintain-pending.yaml"
	pendingLockName     = "pending.lock"

	pendingSchemaVersion = 1

	ownerStateNone            = "none"
	ownerStateReserved        = "reserved"
	ownerStateRunningWait     = "running_wait"
	ownerStateRunningMaintain = "running_maintain"

	// DebounceDelay is the quiet period after the last bump before maintain runs.
	DebounceDelay = 3 * time.Second
	// MaxCoalesceWindow caps how long continuous bumps can defer work.
	MaxCoalesceWindow = 30 * time.Second
	// OwnerLeaseTTL bounds scheduler ownership without relying on PID.
	// Covers coalesce wait + one WorkerTimeout maintain + grace.
	OwnerLeaseTTL = MaxCoalesceWindow + WorkerTimeout + 30*time.Second
	// OwnerHeartbeatInterval renews the owner lease during waits and CE/CI.
	OwnerHeartbeatInterval = 30 * time.Second
	// AbsoluteProcessDeadline caps a single scheduler process lifetime.
	AbsoluteProcessDeadline = 10 * time.Minute
	// MaxSleepSlice bounds individual debounce sleeps for due_at reactivity.
	MaxSleepSlice = 1 * time.Second

	retryBackoff1 = 15 * time.Second
	retryBackoff2 = 60 * time.Second
	retryBackoff3 = 5 * time.Minute
)

// MaintainPendingDocument is the Home-persisted post-code work queue.
// Never stores secrets, tool bodies, or edited file paths.
type MaintainPendingDocument struct {
	SchemaVersion       int    `yaml:"schema_version"`
	PendingGeneration   int64  `yaml:"pending_generation"`
	ProcessedGeneration int64  `yaml:"processed_generation"`
	LastEditAt          string `yaml:"last_edit_at,omitempty"`
	DueAt               string `yaml:"due_at,omitempty"`
	BurstStartedAt      string `yaml:"burst_started_at,omitempty"`
	OwnerToken          string `yaml:"owner_token,omitempty"`
	OwnerFence          int64  `yaml:"owner_fence"`
	OwnerLeaseUntil     string `yaml:"owner_lease_until,omitempty"`
	OwnerState          string `yaml:"owner_state,omitempty"`
	SchedulerPID        int    `yaml:"scheduler_pid,omitempty"`
	Attempt             int    `yaml:"attempt,omitempty"`
	NextRetryAt         string `yaml:"next_retry_at,omitempty"`
	LastErrorCode       string `yaml:"last_error_code,omitempty"`
}

func maintainPendingRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, maintainPendingName))
}

func pendingLockRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, pendingLockName))
}

func pendingStateDirRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState))
}

// MaintainPendingPath returns the absolute Home path (tests/diagnostics).
func MaintainPendingPath(homePath, projectID string) string {
	return filepath.Join(strings.TrimSpace(homePath), filepath.FromSlash(maintainPendingRel(projectID)))
}

// PendingLockPath returns the absolute pending.lock path (tests/diagnostics).
func PendingLockPath(homePath, projectID string) string {
	return filepath.Join(strings.TrimSpace(homePath), filepath.FromSlash(pendingLockRel(projectID)))
}

// ListMaintainPendingCorruptEvidence returns absolute paths of quarantined
// corrupt evidence files for a project (sorted by name). Atlas Home only.
func ListMaintainPendingCorruptEvidence(homePath, projectID string) ([]string, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return nil, fmt.Errorf("hookactions: maintain-pending quarantine list requires home and project id")
	}
	dirRel := pendingStateDirRel(projectID)
	dirAbs, err := fsafety.ContainedJoin(homePath, dirRel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, maintainPendingCorruptPrefix) || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		out = append(out, filepath.Join(dirAbs, name))
	}
	return out, nil
}

func emptyPendingDoc() MaintainPendingDocument {
	return MaintainPendingDocument{
		SchemaVersion: pendingSchemaVersion,
		OwnerState:    ownerStateNone,
		LastErrorCode: MaintainErrNone,
	}
}

// recoveryDirtyPendingDoc starts a new reconciliation epoch after corrupt reset.
// pending=1 / processed=0 means "must evaluate CE/CI" — not reconstructed history.
func recoveryDirtyPendingDoc(now time.Time) MaintainPendingDocument {
	doc := emptyPendingDoc()
	doc.PendingGeneration = 1
	doc.ProcessedGeneration = 0
	doc.LastEditAt = formatPendingTime(now)
	doc.BurstStartedAt = formatPendingTime(now)
	doc.DueAt = formatPendingTime(now) // due immediately; scheduler may debounce further bumps
	doc.LastErrorCode = MaintainErrNone
	clearOwnershipFields(&doc)
	return doc
}

// writeCorruptEvidenceExclusive creates a unique quarantine leaf with O_EXCL.
// On failure the primary maintain-pending.yaml must remain untouched.
var writeCorruptEvidenceExclusive = writeCorruptEvidenceExclusiveProd

func writeCorruptEvidenceExclusiveProd(homePath, projectID string, data []byte) (rel string, err error) {
	if err := fsafety.SafeMkdirAll(homePath, pendingStateDirRel(projectID), home.DirPermHome); err != nil {
		return "", fmt.Errorf("hookactions: quarantine dir: %w", err)
	}
	for attempt := 0; attempt < 16; attempt++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", fmt.Errorf("hookactions: quarantine id: %w", err)
		}
		stamp := pendingNow().UTC().Format("20060102T150405")
		name := maintainPendingCorruptPrefix + stamp + "-" + hex.EncodeToString(b[:]) + ".yaml"
		rel = filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, name))
		abs, err := fsafety.ContainedJoin(homePath, rel)
		if err != nil {
			return "", err
		}
		f, err := createExclusiveNoFollow(abs, 0o600)
		if err != nil {
			if os.IsExist(err) || errors.Is(err, fsafety.ErrSymlink) {
				continue
			}
			return "", fmt.Errorf("hookactions: quarantine create: %w", err)
		}
		n, werr := f.Write(data)
		if werr == nil && n != len(data) {
			werr = fmt.Errorf("short write %d/%d", n, len(data))
		}
		if werr == nil {
			werr = fsafety.SyncFileDurable(f)
		}
		cerr := f.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(abs)
			if werr != nil {
				return "", fmt.Errorf("hookactions: quarantine write: %w", werr)
			}
			return "", fmt.Errorf("hookactions: quarantine close: %w", cerr)
		}
		if err := syncParentDir(abs); err != nil {
			_ = os.Remove(abs)
			return "", fmt.Errorf("hookactions: quarantine sync dir: %w", err)
		}
		return rel, nil
	}
	return "", fmt.Errorf("hookactions: quarantine create: exhausted unique names")
}

func loadPending(homePath, projectID string) (MaintainPendingDocument, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return MaintainPendingDocument{}, fmt.Errorf("hookactions: maintain-pending requires home and project id")
	}
	rel := maintainPendingRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			// Absent file = empty queue (no prior work). Distinct from corrupt.
			return emptyPendingDoc(), nil
		}
		return MaintainPendingDocument{}, err
	}
	doc, err := decodePending(data)
	if err != nil {
		return MaintainPendingDocument{}, err
	}
	return normalizePending(doc), nil
}

func decodePending(data []byte) (MaintainPendingDocument, error) {
	var header struct {
		SchemaVersion *int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return MaintainPendingDocument{}, fmt.Errorf("%w: %w: parse", errPendingCorrupt, errPendingMalformed)
	}
	if header.SchemaVersion == nil {
		return MaintainPendingDocument{}, fmt.Errorf("%w: %w: schema_version missing", errPendingCorrupt, errPendingMalformed)
	}
	if *header.SchemaVersion != pendingSchemaVersion {
		return MaintainPendingDocument{}, fmt.Errorf("%w: %w: schema_version %d", errPendingCorrupt, errPendingSchemaIncompatible, *header.SchemaVersion)
	}

	var doc MaintainPendingDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Corrupt: do not invent an empty queue; preserve evidence on disk.
		return MaintainPendingDocument{}, fmt.Errorf("%w: %w: parse", errPendingCorrupt, errPendingMalformed)
	}
	return doc, nil
}

func normalizePending(doc MaintainPendingDocument) MaintainPendingDocument {
	// Caller must already validate schema_version.
	if doc.PendingGeneration < 0 {
		doc.PendingGeneration = 0
	}
	if doc.ProcessedGeneration < 0 {
		doc.ProcessedGeneration = 0
	}
	if doc.ProcessedGeneration > doc.PendingGeneration {
		// Impossible under correct writers; clamp without inventing pending work.
		doc.ProcessedGeneration = doc.PendingGeneration
	}
	if doc.OwnerFence < 0 {
		doc.OwnerFence = 0
	}
	if strings.TrimSpace(doc.OwnerState) == "" {
		doc.OwnerState = ownerStateNone
	}
	if doc.LastErrorCode == "" {
		doc.LastErrorCode = MaintainErrNone
	}
	doc.SchemaVersion = pendingSchemaVersion
	return doc
}

func savePending(homePath, projectID string, doc MaintainPendingDocument) error {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("hookactions: maintain-pending requires home and project id")
	}
	// Refuse to overwrite an existing corrupt/incompatible evidence file.
	if _, err := loadPending(homePath, projectID); err != nil && ErrPendingCorrupt(err) {
		return err
	}
	doc = normalizePending(doc)
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("hookactions: marshal maintain-pending: %w", err)
	}
	rel := maintainPendingRel(projectID)
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, home.DirPermHome, ".atlas-pending-*.tmp"); err != nil {
		return fmt.Errorf("hookactions: write maintain-pending: %w", err)
	}
	return nil
}

// QuarantineAndResetCorruptPending is the controlled recovery path for untrusted
// maintain-pending.yaml. It:
//  1. Preserves a unique quarantine copy (never overwrites prior evidence)
//  2. Only then replaces the primary with a new dirty epoch (pending=1, processed=0)
//
// If quarantine fails, the primary file is left untouched.
// Does not run CE/CI and does not hold pending.lock across maintain work.
// No-op when the primary is absent or already valid.
// Incompatible schemas are never quarantined or reset.
func QuarantineAndResetCorruptPending(homePath, projectID string) error {
	return withPendingLock(homePath, projectID, func() error {
		homePath = strings.TrimSpace(homePath)
		projectID = strings.TrimSpace(projectID)
		rel := maintainPendingRel(projectID)
		data, err := fsafety.ReadFileContained(homePath, rel)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if _, err := decodePending(data); err == nil {
			return nil
		} else if !pendingCorruptionRecoverable(err) {
			return err
		}

		// A. Preserve evidence first — failure aborts without touching primary.
		if _, err := writeCorruptEvidenceExclusive(homePath, projectID, data); err != nil {
			return fmt.Errorf("hookactions: quarantine maintain-pending: %w", err)
		}

		// B. Conservative dirty epoch: obligates a fresh CE/CI evaluation.
		fresh := recoveryDirtyPendingDoc(pendingNow())
		out, err := yaml.Marshal(&fresh)
		if err != nil {
			return fmt.Errorf("hookactions: marshal recovery maintain-pending: %w", err)
		}
		// Bypass savePending corrupt guard (primary is still corrupt until this write).
		if err := writeRecoveryPendingPrimary(homePath, rel, out); err != nil {
			// Quarantine already preserved; primary remains corrupt → still fail-closed.
			return fmt.Errorf("hookactions: reset maintain-pending: %w", err)
		}
		return nil
	})
}

// writeRecoveryPendingPrimary replaces the primary pending file after evidence
// quarantine. Overridable in tests to simulate crash between quarantine and reset.
var writeRecoveryPendingPrimary = writeRecoveryPendingPrimaryProd

func writeRecoveryPendingPrimaryProd(homePath, rel string, data []byte) error {
	return fsafety.AtomicWriteContainedDirDurable(homePath, rel, data, 0o600, home.DirPermHome, ".atlas-pending-*.tmp")
}

// LoadMaintainPending reads pending state (tests / diagnostics).
func LoadMaintainPending(homePath, projectID string) (MaintainPendingDocument, error) {
	return loadPending(homePath, projectID)
}

// Testable clock / sleep seams (production uses UTC now and time.Sleep).
var (
	pendingNow   = func() time.Time { return time.Now().UTC() }
	pendingSleep = time.Sleep
)

func parsePendingTime(raw string) (time.Time, bool) {
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

func formatPendingTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func retryBackoff(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return retryBackoff1
	case attempt == 2:
		return retryBackoff2
	default:
		return retryBackoff3
	}
}

func pendingHasWork(doc MaintainPendingDocument) bool {
	return doc.PendingGeneration > doc.ProcessedGeneration
}
