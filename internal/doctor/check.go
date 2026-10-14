package doctor

// Severity is the outcome of a single doctor check.
type Severity string

const (
	SeverityPass Severity = "PASS"
	SeverityInfo Severity = "INFO"
	SeverityWarn Severity = "WARN"
	SeverityFail Severity = "FAIL"
)

// Repairability classifies whether Runtime Repair (or a human) can act.
type Repairability string

const (
	RepairAtlas       Repairability = "atlas-repair"
	RepairConfirm     Repairability = "confirm-repair"
	RepairDiagnostic  Repairability = "diagnostic-only"
	RepairManual      Repairability = "manual"
	RepairUnspecified Repairability = ""
)

// Check is one diagnostic finding.
type Check struct {
	Severity          Severity
	Name              string
	Message           string
	Repairability     Repairability
	RecommendedAction string
}

// String formats a check as "PASS name: message".
func (c Check) String() string {
	return string(c.Severity) + " " + c.Name + ": " + c.Message
}

func pass(name, message string) Check {
	return Check{Severity: SeverityPass, Name: name, Message: message}
}

func info(name, message string) Check {
	return Check{Severity: SeverityInfo, Name: name, Message: message}
}

func warn(name, message string) Check {
	return warnAction(name, message, "")
}

func warnAction(name, message, action string) Check {
	return Check{
		Severity:          SeverityWarn,
		Name:              name,
		Message:           message,
		Repairability:     RepairDiagnostic,
		RecommendedAction: action,
	}
}

func fail(name, message string, repair Repairability, action string) Check {
	return Check{
		Severity:          SeverityFail,
		Name:              name,
		Message:           message,
		Repairability:     repair,
		RecommendedAction: action,
	}
}

// ValidRepairability reports whether r is one of the supported enum values.
func ValidRepairability(r Repairability) bool {
	switch r {
	case RepairAtlas, RepairConfirm, RepairDiagnostic, RepairManual:
		return true
	default:
		return false
	}
}

// ValidateRepairabilityMetadata ensures every FAIL has an explicit supported
// Repairability, and every actionable WARN (non-empty RecommendedAction or
// already tagged) uses a supported value when Repairability is set.
func ValidateRepairabilityMetadata(report Report) []string {
	var problems []string
	for _, c := range report.Checks {
		switch c.Severity {
		case SeverityFail:
			if c.Repairability == "" {
				problems = append(problems, c.Name+": FAIL missing Repairability")
				continue
			}
			if !ValidRepairability(c.Repairability) {
				problems = append(problems, c.Name+": FAIL invalid Repairability "+string(c.Repairability))
			}
		case SeverityWarn:
			if c.Repairability == "" {
				// Actionable WARNs should carry Repairability; treat empty as a
				// problem so newly introduced WARN paths stay classified.
				problems = append(problems, c.Name+": WARN missing Repairability")
				continue
			}
			if !ValidRepairability(c.Repairability) {
				problems = append(problems, c.Name+": WARN invalid Repairability "+string(c.Repairability))
			}
		}
	}
	return problems
}
