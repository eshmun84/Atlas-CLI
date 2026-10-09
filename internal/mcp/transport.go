package mcp

import "strings"

// TransportInvalid is returned by NormalizeTransport for unrecognized values.
// Validation must reject this; it must never silently become stdio.
const TransportInvalid Transport = "__invalid__"

// NormalizeTransport maps persisted/input transport values onto V1 transports.
// Blank and "stdio" map to stdio (legacy/default compatibility).
// "http" maps to streamable_http. "sse" is retained as TransportSSE.
// Anything else is TransportInvalid (validation must BLOCK).
func NormalizeTransport(value string) Transport {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(TransportStreamableHTTP), "http":
		return TransportStreamableHTTP
	case string(TransportSSE):
		return TransportSSE
	case string(TransportStdio), "":
		return TransportStdio
	default:
		return TransportInvalid
	}
}

// IsKnownTransport reports whether t is a recognized transport (including legacy sse).
func IsKnownTransport(t Transport) bool {
	switch NormalizeTransport(string(t)) {
	case TransportStdio, TransportStreamableHTTP, TransportSSE:
		return true
	default:
		return false
	}
}

// SelectableTransports returns transports offered when adding a custom MCP.
// SSE is intentionally omitted.
func SelectableTransports() []Transport {
	return []Transport{TransportStdio, TransportStreamableHTTP}
}

// TransportLabel returns a short display label.
func TransportLabel(t Transport) string {
	switch NormalizeTransport(string(t)) {
	case TransportStreamableHTTP:
		return "streamable_http"
	case TransportSSE:
		return "sse"
	case TransportInvalid:
		return "invalid"
	default:
		return "stdio"
	}
}
