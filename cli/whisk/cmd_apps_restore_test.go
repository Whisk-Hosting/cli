package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppsDeletedAndRestore(t *testing.T) {
	var restored string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/deleted-apps", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{
			"id": "app_1", "slug": "crm", "name": "CRM",
			"deleted_at": "2026-10-06T20:00:00Z", "release_at": "2026-10-13T20:00:00Z",
		}}})
	})
	mux.HandleFunc("POST /v1/orgs/acme/deleted-apps/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "app_1" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "APP_NOT_RESTORABLE", "message": "This app can no longer be restored.", "fix": "Create a new app."}})
			return
		}
		restored = r.PathValue("id")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "app_1", "slug": "crm", "status": "stopped"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "apps", "deleted", "--json")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"release_at":"2026-10-13T20:00:00Z"`) {
		t.Fatalf("apps deleted: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "restore", "app_1", "--json")
	if r.Code != 0 || restored != "app_1" || r.JSON["restored"] != true {
		t.Fatalf("apps restore: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "restore", "app_2", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "APP_NOT_RESTORABLE") {
		t.Fatalf("apps restore of a released app should fail with APP_NOT_RESTORABLE: %+v", r)
	}
}
