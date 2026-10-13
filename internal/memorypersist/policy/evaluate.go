package policy

import (
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

// V1 size bounds (aligned with memory.ValidateObservation where applicable).
const (
	MaxTitleLen       = 512
	MaxContentLen     = 64 * 1024
	MaxTopicLen       = 256
	MaxProvenanceLen  = 512
	MaxSourceEventLen = 256
	MinTitleRunes     = 8
	MinContentRunes   = 24
)

// MaxTopicLen is the V1 topic_key byte bound (exported for transport sizing).

var allowedTypes = map[string]struct{}{
	string(memory.TypeDecision):     {},
	string(memory.TypeArchitecture): {},
	string(memory.TypeConstraint):   {},
	string(memory.TypeDiscovery):    {},
	string(memory.TypeBugfix):       {},
	string(memory.TypePattern):      {},
	string(memory.TypeConfig):       {},
	string(memory.TypePreference):   {},
	string(memory.TypeFact):         {},
}

var allowedOrigins = map[Origin]struct{}{
	OriginAtlasLifecycle:        {},
	OriginDeveloperConfirmation: {},
	OriginAtlasOperation:        {},
}

var transientMarkers = []string{
	"tmp/", "temp/", "/tmp", "temporary", "scratch",
	"debug only", "debugging", "wip:", "todo:", "fixme",
	"one-off", "one off", "transient", "ephemeral",
	"guess:", "speculation:", "maybe:", "probably:",
	"conversation transcript", "chat log", "raw output",
}

// Evaluate applies deterministic V1 quality gates. Pure: no I/O.
func Evaluate(c Candidate) Decision {
	var reasons []Reason

	title := strings.TrimSpace(c.Title)
	content := strings.TrimSpace(c.Content)
	scope := strings.TrimSpace(c.Scope)
	if scope == "" {
		scope = string(memory.ScopeProject)
	}
	topic := strings.TrimSpace(c.TopicKey)
	prov := strings.TrimSpace(c.Provenance)
	event := strings.TrimSpace(c.SourceEvent)
	typ := strings.TrimSpace(c.Type)
	explicitness := c.Explicitness
	if explicitness == "" {
		explicitness = ExplicitnessInferred
	}
	confidence := c.Confidence
	if confidence == "" {
		confidence = ConfidenceMedium
	}

	// Input / UTF-8 / size
	if !utf8.ValidString(title) || !utf8.ValidString(content) || !utf8.ValidString(topic) || !utf8.ValidString(prov) {
		return reject(Reason{Gate: GateInput, Code: "invalid_utf8", Message: "candidate contains invalid UTF-8"})
	}
	if title == "" || content == "" {
		return reject(Reason{Gate: GateInput, Code: "required_fields", Message: "title and content are required"})
	}
	if len(title) > MaxTitleLen {
		return reject(Reason{Gate: GateSize, Code: "title_too_long", Message: "title exceeds size bound"})
	}
	if len(content) > MaxContentLen {
		return reject(Reason{Gate: GateSize, Code: "content_too_long", Message: "content exceeds size bound"})
	}
	if len(topic) > MaxTopicLen {
		return reject(Reason{Gate: GateSize, Code: "topic_too_long", Message: "topic key exceeds size bound"})
	}
	if len(prov) > MaxProvenanceLen {
		return reject(Reason{Gate: GateSize, Code: "provenance_too_long", Message: "provenance exceeds size bound"})
	}
	if len(event) > MaxSourceEventLen {
		return reject(Reason{Gate: GateSize, Code: "source_event_too_long", Message: "source event exceeds size bound"})
	}
	if looksLikePathTraversal(prov) || looksLikePathTraversal(event) || looksLikeAbsoluteHome(prov) || looksLikeAbsoluteHome(event) {
		return reject(Reason{Gate: GateInput, Code: "unsafe_provenance", Message: "provenance/source event looks unsafe"})
	}

	// Type
	if _, ok := allowedTypes[typ]; !ok {
		return reject(Reason{Gate: GateType, Code: "invalid_type", Message: "observation type is not in the V1 taxonomy"})
	}

	// Scope — project only in V1
	if scope != string(memory.ScopeProject) {
		return reject(Reason{Gate: GateScope, Code: "invalid_scope", Message: "only project scope is allowed in V1"})
	}

	// Origin
	if _, ok := allowedOrigins[c.Origin]; !ok {
		return reject(Reason{Gate: GateOrigin, Code: "invalid_origin", Message: "candidate origin is not an allowed automatic source"})
	}

	// Sensitivity — fail closed
	combined := title + "\n" + content + "\n" + topic + "\n" + prov + "\n" + event
	if reason, hit := sensitivityReject(combined); hit {
		return reject(reason)
	}

	// Durability / transient
	lower := strings.ToLower(title + " " + content)
	for _, marker := range transientMarkers {
		if strings.Contains(lower, marker) {
			return reject(Reason{
				Gate:    GateDurability,
				Code:    "transient_marker",
				Message: "candidate looks transient rather than durable project knowledge",
			})
		}
	}

	// Specificity
	if utf8.RuneCountInString(title) < MinTitleRunes {
		return reject(Reason{Gate: GateSpecificity, Code: "title_too_short", Message: "title is not specific enough"})
	}
	if utf8.RuneCountInString(content) < MinContentRunes {
		return reject(Reason{Gate: GateSpecificity, Code: "content_too_short", Message: "content is not specific enough"})
	}
	if isVague(title, content) {
		return reject(Reason{Gate: GateSpecificity, Code: "vague", Message: "candidate is too vague to be useful later"})
	}

	// Stability — speculative / low confidence fail closed
	switch explicitness {
	case ExplicitnessSpeculative:
		return reject(Reason{Gate: GateStability, Code: "speculative", Message: "speculative candidates must not become durable Memory"})
	case ExplicitnessExplicit, ExplicitnessConfirmed, ExplicitnessInferred:
	default:
		return reject(Reason{Gate: GateEvidence, Code: "invalid_explicitness", Message: "explicitness value is invalid"})
	}
	if confidence == ConfidenceLow && explicitness != ExplicitnessExplicit {
		return reject(Reason{Gate: GateEvidence, Code: "low_confidence", Message: "low-confidence inference is rejected"})
	}
	if confidence != ConfidenceHigh && confidence != ConfidenceMedium && confidence != ConfidenceLow {
		return reject(Reason{Gate: GateEvidence, Code: "invalid_confidence", Message: "confidence value is invalid"})
	}

	// Evidence / explicitness promotion rules.
	// The propose transport is untrusted: neither inferred nor confirmed may
	// accept. Origin (including developer_confirmation) is also caller-supplied
	// and is not trust proof. Only explicitness=explicit may accept in V1.
	switch explicitness {
	case ExplicitnessInferred:
		reasons = append(reasons, Reason{
			Gate:    GateEvidence,
			Code:    "inferred_needs_confirmation",
			Message: "inferred decisions require a trusted confirmation surface before persistence (deferred in V1)",
		})
		return Decision{Action: ActionRequireConfirmation, Reasons: reasons}
	case ExplicitnessConfirmed:
		reasons = append(reasons, Reason{
			Gate:    GateEvidence,
			Code:    "confirmed_reserved",
			Message: "explicitness confirmed is reserved until a trusted confirmation surface exists",
		})
		return Decision{Action: ActionRequireConfirmation, Reasons: reasons}
	case ExplicitnessExplicit:
		reasons = append(reasons, Reason{
			Gate:    GateEvidence,
			Code:    "explicit_statement",
			Message: "candidate is an explicit durable statement",
		})
	}

	reasons = append(reasons,
		Reason{Gate: GateDurability, Code: "durable", Message: "candidate appears relevant beyond the current session"},
		Reason{Gate: GateScope, Code: "project", Message: "scope is project"},
		Reason{Gate: GateOrigin, Code: "allowed_origin", Message: "origin is an approved automatic source"},
	)
	return Decision{Action: ActionAccept, Reasons: reasons}
}

func reject(r Reason) Decision {
	return Decision{Action: ActionReject, Reasons: []Reason{r}}
}

func looksLikePathTraversal(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "..") || strings.Contains(s, `\`) {
		return true
	}
	return false
}

func looksLikeAbsoluteHome(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(s, "/") || strings.Contains(s, ":\\") {
		return true
	}
	if strings.Contains(lower, "/.atlas/") || strings.Contains(lower, "atlas_home") || strings.Contains(lower, "$atlas_home") {
		return true
	}
	return false
}

func isVague(title, content string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	c := strings.ToLower(strings.TrimSpace(content))
	vagueTitles := []string{"note", "update", "change", "thing", "stuff", "misc", "random", "test", "ok", "done"}
	for _, v := range vagueTitles {
		if t == v {
			return true
		}
	}
	vagueBodies := []string{"see above", "as discussed", "whatever works", "just a note", "n/a", "tbd"}
	for _, v := range vagueBodies {
		if c == v {
			return true
		}
	}
	return false
}
