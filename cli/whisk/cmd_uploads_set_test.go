package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk uploads set sends only what it was given: a visibility, a canonical address, "" for none.
func TestUploadsSet(t *testing.T) {
	st := &dataState{}
	st.bodies = map[string][]map[string]any{}
	mux := http.NewServeMux()
	route := "PATCH /v1/orgs/acme/apps/crm/uploads/01PDF"
	mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		canonical, _ := body["canonical"].(string)
		writeJSON(w, 200, map[string]any{"id": "01PDF", "filename": "spec.pdf", "status": "clean", "visibility": "public",
			"file_path": "/.whisk/file/01PDF", "canonical": canonical})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "uploads", "set", "01PDF", "--canonical", "https://example.com/spec.pdf")
	if r.Code != 0 || !strings.Contains(r.Stdout, "canonical https://example.com/spec.pdf") {
		t.Fatalf("set canonical: %+v", r)
	}
	if body := st.bodies[route][0]; body["canonical"] != "https://example.com/spec.pdf" || body["visibility"] != nil {
		t.Errorf("sent %v", body)
	}
	if r := runRemote(t, dir, srv.URL, nil, "uploads", "set", "01PDF", "--public", "--canonical", ""); r.Code != 0 {
		t.Fatalf("public, no canonical: %+v", r)
	}
	if body := st.bodies[route][1]; body["canonical"] != "" || body["visibility"] != "public" {
		t.Errorf("sent %v", body)
	}
	if r := runRemote(t, dir, srv.URL, nil, "uploads", "set", "01PDF", "--path", "/specs/spec.pdf"); r.Code != 0 {
		t.Fatalf("path: %+v", r)
	}
	if body := st.bodies[route][2]; body["path"] != "/specs/spec.pdf" || body["canonical"] != nil {
		t.Errorf("sent %v", body)
	}
	if r := runRemote(t, dir, srv.URL, nil, "uploads", "set", "01PDF", "--json"); r.Code == 0 || errCode(r) != "INVALID_REQUEST" || len(st.bodies[route]) != 3 {
		t.Errorf("nothing to change: %+v", r)
	}
}
