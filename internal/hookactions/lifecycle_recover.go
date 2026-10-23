package hookactions

import (
	"fmt"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

const (
	maintainLeaseCorruptPrefix   = "maintain.lease.corrupt."
	sessionMaintainCorruptPrefix = "session-maintain.corrupt."
	sessionMaintainInvalidPrefix = "session-maintain.invalid."
)

func classifySchemaDocument(data []byte) (version int, present bool, err error) {
	var header struct {
		SchemaVersion *int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return 0, false, err
	}
	if header.SchemaVersion == nil {
		return 0, false, nil
	}
	return *header.SchemaVersion, true, nil
}

func classifyLeaseBytes(data []byte) (maintainLease, bool, error) {
	version, present, err := classifySchemaDocument(data)
	if err != nil || !present {
		return maintainLease{}, false, fmt.Errorf("%w: %w: parse", errLeaseCorrupt, errLeaseMalformed)
	}
	if version != 1 {
		return maintainLease{}, false, fmt.Errorf("%w: %w: schema_version %d", errLeaseCorrupt, errLeaseSchemaIncompatible, version)
	}
	var doc maintainLease
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return maintainLease{}, false, fmt.Errorf("%w: %w: parse", errLeaseCorrupt, errLeaseMalformed)
	}
	if strings.TrimSpace(doc.State) == "" {
		return maintainLease{}, false, nil
	}
	return doc, true, nil
}

func classifySessionBytes(data []byte) (sessionMaintainFile, error) {
	version, present, err := classifySchemaDocument(data)
	if err != nil || !present {
		return sessionMaintainFile{}, fmt.Errorf("%w: %w: parse", errSessionCorrupt, errSessionMalformed)
	}
	if version != 1 {
		return sessionMaintainFile{}, fmt.Errorf("%w: %w: schema_version %d", errSessionCorrupt, errSessionSchemaIncompatible, version)
	}
	var doc sessionMaintainFile
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return sessionMaintainFile{}, fmt.Errorf("%w: %w: parse", errSessionCorrupt, errSessionMalformed)
	}
	if doc.Sessions == nil {
		doc.Sessions = map[string]sessionMaintainEntry{}
	}
	doc.SchemaVersion = 1
	return doc, nil
}

func sessionIdentityCooperative(admissionID string) bool {
	return admissionID == "" || validAdmissionID(admissionID)
}

func sessionDocHasNonCooperative(doc sessionMaintainFile) bool {
	for _, ent := range doc.Sessions {
		if !sessionIdentityCooperative(ent.AdmissionID) {
			return true
		}
	}
	return false
}

func guardLeaseReplace(homePath, projectID string) error {
	data, err := fsafety.ReadFileContained(homePath, maintainLeaseRel(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, _, err = classifyLeaseBytes(data)
	return err
}

func guardSessionReplace(homePath, projectID string) error {
	data, err := fsafety.ReadFileContained(homePath, sessionMaintainRel(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, err = classifySessionBytes(data)
	return err
}

// recoverMalformedLeaseLocked quarantines a syntactically corrupt lease and
// removes it. A schema other than 1 is returned untouched. A readable lease,
// including one with a non-canonical admission id, is not modified.
// Caller holds pending.lock.
func recoverMalformedLeaseLocked(homePath, projectID string) error {
	rel := maintainLeaseRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_, _, classErr := classifyLeaseBytes(data)
	if classErr == nil {
		return nil
	}
	if !leaseCorruptionRecoverable(classErr) {
		return classErr
	}
	if _, err := publishUniqueEvidence(homePath, projectID, maintainLeaseCorruptPrefix, data); err != nil {
		return fmt.Errorf("hookactions: quarantine corrupt lease: %w", err)
	}
	if err := removePrimaryAfterEvidence(homePath, rel, data); err != nil {
		return fmt.Errorf("hookactions: quarantine corrupt lease: %w", err)
	}
	return nil
}

// recoverMalformedSessionLocked quarantines a syntactically corrupt session
// fence and replaces it with an empty schema-1 document. A future schema is
// not quarantined or replaced. Entries of a readable document are preserved.
// Caller holds pending.lock.
func recoverMalformedSessionLocked(homePath, projectID string) error {
	rel := sessionMaintainRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, classErr := classifySessionBytes(data); classErr == nil {
		return nil
	} else if !sessionCorruptionRecoverable(classErr) {
		return classErr
	}
	if _, err := publishUniqueEvidence(homePath, projectID, sessionMaintainCorruptPrefix, data); err != nil {
		return fmt.Errorf("hookactions: quarantine corrupt session: %w", err)
	}
	empty := sessionMaintainFile{SchemaVersion: 1, Sessions: map[string]sessionMaintainEntry{}}
	out, err := yaml.Marshal(&empty)
	if err != nil {
		return err
	}
	if err := replacePrimaryAfterEvidence(homePath, rel, data, out, ".atlas-session-*.tmp"); err != nil {
		return fmt.Errorf("hookactions: reset corrupt session: %w", err)
	}
	return nil
}

// recoverNonCooperativeSessionLocked preserves exact bytes, then rewrites a
// schema-1 fence without entries whose admission id is neither empty nor
// canonical. Those entries are not owners and must not keep blocking admission.
// Caller holds maintain.lock and pending.lock.
func recoverNonCooperativeSessionLocked(homePath, projectID string) error {
	rel := sessionMaintainRel(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	doc, classErr := classifySessionBytes(data)
	if classErr != nil {
		return classErr
	}
	if !sessionDocHasNonCooperative(doc) {
		return nil
	}
	if _, err := publishUniqueEvidence(homePath, projectID, sessionMaintainInvalidPrefix, data); err != nil {
		return fmt.Errorf("hookactions: quarantine invalid session: %w", err)
	}
	for sid, ent := range doc.Sessions {
		if !sessionIdentityCooperative(ent.AdmissionID) {
			delete(doc.Sessions, sid)
		}
	}
	doc.SchemaVersion = 1
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	if err := replacePrimaryAfterEvidence(homePath, rel, data, out, ".atlas-session-*.tmp"); err != nil {
		return fmt.Errorf("hookactions: rewrite invalid session: %w", err)
	}
	return nil
}
