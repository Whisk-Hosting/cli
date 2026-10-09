package api

import (
	"testing"

	"github.com/whisk-run/contract/status"
)

func TestTerminal(t *testing.T) {
	cases := map[status.Deploy]bool{
		"queued": false, "starting": false, "health_checking": false, "switching": false, "draining": false,
		"live": true, "failed": true, "rolled_back": true, "cancelled": true, "blocked": true, "superseded": true,
	}
	for s, want := range cases {
		if got := Terminal(s); got != want {
			t.Errorf("Terminal(%q) = %v, want %v", s, got, want)
		}
	}
}
