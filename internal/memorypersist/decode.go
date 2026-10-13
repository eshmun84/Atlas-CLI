package memorypersist

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
)

// MaxCandidateJSONBytes bounds stdin for atlas memory propose --json.
//
// Justification (V1 field maxima from policy + JSON framing):
//
//	title        ≤ 512
//	content      ≤ 64 KiB
//	topic_key    ≤ 256
//	provenance   ≤ 512
//	source_event ≤ 256
//	enum fields  ≤ ~320 combined
//	JSON keys/structure/slack ≤ 8 KiB
//
// Sum ≈ 74 KiB; rounded to 96 KiB for encoding headroom. The reader stops at
// max+1 bytes so oversized payloads are detected without Content-Length.
const MaxCandidateJSONBytes = policy.MaxContentLen + // 64 KiB
	policy.MaxTitleLen + // 512
	policy.MaxTopicLen + // 256
	policy.MaxProvenanceLen + // 512
	policy.MaxSourceEventLen + // 256
	8*1024 // enums + JSON keys + slack

var (
	// ErrOversizedCandidate reports a propose payload larger than MaxCandidateJSONBytes.
	ErrOversizedCandidate = errors.New("memorypersist: candidate JSON exceeds size bound")
	// ErrEmptyCandidate reports empty or whitespace-only stdin.
	ErrEmptyCandidate = errors.New("memorypersist: candidate JSON is empty")
	// ErrMalformedCandidate reports undecodable or multi-value JSON (no payload echo).
	ErrMalformedCandidate = errors.New("memorypersist: malformed candidate JSON")
	// ErrTrailingCandidate reports trailing tokens after the first JSON value.
	ErrTrailingCandidate = errors.New("memorypersist: trailing data after candidate JSON")
)

// DecodeCandidate reads exactly one Candidate JSON object from r.
//
// Contract:
//   - reads at most MaxCandidateJSONBytes+1 bytes
//   - rejects oversized / empty / unknown fields / trailing values or garbage
//   - never returns the raw payload in error text
func DecodeCandidate(r io.Reader) (Candidate, error) {
	if r == nil {
		return Candidate{}, ErrEmptyCandidate
	}
	limited := io.LimitReader(r, int64(MaxCandidateJSONBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return Candidate{}, ErrMalformedCandidate
	}
	if len(data) > MaxCandidateJSONBytes {
		return Candidate{}, ErrOversizedCandidate
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Candidate{}, ErrEmptyCandidate
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Candidate
	if err := dec.Decode(&c); err != nil {
		return Candidate{}, ErrMalformedCandidate
	}

	// Require exactly one JSON value: second Decode must be EOF
	// (whitespace-only remainder is OK; another value or garbage is not).
	var extra json.RawMessage
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return c, nil
	case err == nil:
		return Candidate{}, ErrTrailingCandidate
	default:
		return Candidate{}, ErrTrailingCandidate
	}
}

// ClassifyDecodeError maps decode sentinels to CLI status + safe message.
func ClassifyDecodeError(err error) (status, message string) {
	switch {
	case errors.Is(err, ErrOversizedCandidate):
		return StatusInvalidInput, "candidate JSON exceeds size bound"
	case errors.Is(err, ErrEmptyCandidate):
		return StatusInvalidInput, "candidate JSON is required"
	case errors.Is(err, ErrTrailingCandidate):
		return StatusInvalidInput, "candidate JSON must be a single value"
	case errors.Is(err, ErrMalformedCandidate):
		return StatusInvalidInput, "malformed candidate JSON"
	default:
		return StatusInvalidInput, "malformed candidate JSON"
	}
}
