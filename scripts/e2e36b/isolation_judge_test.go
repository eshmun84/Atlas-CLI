package main

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

func TestJudgeMemorySwap_PeerCanaryNeverPASS(t *testing.T) {
	res := composeapp.Result{}
	res.Sources = []contextcompose.SourceStatus{{
		Source: contextcompose.SourceMemory,
		Status: contextcompose.SourceStatusSupplied,
	}}
	res.Items = []contextcompose.Item{{
		Source:  contextcompose.SourceMemory,
		Title:   "Architectural decision " + canaryMemB,
		Excerpt: "Token " + canaryMemB,
	}}
	got, detail := judgeMemorySwapIsolation(res, canaryMemB, canaryMemA)
	if got == resultPASS {
		t.Fatalf("peer Memory canary must never be PASS; got PASS (%s)", detail)
	}
	if got != resultUNVERIFIED {
		t.Fatalf("got %s want UNVERIFIED; detail=%s", got, detail)
	}
}

func TestJudgeCodeGraphSwap_PeerSymbolNeverPASS(t *testing.T) {
	res := composeapp.Result{}
	res.Sources = []contextcompose.SourceStatus{{
		Source: contextcompose.SourceCodeGraph,
		Status: contextcompose.SourceStatusSupplied,
	}}
	res.Items = []contextcompose.Item{{
		Source: contextcompose.SourceCodeGraph,
		Title:  symbolB,
		Path:   "src/payments.go",
		Ref:    "src/payments.go:4",
	}}
	got, detail := judgeCodeGraphSwapIsolation(res, symbolB, symbolA)
	if got == resultPASS {
		t.Fatalf("peer CodeGraph symbol must never be PASS; got PASS (%s)", detail)
	}
	if got != resultUNVERIFIED {
		t.Fatalf("got %s want UNVERIFIED; detail=%s", got, detail)
	}
}

func TestJudgeCESwap_FailClosedPASS(t *testing.T) {
	res := composeapp.Result{}
	res.Sources = []contextcompose.SourceStatus{{
		Source: contextcompose.SourceContextEconomy,
		Status: contextcompose.SourceStatusFailed,
	}}
	got, detail := judgeCESwapIsolation(res, canaryCEB, canaryCEA)
	if got != resultPASS {
		t.Fatalf("got %s want PASS; detail=%s", got, detail)
	}
}

func TestJudgeCESwap_PeerPathNeverPASS(t *testing.T) {
	res := composeapp.Result{}
	res.Sources = []contextcompose.SourceStatus{{
		Source: contextcompose.SourceContextEconomy,
		Status: contextcompose.SourceStatusSupplied,
	}}
	res.Items = []contextcompose.Item{{
		Source: contextcompose.SourceContextEconomy,
		Path:   canaryCEB,
		Title:  canaryCEB,
	}}
	got, detail := judgeCESwapIsolation(res, canaryCEB, canaryCEA)
	if got == resultPASS {
		t.Fatalf("peer CE canary must never be PASS; detail=%s", detail)
	}
}

func TestJudgeMemorySwap_FailClosedPASS(t *testing.T) {
	res := composeapp.Result{}
	res.Sources = []contextcompose.SourceStatus{{
		Source: contextcompose.SourceMemory,
		Status: contextcompose.SourceStatusFailed,
	}}
	got, detail := judgeMemorySwapIsolation(res, canaryMemB, canaryMemA)
	if got != resultPASS {
		t.Fatalf("got %s want PASS; detail=%s", got, detail)
	}
}

func TestCategoryCertification_CannotHideFail(t *testing.T) {
	rows := []matrixRow{
		{ID: "isolation.ab", Result: resultPASS, Detail: "ok"},
		{ID: "isolation.memory_path_scoped", Result: resultFAIL, Detail: "leak"},
	}
	got, detail := categoryCertificationResult(rows, []string{"isolation.ab"}, []string{"isolation."})
	if got != resultFAIL {
		t.Fatalf("got %s want FAIL; detail=%s", got, detail)
	}
}

func TestCategoryCertification_UnverifiedDoesNotBlockRequiredPASS(t *testing.T) {
	rows := []matrixRow{
		{ID: "isolation.ab", Result: resultPASS, Detail: "ok"},
		{ID: "isolation.memory_path_scoped", Result: resultUNVERIFIED, Detail: "path-scoped limitation"},
		{ID: "isolation.graph_path_scoped", Result: resultUNVERIFIED, Detail: "path-scoped limitation"},
	}
	got, detail := categoryCertificationResult(rows, []string{"isolation.ab"}, []string{"isolation."})
	if got != resultPASS {
		t.Fatalf("got %s want PASS; detail=%s", got, detail)
	}
}

func TestCategoryCertification_MissingRequired(t *testing.T) {
	rows := []matrixRow{
		{ID: "isolation.memory_path_scoped", Result: resultPASS, Detail: "fail-closed"},
	}
	got, _ := categoryCertificationResult(rows, []string{"isolation.ab"}, []string{"isolation."})
	if got != resultFAIL {
		t.Fatalf("got %s want FAIL when required isolation.ab missing", got)
	}
}

func TestBuildCertificationMatrix_HidesNoFail(t *testing.T) {
	rows := []matrixRow{
		{ID: "hooks.projections", Result: resultPASS},
		{ID: "hooks.contract", Result: resultPASS},
		{ID: "memory.skill", Result: resultPASS},
		{ID: "compose.a", Result: resultPASS},
		{ID: "compose.b", Result: resultPASS},
		{ID: "compose.determinism", Result: resultPASS},
		{ID: "isolation.ab", Result: resultPASS},
		{ID: "isolation.memory_path_scoped", Result: resultUNVERIFIED},
		{ID: "partial.mem_unavailable", Result: resultPASS},
		{ID: "readonly.final", Result: resultPASS},
		{ID: "security.symlink_memory", Result: resultPASS},
		{ID: "security.missing_json", Result: resultFAIL, Detail: "boom"},
	}
	cert := buildCertificationMatrix(rows)
	byID := map[string]string{}
	for _, r := range cert {
		byID[r.ID] = r.Result
	}
	if byID["cert.project_isolation"] != resultPASS {
		t.Fatalf("isolation cert=%s want PASS", byID["cert.project_isolation"])
	}
	if byID["cert.security"] != resultFAIL {
		t.Fatalf("security cert=%s want FAIL (must not hide FAIL row)", byID["cert.security"])
	}
}
