package memoryhealth

import (
	"strings"
	"testing"
)

func TestSanitizeMessage_StripsHomeAndBounds(t *testing.T) {
	home := "/Users/demo/.atlas-home"
	msg := home + "/projects/x/memory/memory.db: permission denied"
	out := sanitizeMessage(msg, home)
	if out == "" || strings.Contains(out, home) {
		t.Fatalf("got %q", out)
	}
	if !strings.Contains(out, "$ATLAS_HOME") {
		t.Fatalf("expected $ATLAS_HOME placeholder, got %q", out)
	}

	long := strings.Repeat("a", maxMessageLen+50)
	out = sanitizeMessage(long, "")
	if len([]rune(out)) > maxMessageLen {
		t.Fatalf("len=%d", len([]rune(out)))
	}
}
