package context

import "time"

// Freshness labels for Status/Doctor.
const (
	StatusNA         = "n/a"
	StatusMissing    = "missing"
	StatusPresent    = "present"
	StatusStale      = "stale"
	StatusUnreadable = "unreadable"
)

// ComputeFreshnessFingerprint rebuilds a live fingerprint without writing.
func ComputeFreshnessFingerprint(root, projectName string) (string, error) {
	idx, err := BuildIndex(root, projectName, time.Unix(0, 0).UTC())
	if err != nil {
		return "", err
	}
	return idx.Fingerprint, nil
}

// IsStale reports whether the stored index fingerprint no longer matches the tree.
func IsStale(root string, stored IndexDocument) (bool, error) {
	live, err := ComputeFreshnessFingerprint(root, stored.ProjectName)
	if err != nil {
		return false, err
	}
	if stored.Fingerprint == "" {
		return true, nil
	}
	return live != stored.Fingerprint, nil
}

// DescribeStale returns a short human reason when stale.
func DescribeStale(stale bool) string {
	if stale {
		return "index fingerprint differs from current project tree"
	}
	return ""
}
