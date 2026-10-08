package codegraph_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
)

func TestParseVersionAndCompatibility(t *testing.T) {
	t.Parallel()
	v, err := codegraph.ParseVersion("3.17.0")
	if err != nil || v != "3.17.0" {
		t.Fatalf("parse=%q err=%v", v, err)
	}
	if !codegraph.IsCompatible("3.17.0") {
		t.Fatal("3.17.0 should be compatible with major-3 baseline")
	}
	if codegraph.IsCompatible("2.9.0") {
		t.Fatal("2.x must be incompatible")
	}
	if codegraph.IsCompatible("4.0.0") {
		t.Fatal("4.x must be incompatible until Atlas widens the gate")
	}
	if codegraph.IsCompatible("0.1.0") {
		t.Fatal("0.1.0 must not be treated as compatible")
	}
}

func TestV3ContractIsNotRuntimeProbedList(t *testing.T) {
	t.Parallel()
	if len(codegraph.V3Contract) == 0 {
		t.Fatal("V3Contract should document expected major-3 surface")
	}
	for _, feature := range codegraph.V3Contract {
		if feature == codegraph.ProbedCapabilityVersion {
			t.Fatal("V3Contract must stay separate from Probe capability tokens")
		}
	}
}
