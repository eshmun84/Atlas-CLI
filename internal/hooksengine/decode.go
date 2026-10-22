package hooksengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// DecodeRequest reads a bounded JSON request from r.
// Unknown fields are rejected. Trailing junk after one JSON value is rejected.
func DecodeRequest(r io.Reader) (Request, error) {
	if r == nil {
		return Request{}, fmt.Errorf("%w: empty body", ErrInvalidRequest)
	}
	limited := &io.LimitedReader{R: r, N: MaxRequestBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return Request{}, fmt.Errorf("%w: read: %v", ErrInvalidRequest, err)
	}
	if len(raw) == 0 {
		return Request{}, fmt.Errorf("%w: empty body", ErrInvalidRequest)
	}
	if len(raw) > MaxRequestBytes {
		return Request{}, fmt.Errorf("%w: request exceeds %d bytes", ErrInvalidRequest, MaxRequestBytes)
	}
	if !utf8.Valid(raw) {
		return Request{}, fmt.Errorf("%w: body is not valid UTF-8", ErrInvalidRequest)
	}
	for _, b := range raw {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
			return Request{}, fmt.Errorf("%w: body contains control characters", ErrInvalidRequest)
		}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var req Request
	if err := dec.Decode(&req); err != nil {
		return Request{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	// dec.More() misses concatenated top-level JSON values; require EOF.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return Request{}, fmt.Errorf("%w: trailing data after JSON value", ErrInvalidRequest)
	}
	if err := validateSchemaVersion(req.SchemaVersion); err != nil {
		return Request{}, err
	}
	return req, nil
}

func validateSchemaVersion(v int) error {
	if v != SchemaVersion {
		return fmt.Errorf("%w: schema_version must be %d", ErrInvalidRequest, SchemaVersion)
	}
	return nil
}

// DecodeRequestBytes is DecodeRequest for an in-memory buffer.
func DecodeRequestBytes(raw []byte) (Request, error) {
	return DecodeRequest(bytes.NewReader(raw))
}

func trimRequest(req *Request) {
	req.Adapter = strings.TrimSpace(req.Adapter)
	req.Event.ID = strings.TrimSpace(req.Event.ID)
	req.Event.Type = strings.TrimSpace(req.Event.Type)
	req.Event.OccurredAt = strings.TrimSpace(req.Event.OccurredAt)
	p := &req.Event.Payload
	p.ProjectID = strings.TrimSpace(p.ProjectID)
	p.SessionDirectory = strings.TrimSpace(p.SessionDirectory)
	p.SessionID = strings.TrimSpace(p.SessionID)
	p.TaskID = strings.TrimSpace(p.TaskID)
	p.ToolName = strings.TrimSpace(p.ToolName)
	p.ToolCallID = strings.TrimSpace(p.ToolCallID)
	p.Status = strings.TrimSpace(p.Status)
	p.Reason = strings.TrimSpace(p.Reason)
	p.TaskSummary = strings.TrimSpace(p.TaskSummary)
	p.TaskSummaryState = strings.TrimSpace(p.TaskSummaryState)
}
