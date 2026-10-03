package doctor_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
)

func TestWriteReport_IncludesSectionsAndCounts(t *testing.T) {
	t.Parallel()

	report := doctor.Report{
		Checks: []doctor.Check{
			{Severity: doctor.SeverityPass, Name: "workspace", Message: "discovery completed"},
			{Severity: doctor.SeverityWarn, Name: "atlas config", Message: ".atlas/config.yaml not found"},
			{Severity: doctor.SeverityFail, Name: "workspace", Message: "discovery failed"},
		},
	}

	var buf bytes.Buffer
	doctor.WriteReport(&buf, report)
	out := buf.String()

	for _, want := range []string{
		"Atlas Doctor",
		"Checks:",
		"PASS workspace: discovery completed",
		"WARN atlas config: .atlas/config.yaml not found",
		"FAIL workspace: discovery failed",
		"Summary:",
		"Passed: 1",
		"Warnings: 1",
		"Failed: 1",
		"Result: not ready",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in report:\n%s", want, out)
		}
	}
}
