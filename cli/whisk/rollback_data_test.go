package whisk

import (
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/apitypes"

	"github.com/whisk-run/cli/internal/api"
)

// whisk rollback --data posts to its own route, follows the deploy, and says where the data went
// back to and where the database it replaced is kept (CONTROL-PLANE.md §6.5).
func TestRollbackWithData(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{}

	r := runRemote(t, dir, srv.URL, git, "rollback", "d-live", "--data", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "INVALID_REQUEST") || len(st.rolledData) != 0 || len(st.posted) != 0 {
		t.Fatalf("a deploy named with --data is refused before anything is sent: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, git, "rollback", "--data", "--json")
	if r.Code != 0 || r.JSON["status"] != "live" || len(st.rolledData) != 1 || len(st.posted) != 0 {
		t.Fatalf("rollback --data: %+v (posted %v, rolled %v)", r, st.posted, st.rolledData)
	}
	if len(st.rolledData[0]) != 0 {
		t.Fatalf("rollback --data posts an empty body for production: %v", st.rolledData[0])
	}
	data, _ := r.JSON["data"].(map[string]any)
	if data["state"] != "restored" || data["aside"] != "app_a1_pre_1788858000" {
		t.Fatalf("the summary carries where the data stands: %+v", r.JSON)
	}

	r = runRemote(t, dir, srv.URL, git, "rollback", "--data")
	if r.Code != 0 || !strings.Contains(r.Stdout, "whisk db query --database app_a1_pre_1788858000") || !strings.Contains(r.Stderr+r.Stdout, "just before deploy d-mig's migration") {
		t.Fatalf("a person reads where the data went and where the old one is kept: %+v", r)
	}
}

func TestDataLines(t *testing.T) {
	point := time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)
	kept := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		r    *api.DataRollback
		want []string
	}{
		{"none", nil, nil},
		{"waiting", &api.DataRollback{State: apitypes.DataRollbackWaiting, FromDeploy: "d1", PointAt: point},
			[]string{"data: going back to " + at(point) + ", just before deploy d1's migration"}},
		{"held", &api.DataRollback{State: apitypes.DataRollbackHeld, FromDeploy: "d1", PointAt: point},
			[]string{"data: going back to " + at(point) + ", just before deploy d1's migration; the app's requests wait meanwhile"}},
		{"restored from the instant copy, kept until a time", &api.DataRollback{State: apitypes.DataRollbackRestored, FromDeploy: "d1", PointAt: point, Instant: true, Aside: "app_x_pre_1", KeptUntil: &kept},
			[]string{"data: as it was at " + at(point) + ", just before deploy d1's migration, from the instant copy taken then",
				"  the database it replaced is kept as app_x_pre_1 until " + when(&kept) + ": whisk db query --database app_x_pre_1 reads it"}},
		{"given back", &api.DataRollback{State: apitypes.DataRollbackGivenBack, FromDeploy: "d1", PointAt: point},
			[]string{"data: given back, as the rollback did not go live: the database is the one that was live"}},
		{"unchanged", &api.DataRollback{State: apitypes.DataRollbackUnchanged, FromDeploy: "d1", PointAt: point},
			[]string{"data: unchanged, as the rollback ended before it put anything back"}},
	}
	for _, c := range cases {
		if got := dataLines(c.r); strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: dataLines = %q, want %q", c.name, got, c.want)
		}
	}
}
