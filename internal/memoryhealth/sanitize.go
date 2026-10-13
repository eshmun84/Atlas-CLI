package memoryhealth

import (
	"strings"
)

const maxMessageLen = 240

// sanitizeMessage strips absolute Atlas Home paths and truncates for display.
func sanitizeMessage(msg, homePath string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if home := strings.TrimSpace(homePath); home != "" {
		msg = strings.ReplaceAll(msg, home, "$ATLAS_HOME")
		// Also strip common temp-home prefixes that may appear without trailing slash variance.
		msg = strings.ReplaceAll(msg, strings.TrimRight(home, "/"), "$ATLAS_HOME")
	}
	// Never leak absolute filesystem roots in normal diagnostics.
	if strings.HasPrefix(msg, "/") {
		if i := strings.Index(msg, "memory:"); i >= 0 {
			msg = msg[i:]
		} else if i := strings.Index(msg, ": "); i >= 0 && i+2 < len(msg) {
			// Keep classifier suffix when present.
			rest := msg[i+2:]
			if !strings.HasPrefix(rest, "/") {
				msg = rest
			} else {
				msg = "internal diagnostic error"
			}
		}
	}
	runes := []rune(msg)
	if len(runes) > maxMessageLen {
		msg = string(runes[:maxMessageLen-1]) + "…"
	}
	return msg
}

func checkMeta(kv ...string) map[string]string {
	if len(kv) == 0 || len(kv)%2 != 0 {
		return nil
	}
	out := make(map[string]string, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k := strings.TrimSpace(kv[i])
		v := strings.TrimSpace(kv[i+1])
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
