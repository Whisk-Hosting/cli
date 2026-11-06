package manifest

import "testing"

// The analytics leave out a browser asking not to be tracked and the business's own people unless
// the manifest says otherwise (CONTROL-PLANE.md §6.41).
func TestAnalyticsDefaults(t *testing.T) {
	cases := []struct {
		a              *Analytics
		respects, team bool
	}{
		{nil, true, false},
		{&Analytics{}, true, false},
		{&Analytics{DoNotTrack: DoNotTrackIgnore}, false, false},
		{&Analytics{Team: TeamCount}, true, true},
		{&Analytics{Team: TeamLeaveOut, DoNotTrack: DoNotTrackRespect}, true, false},
	}
	for _, c := range cases {
		m := Manifest{Analytics: c.a}
		if m.RespectsDoNotTrack() != c.respects || m.CountsTeam() != c.team {
			t.Errorf("%+v: respects %v, counts the team %v", c.a, m.RespectsDoNotTrack(), m.CountsTeam())
		}
	}
}
