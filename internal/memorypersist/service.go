package memorypersist

import (
	"strings"
	"sync"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
)

// projectWriteGates serialize equivalent same-process writes per Home+project.
// Cross-process contention still relies on SQLite busy_timeout.
var projectWriteGates sync.Map // key → *sync.Mutex

func lockProjectWrite(homePath, projectID string) func() {
	key := strings.TrimSpace(homePath) + "\x00" + strings.TrimSpace(projectID)
	v, _ := projectWriteGates.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Service is the single governed write authority for automatic Memory persistence.
type Service struct {
	Env Env
	// Evaluate overrides policy.Evaluate in tests.
	Evaluate func(policy.Candidate) policy.Decision
	// OpenStore opens a writable Memory store. Overridable in tests.
	OpenStore func(homePath, projectID string) (*memory.Store, error)
}

// Propose evaluates a candidate and, when accepted, persists through memory.Store.
func (s Service) Propose(in Candidate) Response {
	ref, err := s.Env.resolveProject()
	if err != nil {
		status, msg := classify(err)
		resp := errorResponse(status, msg)
		return resp
	}

	if ref.Mode == config.MemoryPersistenceOff {
		resp := errorResponse(StatusDisabled, "memory persistence is disabled")
		resp.ProjectID = ref.ProjectID
		return resp
	}

	eval := s.Evaluate
	if eval == nil {
		eval = policy.Evaluate
	}
	decision := eval(toPolicyCandidate(in))

	switch decision.Action {
	case policy.ActionReject:
		return Response{
			SchemaVersion: SchemaVersion,
			Command:       CommandPropose,
			Status:        StatusRejected,
			ProjectID:     ref.ProjectID,
			Decision:      &decision,
			Action:        "none",
		}
	case policy.ActionRequireConfirmation:
		return Response{
			SchemaVersion: SchemaVersion,
			Command:       CommandPropose,
			Status:        StatusConfirmationRequired,
			ProjectID:     ref.ProjectID,
			Decision:      &decision,
			Action:        "none",
		}
	case policy.ActionAccept:
		// continue to write
	default:
		return Response{
			SchemaVersion: SchemaVersion,
			Command:       CommandPropose,
			Status:        StatusRejected,
			ProjectID:     ref.ProjectID,
			Decision:      &decision,
			Action:        "none",
		}
	}

	unlock := lockProjectWrite(ref.Home, ref.ProjectID)
	defer unlock()

	open := s.OpenStore
	if open == nil {
		open = memory.Open
	}

	sk, sr := buildProvenance(in)
	scope := memory.Scope(strings.TrimSpace(in.Scope))
	if scope == "" {
		scope = memory.ScopeProject
	}
	obs := memory.Observation{
		Type:       memory.ObservationType(strings.TrimSpace(in.Type)),
		Scope:      scope,
		Title:      strings.TrimSpace(in.Title),
		Content:    strings.TrimSpace(in.Content),
		TopicKey:   strings.TrimSpace(in.TopicKey),
		SourceKind: sk,
		SourceRef:  sr,
	}

	var last error
	for attempt := 0; attempt < 4; attempt++ {
		store, err := open(ref.Home, ref.ProjectID)
		if err != nil {
			last = err
			if isTransientPersistError(err) && attempt < 3 {
				time.Sleep(time.Duration(15*(attempt+1)) * time.Millisecond)
				continue
			}
			status, msg := classify(err)
			resp := errorResponse(status, msg)
			resp.ProjectID = ref.ProjectID
			resp.Decision = &decision
			return resp
		}

		result, err := store.SaveObservation(obs)
		_ = store.Close()
		if err != nil {
			last = err
			if isTransientPersistError(err) && attempt < 3 {
				time.Sleep(time.Duration(15*(attempt+1)) * time.Millisecond)
				continue
			}
			status, msg := classify(err)
			if status == StatusInternalError && isValidationError(err) {
				status = StatusInvalidInput
				msg = "candidate failed memory validation"
			}
			resp := errorResponse(status, msg)
			resp.ProjectID = ref.ProjectID
			resp.Decision = &decision
			return resp
		}

		dto := toDTO(result.Observation)
		status := StatusAccepted
		action := string(result.Action)
		switch result.Action {
		case memory.SaveDuplicate:
			status = StatusDuplicate
		case memory.SaveUpdated:
			status = StatusUpdated
		case memory.SaveCreated:
			status = StatusAccepted
			action = "created"
		}
		return Response{
			SchemaVersion: SchemaVersion,
			Command:       CommandPropose,
			Status:        status,
			ProjectID:     ref.ProjectID,
			Decision:      &decision,
			Action:        action,
			Observation:   &dto,
		}
	}

	status, msg := classify(last)
	resp := errorResponse(status, msg)
	resp.ProjectID = ref.ProjectID
	resp.Decision = &decision
	return resp
}

func isTransientPersistError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "file exists") ||
		strings.Contains(msg, "busy")
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "memory: invalid") ||
		strings.Contains(msg, "memory: title") ||
		strings.Contains(msg, "memory: content") ||
		strings.Contains(msg, "memory: topic") ||
		strings.Contains(msg, "memory: source") ||
		strings.Contains(msg, "memory: project id") ||
		strings.Contains(msg, "memory: session")
}
