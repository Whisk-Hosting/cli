package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk uploads link signs the paths given for the time asked; rotate-key gives the app a new
// key, at once with --now; the platform's refusal is the command's (CLI.md §5.9).
func TestUploadsLinkAndRotate(t *testing.T) {
	st := &dataState{}
	st.bodies = map[string][]map[string]any{}
	mux := http.NewServeMux()
	app := "/v1/orgs/acme/apps/crm"
	mux.HandleFunc("POST "+app+"/uploads/links", func(w http.ResponseWriter, r *http.Request) {
		if missingKey(r) || r.Header.Get("Idempotency-Key") != "" {
			t.Errorf("a no-replay route carried an Idempotency-Key")
		}
		body := st.record(r)
		paths, _ := body["paths"].([]any)
		if len(paths) > 0 && paths[0] == "/notes" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "\"/notes\" is not an image or media address.", "fix": "Sign /.whisk/img/<id>."}})
			return
		}
		writeJSON(w, 200, map[string]any{"expires_at": "2026-10-09T05:00:00Z", "links": []any{
			map[string]any{"id": "01UP", "path": "/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s", "url": "https://crm--acme.whisk.page/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s"}}})
	})
	mux.HandleFunc("POST "+app+"/uploads/links/rotate", func(w http.ResponseWriter, r *http.Request) {
		if missingKey(r) {
			t.Errorf("rotate without an Idempotency-Key")
		}
		body := st.record(r)
		until := "2026-10-09T16:00:00Z"
		if body["immediately"] == true {
			until = "2026-10-09T04:00:00Z"
		}
		writeJSON(w, 200, map[string]any{"generation": 2, "rotated_at": "2026-10-09T04:00:00Z", "previous_until": until})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "uploads", "link", "/.whisk/img/01UP?w=800", "--expires", "30m", "--json")
	links, _ := r.JSON["links"].([]any)
	if r.Code != 0 || len(links) != 1 || r.JSON["expires_at"] != "2026-10-09T05:00:00Z" {
		t.Fatalf("uploads link: %+v", r)
	}
	if body := st.bodies["POST "+app+"/uploads/links"][0]; body["expires_in"] != float64(1800) || body["paths"].([]any)[0] != "/.whisk/img/01UP?w=800" {
		t.Errorf("asked %v", body)
	}
	r = runRemote(t, dir, srv.URL, nil, "uploads", "link", "/.whisk/img/01UP?w=800")
	if r.Code != 0 || !strings.Contains(r.Stdout, "https://crm--acme.whisk.page/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s") || !strings.Contains(r.Stdout, "Anyone with a link") {
		t.Errorf("uploads link, for a person: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "uploads", "link", "/notes", "--json")
	if r.Code == 0 || errCode(r) != "INVALID_REQUEST" {
		t.Errorf("a refused path: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "uploads", "rotate-key")
	if r.Code != 0 || !strings.Contains(r.Stdout, "keep working until they expire") {
		t.Errorf("rotate-key: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "uploads", "rotate-key", "--now", "--json")
	if r.Code != 0 || st.bodies["POST "+app+"/uploads/links/rotate"][1]["immediately"] != true {
		t.Errorf("rotate-key --now: %+v", r)
	}
}
