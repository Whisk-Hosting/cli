package whisk

import (
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/apitypes"

	"github.com/whisk-run/cli/internal/api"
)

// A held update says how long the app's requests waited, or what the version that was live
// answers on when the update did not go live (CONTROL-PLANE.md §6.5).
func TestHoldLines(t *testing.T) {
	start := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	ended := start.Add(4 * time.Second)
	quick := start.Add(300 * time.Millisecond)
	point := start.Add(time.Second)
	for _, tc := range []struct {
		name string
		h    *apitypes.UpdateHold
		want string
	}{
		{"none", nil, ""},
		{"holding", &apitypes.UpdateHold{State: apitypes.UpdateHoldHeld, StartedAt: start}, "switch: the app's requests wait while the update switches"},
		{"switched", &apitypes.UpdateHold{State: apitypes.UpdateHoldSwitched, StartedAt: start, EndedAt: &ended},
			"switch: the app's requests waited 4 seconds while the update switched, so nothing was written in between"},
		{"switched at once", &apitypes.UpdateHold{State: apitypes.UpdateHoldSwitched, StartedAt: start, EndedAt: &quick},
			"switch: the app's requests waited 1 second while the update switched, so nothing was written in between"},
		{"put back", &apitypes.UpdateHold{State: apitypes.UpdateHoldPutBack, PointAt: &point, StartedAt: start, EndedAt: &ended},
			"switch: the update did not go live, so its data was put back as it was at " + when(&point) + ", and the version that was live answers again"},
		{"unchanged", &apitypes.UpdateHold{State: apitypes.UpdateHoldUnchanged, StartedAt: start, EndedAt: &ended},
			"switch: the update did not go live and changed no data, and the version that was live answers again"},
		{"released", &apitypes.UpdateHold{State: apitypes.UpdateHoldReleased, StartedAt: start, EndedAt: &ended},
			"switch: the update went on while the version that was live served"},
	} {
		if got := strings.Join(holdLines(tc.h), "\n"); got != tc.want {
			t.Errorf("%s: holdLines = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A failed deploy is followed while it puts back what it changed, a rollback's data or a held
// update's, and passes at once otherwise.
func TestUnsettled(t *testing.T) {
	ended := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		d    api.Deploy
		want bool
	}{
		{"an ordinary deploy", api.Deploy{}, false},
		{"a rollback giving its data back", api.Deploy{Data: &api.DataRollback{State: apitypes.DataRollbackHeld}}, true},
		{"a rollback settled", api.Deploy{Data: &api.DataRollback{State: apitypes.DataRollbackGivenBack, EndedAt: &ended}}, false},
		{"an update putting its data back", api.Deploy{Hold: &apitypes.UpdateHold{State: apitypes.UpdateHoldHeld}}, true},
		{"an update put back", api.Deploy{Hold: &apitypes.UpdateHold{State: apitypes.UpdateHoldPutBack, EndedAt: &ended}}, false},
	} {
		if got := unsettled(tc.d); got != tc.want {
			t.Errorf("%s: unsettled = %v, want %v", tc.name, got, tc.want)
		}
	}
}
