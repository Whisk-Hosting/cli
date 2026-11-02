// Package edgespan is what the edge tells the control plane about each request it passes to an
// app (CADDY.md §5.1 "Edge spans", CONTROL-PLANE.md §6.17 "Traces"): one span per request, under
// the request id's trace, whose span id is the parent id the app received in traceparent. So
// every app's Traces page lists its requests with no code of its own, and an app that sends its
// own spans finds them under the edge's.
package edgespan

import "time"

// Path is where the edge posts batches: on the control plane's forward-auth listener, which only
// the edge reaches over the mesh.
const Path = "/edge-spans"

// MaxBatch is the most spans one post carries.
const MaxBatch = 500

// MaxPath is the longest path a span keeps; the rest is cut.
const MaxPath = 1024

// Span is one request as the edge saw it. The query string is never sent.
type Span struct {
	OrgID         string `json:"org_id"`
	AppID         string `json:"app_id"`
	EnvironmentID string `json:"environment_id,omitempty"` // a preview's environment; "" is production
	TraceID       string `json:"trace_id"`                 // 32 lowercase hex: the request id's 16 bytes
	SpanID        string `json:"span_id"`                  // 16 lowercase hex: the parent id in the app's traceparent
	Start         int64  `json:"start_unix_nano"`
	DurationNS    int64  `json:"duration_ns"`
	Method        string `json:"method"`
	Host          string `json:"host"`
	Path          string `json:"path"`
	Status        int    `json:"status"`
	Bytes         int64  `json:"bytes,omitempty"`
	// Audience is who the gate let through: team, customer, service, anonymous, or "" when the
	// gate answered the request itself before deciding.
	Audience string `json:"audience,omitempty"`
}

// Batch is one post's body.
type Batch struct {
	Spans []Span `json:"spans"`
}

// StartTime is the span's start as a time.
func (s Span) StartTime() time.Time { return time.Unix(0, s.Start).UTC() }

// Cut is the path cut to MaxPath bytes, at a rune boundary. Pure.
func Cut(path string) string {
	if len(path) <= MaxPath {
		return path
	}
	n := MaxPath
	for n > 0 && path[n]&0xC0 == 0x80 {
		n--
	}
	return path[:n]
}
