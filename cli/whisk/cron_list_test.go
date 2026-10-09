package whisk

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// whisk cron list reads what is deployed when the directory is bound and signed in, since a live
// deploy can come from another manifest than the whisk.yaml beside it; only without either does
// it read whisk.yaml, and then says so (CLI.md §5.8).
func TestCronListReadsTheDeployedSchedules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orgs/acme/apps/crm/functions" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{"items": []any{
			map[string]any{"name": "reminders", "triggers": []any{map[string]any{"cron": "0 8 * * *", "tz": "Pacific/Auckland", "next_run": "2026-10-10T19:00:00Z"}}},
			map[string]any{"name": "demo-reset", "triggers": []any{map[string]any{"cron": "0 * * * *", "runs_as": "7 * * * *"}}},
			map[string]any{"name": "on-signup", "triggers": []any{map[string]any{"event": "user.created"}}},
		}})
	}))
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte("whisk: 1\nname: crm\nfunctions:\n  - name: reminders\n    cron: \"0 8 * * *\"\n    graph: workflows/r.graph.yaml\n"), 0o644))

	r := runRemote(t, dir, srv.URL, nil, "cron", "list", "--json")
	fns, _ := r.JSON["functions"].([]any)
	if r.Code != 0 || r.JSON["source"] != "deployed" || len(fns) != 2 {
		t.Fatalf("cron list: %+v", r)
	}
	if reset := fns[1].(map[string]any); reset["name"] != "demo-reset" || reset["tz"] != "UTC" || reset["runs_as"] != "7 * * * *" {
		t.Errorf("deployed cron = %v", reset)
	}
	r = runRemote(t, dir, srv.URL, nil, "cron", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "demo-reset") || !strings.Contains(r.Stdout, "runs as 7 * * * *") || strings.Contains(r.Stdout, "From whisk.yaml") {
		t.Errorf("cron list table: %+v", r)
	}
}
