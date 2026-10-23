package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

func TestRestartDeploy(t *testing.T) {
	for _, c := range []struct {
		name   string
		recent []api.Deploy
		want   string
		ok     bool
	}{
		{"the newest is production's", []api.Deploy{{ID: "d3", Environment: "production"}, {ID: "d2", Environment: "production"}}, "d3", true},
		{"a preview's deploy came in between", []api.Deploy{{ID: "d4", Environment: "preview:new-form"}, {ID: "d3", Environment: "production"}}, "d3", true},
		{"no deploys", nil, "", false},
	} {
		d, ok := restartDeploy(c.recent)
		if d.ID != c.want || ok != c.ok {
			t.Errorf("%s: %q %v, want %q %v", c.name, d.ID, ok, c.want, c.ok)
		}
	}
}

func TestAppsRestart(t *testing.T) {
	restarted := ""
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/orgs/acme/apps/{app}/restart", func(w http.ResponseWriter, r *http.Request) {
		restarted = r.PathValue("app")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "app_1", "slug": "crm", "status": "active"})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/{app}/deploys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
			{"id": "d_restart", "environment": "production", "status": "queued", "commit_sha": "abc123def456"},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "apps", "restart", "--no-wait", "--json")
	if r.Code != 0 || restarted != "crm" || r.JSON["deploy_id"] != "d_restart" {
		t.Fatalf("apps restart: %+v (restarted %q)", r, restarted)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "restart", "--no-wait")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Restart queued as deploy d_restart") {
		t.Fatalf("the human line: %+v", r)
	}
}
