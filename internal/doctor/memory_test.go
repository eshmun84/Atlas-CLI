package doctor

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/memoryhealth"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestEvaluate_MemoryHealthyMapsToPass(t *testing.T) {
	report := Evaluate(inspect.Inspection{
		Runtime: runtime.Health{
			Initialized: true,
			ConfigLoads: true,
			Memory: memoryhealth.Diagnostics{
				Status:           memoryhealth.StatusHealthy,
				StorageLabel:     "ready",
				SchemaLabel:      "compatible",
				RetrievalLabel:   "ready",
				PersistenceLabel: "governed",
				QualityLabel:     "0 anomalies",
				Metrics:          memoryhealth.Metrics{ObservationCount: 3},
				Checks: []memoryhealth.Check{{
					ID:       memoryhealth.CheckConfigEnabled,
					Severity: memoryhealth.SeverityPass,
					Status:   "enabled",
					Message:  "Memory is enabled",
				}},
			},
		},
	})
	found := false
	for _, c := range report.Checks {
		if c.Name == "memory" {
			found = true
			if c.Severity != SeverityPass {
				t.Fatalf("severity=%s msg=%s", c.Severity, c.Message)
			}
			if !strings.Contains(c.Message, "healthy") {
				t.Fatalf("message=%s", c.Message)
			}
		}
		if strings.HasPrefix(c.Name, "memory ") && c.Severity == SeverityFail {
			t.Fatalf("unexpected fail %s: %s", c.Name, c.Message)
		}
	}
	if !found {
		t.Fatal("memory summary check missing")
	}
}

func TestEvaluate_MemoryFailedMapsToFail(t *testing.T) {
	report := Evaluate(inspect.Inspection{
		Runtime: runtime.Health{
			Memory: memoryhealth.Diagnostics{
				Status:       memoryhealth.StatusFailed,
				StorageLabel: "symlink",
				SchemaLabel:  "unchecked",
				Checks: []memoryhealth.Check{{
					ID:       memoryhealth.CheckStorageSymlinkSafe,
					Severity: memoryhealth.SeverityFail,
					Status:   "symlink",
					Message:  "Memory database path is a symlink",
				}},
			},
		},
	})
	var summary *Check
	for i := range report.Checks {
		if report.Checks[i].Name == "memory" {
			summary = &report.Checks[i]
			break
		}
	}
	if summary == nil || summary.Severity != SeverityFail {
		t.Fatalf("summary=%v", summary)
	}
}

func TestEvaluate_MemoryUnavailableMapsToInfo(t *testing.T) {
	report := Evaluate(inspect.Inspection{
		Runtime: runtime.Health{
			Memory: memoryhealth.Diagnostics{
				Status:       memoryhealth.StatusUnavailable,
				StorageLabel: "missing",
				Enabled:      true,
				Checks: []memoryhealth.Check{{
					ID:       memoryhealth.CheckStorageExists,
					Severity: memoryhealth.SeverityInfo,
					Status:   "missing",
					Message:  "Memory database is not present yet",
				}},
			},
		},
	})
	for _, c := range report.Checks {
		if c.Name == "memory" {
			if c.Severity != SeverityInfo {
				t.Fatalf("severity=%s", c.Severity)
			}
			return
		}
	}
	t.Fatal("missing memory summary")
}
