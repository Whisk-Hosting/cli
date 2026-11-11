package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk cdn purge sends the paths, or none for --all, and says what was dropped; neither or
// both is refused before anything is sent.
func TestCDNPurge(t *testing.T) {
	var bodies []map[string]any
	on := true
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/cdn/purge", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		paths, _ := body["paths"].([]any)
		urls := []string{}
		for _, p := range paths {
			urls = append(urls, "https://crm.acme.whisk.page"+p.(string))
		}
		writeJSON(w, 200, map[string]any{"on": on, "all": on && len(paths) == 0, "urls": urls})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "cdn", "purge", "/entries/42", "/entries/*")
	if r.Code != 0 || len(bodies) != 1 || !strings.Contains(r.Stdout, "Dropped 2 address(es)") || !strings.Contains(r.Stdout, "https://crm.acme.whisk.page/entries/*") {
		t.Fatalf("purge paths: %+v %v", r, bodies)
	}
	r = runRemote(t, dir, srv.URL, nil, "cdn", "purge", "--all", "--json")
	p, _ := r.JSON["purge"].(map[string]any)
	if paths, _ := bodies[1]["paths"].([]any); r.Code != 0 || p["all"] != true || len(paths) != 0 {
		t.Fatalf("purge --all: %+v %v", r, bodies)
	}
	on = false
	if r = runRemote(t, dir, srv.URL, nil, "cdn", "purge", "--all"); r.Code != 0 || !strings.Contains(r.Stdout, "not served through the CDN") {
		t.Fatalf("purge with the CDN off: %+v", r)
	}
	for _, args := range [][]string{{"cdn", "purge"}, {"cdn", "purge", "--all", "/x"}} {
		if r = runRemote(t, dir, srv.URL, nil, append(args, "--json")...); r.Code == 0 || !strings.Contains(r.Stdout, "INVALID_REQUEST") {
			t.Errorf("%v: %+v", args, r)
		}
	}
	if len(bodies) != 3 {
		t.Fatalf("a refused purge sent nothing: %d calls", len(bodies))
	}
}
