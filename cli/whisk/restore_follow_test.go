package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

func TestUndoLine(t *testing.T) {
	at := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	yes, no := true, false
	cases := []struct {
		restorable *bool
		want       string
	}{
		{nil, "whisk restore --at 2026-10-06T09:00:00Z"},
		{&yes, "whisk restore --at 2026-10-06T09:00:00Z"},
		{&no, "no self-service restore"},
	}
	for _, c := range cases {
		if got := undoLine(api.Snapshot{CreatedAt: at, Restorable: c.restorable}); !strings.Contains(got, c.want) {
			t.Errorf("%v: %q lacks %q", c.restorable, got, c.want)
		}
	}
}

func TestRestoreEndedAndRow(t *testing.T) {
	for status, want := range map[string]bool{"queued": false, "running": false, "done": true, "failed": true} {
		if restoreEnded(status) != want {
			t.Errorf("%s: want %v", status, want)
		}
	}
	row := restoreRow(api.Restore{ID: "r1", Status: "failed", Environment: "production", At: time.Date(2026, 9, 7, 14, 13, 0, 0, time.UTC), Error: &api.ErrorBody{Code: "RESTORE_FAILED"}})
	if !strings.Contains(row, "r1") || !strings.Contains(row, "2026-09-07T14:13:00Z") || !strings.Contains(row, "RESTORE_FAILED") {
		t.Errorf("row %q", row)
	}
}

// restoreAPI starts a restore that runs for two reads then finishes, and one that failed.
func restoreAPI() *httptest.Server {
	var reads atomic.Int32
	mux := http.NewServeMux()
	rec := func(id, status string) map[string]any {
		return map[string]any{"id": id, "environment": "production", "at": "2026-09-07T14:13:00Z", "swap": false, "status": status,
			"target_database": "app_a1_restore_20260907141300", "created_at": "2026-10-06T09:00:00Z"}
	}
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/restore", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, rec("r1", "queued"))
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/restores/r1", func(w http.ResponseWriter, r *http.Request) {
		status := "running"
		if reads.Add(1) > 2 {
			status = "done"
		}
		writeJSON(w, 200, rec("r1", status))
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/restores/r2", func(w http.ResponseWriter, r *http.Request) {
		f := rec("r2", "failed")
		f["error"] = map[string]any{"code": "RESTORE_FAILED", "message": "The restore could not finish.", "fix": "Try again."}
		writeJSON(w, 200, f)
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/restores", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []any{rec("r1", "done")}})
	})
	return httptest.NewServer(mux)
}

func TestRestoreWaitShowList(t *testing.T) {
	old := restorePoll
	restorePoll = time.Millisecond
	defer func() { restorePoll = old }()
	srv := restoreAPI()
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "restore", "--at", "2026-09-07T14:13:00Z", "--wait", "--json")
	if r.Code != 0 || r.JSON["restore"].(map[string]any)["status"] != "done" || !strings.Contains(r.Stderr, "restore: r1 is running") {
		t.Fatalf("restore --wait: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "restore", "show", "r2", "--json")
	if r.Code != 1 || errCode(r) != "RESTORE_FAILED" || r.JSON["error"].(map[string]any)["details"].(map[string]any)["restore"] != "r2" {
		t.Fatalf("restore show failed: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "restore", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "r1  done") {
		t.Fatalf("restore list: %+v", r)
	}
}
