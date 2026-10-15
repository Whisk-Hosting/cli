package apitypes

import "testing"

// Only an analysis that found no call sets a finding aside; anything else, including the empty
// value of a scan stored before reachability was recorded, counts as called (CONTROL-PLANE.md
// §6.28).
func TestPackageReachNotCalled(t *testing.T) {
	for _, tc := range []struct {
		reach PackageReach
		want  bool
	}{
		{ReachNotCalled, true},
		{ReachCalled, false},
		{ReachUnknown, false},
		{"", false},
		{"Not_Called", false},
	} {
		if got := tc.reach.NotCalled(); got != tc.want {
			t.Errorf("%q.NotCalled() = %v, want %v", tc.reach, got, tc.want)
		}
	}
}
