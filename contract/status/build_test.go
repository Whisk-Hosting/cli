package status

import "testing"

// A build runs once and ends once (buildMoves): nothing moves a finished build on.
func TestBuildMoves(t *testing.T) {
	for _, c := range []struct {
		from, to Build
		want     bool
	}{
		{BuildQueued, BuildRunning, true},
		{BuildQueued, BuildCancelled, true},
		{BuildQueued, BuildSucceeded, true}, // a release's build, recorded built
		{BuildRunning, BuildSucceeded, true},
		{BuildRunning, BuildFailed, true},
		{BuildRunning, BuildCancelled, true},
		{BuildRunning, BuildQueued, false},
		{BuildCancelled, BuildSucceeded, false},
		{BuildFailed, BuildSucceeded, false},
		{BuildSucceeded, BuildCancelled, false},
		{"nonsense", BuildRunning, false},
	} {
		if got := c.from.CanMove(c.to); got != c.want {
			t.Errorf("%s to %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
	for _, st := range []Build{BuildSucceeded, BuildFailed, BuildCancelled} {
		if !st.Finished() {
			t.Errorf("%s is not finished", st)
		}
	}
	if BuildQueued.Finished() || BuildRunning.Finished() || Build("nonsense").Finished() {
		t.Error("a build still to run, or no status at all, reads as finished")
	}
	for _, st := range Builds() {
		if _, ok := buildMoves[st]; !ok {
			t.Errorf("%s has no row in buildMoves", st)
		}
	}
}
