package memory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("memory: generate id: %w", err)
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func newObservationID() (string, error) { return newID("obs_") }
func newSessionID() (string, error)     { return newID("ses_") }
