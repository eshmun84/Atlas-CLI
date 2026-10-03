package doctor

// Severity is the outcome of a single doctor check.
type Severity string

const (
	SeverityPass Severity = "PASS"
	SeverityWarn Severity = "WARN"
	SeverityFail Severity = "FAIL"
)

// Check is one diagnostic finding.
type Check struct {
	Severity Severity
	Name     string
	Message  string
}

// String formats a check as "PASS name: message".
func (c Check) String() string {
	return string(c.Severity) + " " + c.Name + ": " + c.Message
}
