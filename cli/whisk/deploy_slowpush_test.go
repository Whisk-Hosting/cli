package whisk

import (
	"strings"
	"testing"
	"time"
)

func TestPushUnreachable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		took    time.Duration
		message string
		fix     string
		seconds any
	}{
		{"a host out of reach", 3 * time.Second, "git push could not reach the platform: fatal: the remote end hung up unexpectedly", "Retry with backoff", nil},
		{"a slow upload cut part-way", 833 * time.Second, "git push lost its connection after 13 minutes of uploading: fatal: the remote end hung up unexpectedly", "keep large files", 833},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pushUnreachable("fatal: the remote end hung up unexpectedly", tc.took, map[string]any{})
			if e.Code != "PLATFORM_UNAVAILABLE" || e.Message != tc.message || !strings.Contains(e.Fix, tc.fix) {
				t.Errorf("pushUnreachable = %s %q fix %q", e.Code, e.Message, e.Fix)
			}
			if got := e.Details["push_seconds"]; got != tc.seconds {
				t.Errorf("details.push_seconds = %v, want %v", got, tc.seconds)
			}
		})
	}
}
