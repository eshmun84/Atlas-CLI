package hookactions

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const admissionIDBytes = 16

func newAdmissionID() (string, error) {
	var raw [admissionIDBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("hookactions: generate admission id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func validAdmissionID(raw string) bool {
	if len(raw) != admissionIDBytes*2 {
		return false
	}
	for _, c := range raw {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func validInternalProjectID(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 128 || strings.Contains(raw, "..") {
		return false
	}
	for _, c := range raw {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
