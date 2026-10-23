package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppsDisplayName(t *testing.T) {
	var asked string
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /v1/orgs/acme/apps/{app}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name *string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Name == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		asked = r.PathValue("app") + " " + *in.Name
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "app_1", "slug": "crm", "name": *in.Name, "hostname": "crm.acme.whisk.page", "status": "active"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	for _, bad := range []string{"  ", strings.Repeat("a", MaxDisplayName+1), "Two\nlines"} {
		r := runRemote(t, dir, srv.URL, nil, "apps", "display-name", bad)
		if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "INVALID_REQUEST") || asked != "" {
			t.Fatalf("%q is refused before any call: %+v", bad, r)
		}
	}
	r := runRemote(t, dir, srv.URL, nil, "apps", "display-name", " Lakeside Supply ", "--json")
	if r.Code != 0 || asked != "crm Lakeside Supply" {
		t.Fatalf("apps display-name: %+v (asked %q)", r, asked)
	}
	if app, _ := r.JSON["app"].(map[string]any); app["name"] != "Lakeside Supply" {
		t.Fatalf("the result carries the app: %+v", r.JSON)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "display-name", "Lakeside Supply")
	if r.Code != 0 || !strings.Contains(r.Stdout, `acme/crm is shown as "Lakeside Supply": https://crm.acme.whisk.page`) {
		t.Fatalf("the human line: %+v", r)
	}
}
