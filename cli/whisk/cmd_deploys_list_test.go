package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk deploys list --env asks the API for one environment's deploys, and says so when it has
// none; without it every environment's are listed.
func TestDeploysListOfOneEnvironment(t *testing.T) {
	asked := []string{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys", func(w http.ResponseWriter, r *http.Request) {
		env := r.URL.Query().Get("env")
		asked = append(asked, env)
		items := []map[string]any{}
		if env != "preview:empty" {
			items = append(items, map[string]any{"id": "d_1", "environment": "preview:form", "status": "live", "commit_sha": "abc123def456"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "deploys", "list", "--env", "preview:form", "--json")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"d_1"`) {
		t.Fatalf("deploys list --env: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "deploys", "list", "--env", "preview:empty")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No deploys to preview:empty yet.") {
		t.Fatalf("an environment with none: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "builds", "list", "--json")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"d_1"`) {
		t.Fatalf("builds list: %+v", r)
	}
	if strings.Join(asked, ",") != "preview:form,preview:empty," {
		t.Errorf("env asked = %q, want preview:form, preview:empty, then none", asked)
	}
}

// whisk envs delete never asks, so the --yes agents pass to a delete is taken.
func TestEnvsDeleteTakesYes(t *testing.T) {
	deleted := ""
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /v1/orgs/acme/apps/crm/environments/{env}", func(w http.ResponseWriter, r *http.Request) {
		deleted = r.PathValue("env")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "envs", "delete", "preview:form", "--yes", "--json")
	if r.Code != 0 || deleted != "preview:form" || r.JSON["deleted"] != true {
		t.Fatalf("envs delete --yes: %+v (deleted %q)", r, deleted)
	}
}
