package policy_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
)

func durableCandidate() policy.Candidate {
	return policy.Candidate{
		Type:         string(memory.TypeDecision),
		Title:        "Use SQLite for project Memory",
		Content:      "Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.",
		Scope:        string(memory.ScopeProject),
		TopicKey:     "memory.storage",
		Provenance:   "cap=memory-persist-36c;ref=lifecycle-decision",
		Confidence:   policy.ConfidenceHigh,
		Explicitness: policy.ExplicitnessExplicit,
		Origin:       policy.OriginAtlasLifecycle,
		SourceEvent:  "lifecycle.decision.accepted",
	}
}

func TestEvaluate_AcceptedDurableDecision(t *testing.T) {
	t.Parallel()
	d := policy.Evaluate(durableCandidate())
	if d.Action != policy.ActionAccept {
		t.Fatalf("action=%s reasons=%+v", d.Action, d.Reasons)
	}
}

func TestEvaluate_TransientRejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Title = "Temporary debug path note"
	c.Content = "Use /tmp/scratch for this one-off debugging session only."
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject {
		t.Fatalf("action=%s", d.Action)
	}
	if d.Reasons[0].Gate != policy.GateDurability {
		t.Fatalf("gate=%s", d.Reasons[0].Gate)
	}
}

func TestEvaluate_SpeculativeRejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Explicitness = policy.ExplicitnessSpeculative
	c.Title = "Consider Postgres for Memory later"
	c.Content = "We might migrate Memory off SQLite if scale becomes an issue in future years."
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Code != "speculative" {
		t.Fatalf("got %+v", d)
	}
}

func TestEvaluate_SecretRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
	}{
		{"bearer", "Set Authorization: Bearer FAKESECRET_g1h2i3j4k5l6m7n8o9p0"},
		{"pem", "Key material:\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0Z3VS5JJcds3xfn\n-----END RSA PRIVATE KEY-----"},
		{"password", "DB password: hunter2secret"},
		{"env", "Export OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz0123456789"},
		{"github", "Use token ghp_abcdefghijklmnopqrstuvwxyz012345"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := durableCandidate()
			c.Content = tc.content
			d := policy.Evaluate(c)
			if d.Action != policy.ActionReject || d.Reasons[0].Gate != policy.GateSensitivity {
				t.Fatalf("got %+v", d)
			}
		})
	}
}

func TestEvaluate_OversizedRejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Content = strings.Repeat("x", policy.MaxContentLen+1)
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Gate != policy.GateSize {
		t.Fatalf("got %+v", d)
	}
}

func TestEvaluate_InvalidScopeRejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Scope = "global"
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Gate != policy.GateScope {
		t.Fatalf("got %+v", d)
	}
}

func TestEvaluate_LowEvidenceConservative(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Explicitness = policy.ExplicitnessInferred
	c.Confidence = policy.ConfidenceLow
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Code != "low_confidence" {
		t.Fatalf("got %+v", d)
	}
}

func TestEvaluate_InferredRequiresConfirmation(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Explicitness = policy.ExplicitnessInferred
	c.Confidence = policy.ConfidenceMedium
	d := policy.Evaluate(c)
	if d.Action != policy.ActionRequireConfirmation {
		t.Fatalf("got %+v", d)
	}
	if d.Reasons[0].Code != "inferred_needs_confirmation" {
		t.Fatalf("reasons=%+v", d.Reasons)
	}
}

func TestEvaluate_ConfirmedRequiresConfirmation(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Explicitness = policy.ExplicitnessConfirmed
	c.Confidence = policy.ConfidenceHigh
	d := policy.Evaluate(c)
	if d.Action != policy.ActionRequireConfirmation || d.Reasons[0].Code != "confirmed_reserved" {
		t.Fatalf("got %+v", d)
	}

	// Origin is untrusted — developer_confirmation must not unlock writes.
	c.Origin = policy.OriginDeveloperConfirmation
	d2 := policy.Evaluate(c)
	if d2.Action != policy.ActionRequireConfirmation {
		t.Fatalf("confirmed+developer_confirmation must not accept: %+v", d2)
	}
}

func TestEvaluate_ExplicitAccepted(t *testing.T) {
	t.Parallel()
	d := policy.Evaluate(durableCandidate())
	if d.Action != policy.ActionAccept {
		t.Fatalf("got %+v", d)
	}
	found := false
	for _, r := range d.Reasons {
		if r.Code == "explicit_statement" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reasons=%+v", d.Reasons)
	}
}

func TestEvaluate_InvalidOriginRejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Origin = "assistant_prose"
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Gate != policy.GateOrigin {
		t.Fatalf("got %+v", d)
	}
}

func TestEvaluate_UnsafeProvenanceRejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Provenance = "/Users/me/.atlas/projects/x/memory/memory.db"
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Code != "unsafe_provenance" {
		t.Fatalf("got %+v", d)
	}
}

func TestEvaluate_InvalidUTF8Rejected(t *testing.T) {
	t.Parallel()
	c := durableCandidate()
	c.Title = "Bad title \xff\xfe still long enough"
	d := policy.Evaluate(c)
	if d.Action != policy.ActionReject || d.Reasons[0].Code != "invalid_utf8" {
		t.Fatalf("got %+v", d)
	}
}
