package hookproj

import (
	"crypto/sha256"
	"encoding/hex"
)

// DigestBytes returns a stable sha256 hex digest of raw bytes.
func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
