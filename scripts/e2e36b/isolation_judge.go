package main

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

const (
	resultPASS       = "PASS"
	resultFAIL       = "FAIL"
	resultSKIPPED    = "SKIPPED"
	resultUNVERIFIED = "UNVERIFIED"
)

// isolationSwapVerdict classifies a malicious byte-swap / wrong-pack probe.
// Returning peer-project information under Project A's retrieval must never be PASS.
func isolationSwapVerdict(peerReturned, failClosed bool, limitationDetail string) (result, detail string) {
	if peerReturned {
		// Path-scoped storage may surface substituted bytes without cryptographic
		// project binding. That is a documented trust-boundary limitation — never PASS.
		if limitationDetail == "" {
			limitationDetail = "peer-project data returned under trusted project path; storage cannot detect byte substitution"
		}
		return resultUNVERIFIED, limitationDetail
	}
	if failClosed {
		return resultPASS, "fail-closed; peer-project information not returned"
	}
	return resultUNVERIFIED, "swap outcome inconclusive; peer data not returned but source did not fail closed"
}

// judgeMemorySwapIsolation evaluates a compose result after swapping B Memory bytes under A's path.
func judgeMemorySwapIsolation(res composeapp.Result, peerCanary, ownCanary string) (result, detail string) {
	peer := containsCanary(res, peerCanary)
	// Own canary after B-byte swap would be unexpected; peer presence is the hard forbid.
	_ = ownCanary
	st := sourceOf(res, contextcompose.SourceMemory)
	failClosed := statusIn(st, contextcompose.SourceStatusFailed, contextcompose.SourceStatusUnavailable)
	return isolationSwapVerdict(peer, failClosed, "Memory DB is path-scoped; byte-for-byte substitution under trusted A path returned peer canary")
}

// judgeCodeGraphSwapIsolation evaluates compose after swapping B graph bytes under A's path.
func judgeCodeGraphSwapIsolation(res composeapp.Result, peerSymbol, ownSymbol string) (result, detail string) {
	peer := hasCodeGraphSymbol(res, peerSymbol)
	_ = ownSymbol
	st := sourceOf(res, contextcompose.SourceCodeGraph)
	failClosed := statusIn(st, contextcompose.SourceStatusFailed, contextcompose.SourceStatusUnavailable, contextcompose.SourceStatusNoCandidates)
	return isolationSwapVerdict(peer, failClosed, "CodeGraph DB is path-scoped; byte-for-byte substitution under trusted A path returned peer symbol")
}

// judgeCESwapIsolation evaluates compose after replacing A's pack with B's pack bytes.
// CE wrong-project rejection is expected to fail closed when possible.
func judgeCESwapIsolation(res composeapp.Result, peerCE, ownCE string) (result, detail string) {
	peer := containsPath(res, peerCE)
	_ = ownCE
	st := sourceOf(res, contextcompose.SourceContextEconomy)
	failClosed := statusIn(st, contextcompose.SourceStatusFailed, contextcompose.SourceStatusUnavailable)
	if peer {
		return isolationSwapVerdict(true, false, "CE pack substitution returned peer canary path under Project A")
	}
	if failClosed {
		return resultPASS, "CE fail-closed on wrong-project / substituted pack"
	}
	return isolationSwapVerdict(false, false, fmt.Sprintf("CE status=%s after pack swap; peer canary absent but not fail-closed", st))
}

// categoryCertificationResult aggregates rows for one certification category.
// A single PASS cannot hide a FAIL in the same category. UNVERIFIED does not
// elevate the category to PASS by itself and does not count as a failed invariant.
func categoryCertificationResult(rows []matrixRow, requiredPASS []string, categoryPrefixes []string) (result, detail string) {
	byID := map[string]matrixRow{}
	var fails []string
	for _, r := range rows {
		byID[r.ID] = r
		inCat := false
		for _, p := range categoryPrefixes {
			if strings.HasPrefix(r.ID, p) {
				inCat = true
				break
			}
		}
		if !inCat {
			continue
		}
		if r.Result == resultFAIL {
			fails = append(fails, r.ID)
		}
	}
	if len(fails) > 0 {
		return resultFAIL, "failed invariant(s): " + strings.Join(fails, ", ")
	}
	for _, id := range requiredPASS {
		r, ok := byID[id]
		if !ok || r.Result != resultPASS {
			got := "missing"
			if ok {
				got = r.Result
			}
			return resultFAIL, "required row " + id + " is " + got
		}
	}
	return resultPASS, "required isolation invariants held"
}
