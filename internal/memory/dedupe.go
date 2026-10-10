package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// durableHash computes a deterministic content hash over durable fields only.
// Timestamps and IDs are excluded.
func durableHash(projectID string, scope Scope, typ ObservationType, title, content, topicKey, sourceKind, sourceRef string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(projectID))
	b.WriteByte(0)
	b.WriteString(string(scope))
	b.WriteByte(0)
	b.WriteString(string(typ))
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(title))
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(content))
	b.WriteByte(0)
	b.WriteString(topicKey)
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(sourceKind))
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(sourceRef))
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
