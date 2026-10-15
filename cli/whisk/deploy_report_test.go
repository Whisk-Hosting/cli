package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorLines(t *testing.T) {
	cases := []struct {
		details map[string]any
		want    string
	}{
		{map[string]any{}, ""},
		{map[string]any{"log": []any{"a", "b"}}, "a|b"},
		{map[string]any{"log_tail": []any{"old"}}, "old"},
		{map[string]any{"log": []any{"new"}, "log_tail": []any{"old"}}, "new"},
		{map[string]any{"log": []any{"a", 3}}, "a"},
	}
	for _, c := range cases {
		if got := strings.Join(errorLines(c.details), "|"); got != c.want {
			t.Errorf("%v: got %q, want %q", c.details, got, c.want)
		}
	}
}

// deployReportAPI answers a crashed deploy, its build log, and an app whose container died.
func deployReportAPI() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys/d1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "d1", "build_id": "b1", "status": "failed", "phases": []any{},
			"error": map[string]any{"code": "CONTAINER_CRASHED", "message": "The app exited during start.", "fix": "Read the log.", "details": map[string]any{"log": []any{"Error: DATABASE_URL is not set"}}}})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/builds/b1/log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("#1 FROM node\n#9 DONE\n"))
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "a1", "slug": "crm", "state": "broken", "url": "https://crm--acme.whisk.page", "unset_secrets": []any{},
			"problem": map[string]any{"code": "CONTAINER_CRASHED", "message": "The app's container exited with code 1 after it went live, and stopped restarting.", "at": "2026-10-06T09:00:00Z", "exit_code": 1, "log": []any{"panic: boom"}}})
	})
	return httptest.NewServer(mux)
}

func TestDeploysLogAndStatusProblem(t *testing.T) {
	srv := deployReportAPI()
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "deploys", "log", "d1")
	if r.Code != 0 || !strings.Contains(r.Stdout, "#9 DONE") || !strings.Contains(r.Stdout, "DATABASE_URL is not set") {
		t.Fatalf("deploys log: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "deploys", "log", "d1", "--json")
	if r.Code != 0 || r.JSON["build_log"] != "#1 FROM node\n#9 DONE\n" || len(r.JSON["app_log"].([]any)) != 1 {
		t.Fatalf("deploys log --json: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "status")
	if r.Code != 0 || !strings.Contains(r.Stdout, "CONTAINER_CRASHED") || !strings.Contains(r.Stdout, "panic: boom") {
		t.Fatalf("status: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "status", "--json")
	if p, _ := r.JSON["problem"].(map[string]any); r.Code != 0 || p["exit_code"] != float64(1) {
		t.Fatalf("status --json: %+v", r)
	}
}
