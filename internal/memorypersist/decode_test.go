package memorypersist_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
)

func validCandidateJSON(t *testing.T) []byte {
	t.Helper()
	c := memorypersist.Candidate{
		Type:         string(memory.TypeDecision),
		Title:        "Prefer SQLite Memory Core",
		Content:      "Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.",
		Scope:        string(memory.ScopeProject),
		TopicKey:     "memory.storage.engine",
		Origin:       policy.OriginAtlasLifecycle,
		Explicitness: policy.ExplicitnessExplicit,
		Confidence:   policy.ConfidenceHigh,
		Provenance:   "cap=decode-test",
		SourceEvent:  "decode.valid",
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeCandidate_Valid(t *testing.T) {
	t.Parallel()
	c, err := memorypersist.DecodeCandidate(bytes.NewReader(validCandidateJSON(t)))
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Prefer SQLite Memory Core" || c.Origin != policy.OriginAtlasLifecycle {
		t.Fatalf("%#v", c)
	}
}

func TestDecodeCandidate_ExactlyAtLimit(t *testing.T) {
	t.Parallel()
	base := map[string]any{
		"type":         "decision",
		"title":        "Prefer SQLite Memory Core",
		"scope":        "project",
		"origin":       "atlas_lifecycle",
		"explicitness": "explicit",
		"confidence":   "high",
	}
	// Grow content until marshaled size == MaxCandidateJSONBytes.
	content := strings.Repeat("a", 1024)
	var payload []byte
	for {
		base["content"] = content + " Atlas stores durable project Memory in SQLite under Atlas Home."
		var err error
		payload, err = json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) == memorypersist.MaxCandidateJSONBytes {
			break
		}
		if len(payload) > memorypersist.MaxCandidateJSONBytes {
			// Trim content to land exactly on the bound.
			over := len(payload) - memorypersist.MaxCandidateJSONBytes
			if over >= len(content) {
				t.Fatalf("cannot trim to exact bound: payload=%d max=%d", len(payload), memorypersist.MaxCandidateJSONBytes)
			}
			content = content[:len(content)-over]
			base["content"] = content + " Atlas stores durable project Memory in SQLite under Atlas Home."
			payload, err = json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			// Adjust by single bytes if JSON length shifted.
			for len(payload) < memorypersist.MaxCandidateJSONBytes {
				content += "x"
				base["content"] = content + " Atlas stores durable project Memory in SQLite under Atlas Home."
				payload, err = json.Marshal(base)
				if err != nil {
					t.Fatal(err)
				}
			}
			for len(payload) > memorypersist.MaxCandidateJSONBytes && len(content) > 0 {
				content = content[:len(content)-1]
				base["content"] = content + " Atlas stores durable project Memory in SQLite under Atlas Home."
				payload, err = json.Marshal(base)
				if err != nil {
					t.Fatal(err)
				}
			}
			break
		}
		content += strings.Repeat("b", 2048)
	}
	if len(payload) != memorypersist.MaxCandidateJSONBytes {
		t.Fatalf("setup len=%d want %d", len(payload), memorypersist.MaxCandidateJSONBytes)
	}
	_, err := memorypersist.DecodeCandidate(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("exact limit must decode: %v", err)
	}
}

func TestDecodeCandidate_OverLimit(t *testing.T) {
	t.Parallel()
	payload := bytes.Repeat([]byte("x"), memorypersist.MaxCandidateJSONBytes+1)
	_, err := memorypersist.DecodeCandidate(bytes.NewReader(payload))
	if !errors.Is(err, memorypersist.ErrOversizedCandidate) {
		t.Fatalf("err=%v", err)
	}
	status, msg := memorypersist.ClassifyDecodeError(err)
	if status != memorypersist.StatusInvalidInput || msg == "" {
		t.Fatalf("status=%q msg=%q", status, msg)
	}
	if strings.Contains(msg, string(payload[:32])) {
		t.Fatal("must not echo payload")
	}
}

func TestDecodeCandidate_TwoConcatenatedObjects(t *testing.T) {
	t.Parallel()
	one := validCandidateJSON(t)
	payload := append(append([]byte{}, one...), one...)
	_, err := memorypersist.DecodeCandidate(bytes.NewReader(payload))
	if !errors.Is(err, memorypersist.ErrTrailingCandidate) {
		t.Fatalf("err=%v", err)
	}
}

func TestDecodeCandidate_TrailingGarbage(t *testing.T) {
	t.Parallel()
	payload := append(validCandidateJSON(t), []byte(" trailing-garbage")...)
	_, err := memorypersist.DecodeCandidate(bytes.NewReader(payload))
	if !errors.Is(err, memorypersist.ErrTrailingCandidate) {
		t.Fatalf("err=%v", err)
	}
}

func TestDecodeCandidate_UnknownField(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"type":"decision",
		"title":"Prefer SQLite Memory Core",
		"content":"Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.",
		"origin":"atlas_lifecycle",
		"explicitness":"explicit",
		"confirm":true
	}`)
	_, err := memorypersist.DecodeCandidate(bytes.NewReader(payload))
	if !errors.Is(err, memorypersist.ErrMalformedCandidate) {
		t.Fatalf("confirm/unknown must fail closed: %v", err)
	}
}

func TestDecodeCandidate_EmptyAndWhitespace(t *testing.T) {
	t.Parallel()
	for _, in := range []io.Reader{
		bytes.NewReader(nil),
		strings.NewReader(""),
		strings.NewReader("   \n\t  "),
	} {
		_, err := memorypersist.DecodeCandidate(in)
		if !errors.Is(err, memorypersist.ErrEmptyCandidate) {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestDecodeCandidate_StopsAtMaxPlusOne(t *testing.T) {
	t.Parallel()
	// Reader that would hang/block if decoder tried to read unbounded input.
	r := &countingReader{data: bytes.Repeat([]byte("y"), memorypersist.MaxCandidateJSONBytes+4096)}
	_, err := memorypersist.DecodeCandidate(r)
	if !errors.Is(err, memorypersist.ErrOversizedCandidate) {
		t.Fatalf("err=%v", err)
	}
	if r.n > memorypersist.MaxCandidateJSONBytes+1 {
		t.Fatalf("read %d bytes; bound is max+1=%d", r.n, memorypersist.MaxCandidateJSONBytes+1)
	}
}

type countingReader struct {
	data []byte
	off  int
	n    int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	r.n += n
	return n, nil
}
