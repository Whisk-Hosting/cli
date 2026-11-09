package whisk

import (
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/gitcmd"
)

func TestPushRetry(t *testing.T) {
	reset := "fatal: unable to access 'https://git.whisk.run/o/a.git/': Send failure: Connection reset by peer"
	cases := []struct {
		name    string
		stderr  string
		took    time.Duration
		attempt int
		wait    time.Duration
		again   bool
	}{
		{"a reset at the start is tried again", reset, time.Second, 1, 2 * time.Second, true},
		{"and once more, a little later", reset, time.Second, 2, 5 * time.Second, true},
		{"then it is the agent's to retry", reset, time.Second, 3, 0, false},
		{"a long upload that dropped is not repeated", reset, 3 * time.Minute, 1, 0, false},
		{"a refused token is not a network failure", "fatal: Authentication failed for 'https://git.whisk.run/'", time.Second, 1, 0, false},
		{"a rejected push is not repeated", " ! [remote rejected] main -> main (pre-receive hook declined)", time.Second, 1, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wait, again := pushRetry(tc.stderr, gitcmd.PushUnknown, tc.took, tc.attempt)
			if wait != tc.wait || again != tc.again {
				t.Fatalf("pushRetry = %s %v, want %s %v", wait, again, tc.wait, tc.again)
			}
		})
	}
}
