package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk operator resources lists a business's record and confirms an adopted row (CLI.md §5.14).
func TestOperatorResources(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/operator/orgs/acme/resources":
			_, _ = w.Write([]byte(`{"org":"acme","status":"shredded","done":true,"stuck":[],"resources":[
				{"id":"r1","kind":"repo","provider":"git","external_id":"01APP","state":"released","release":"delete","export":"repo"},
				{"id":"r2","kind":"stripe_customer","provider":"stripe","external_id":"cus_1","state":"retained","release":"retain","retained":"invoices are kept for tax law"},
				{"id":"r3","kind":"bucket","provider":"objstore","external_id":"org-old","state":"adopted","release":"delete"}]}`))
		case "GET /v1/operator/orgs/empty/resources":
			_, _ = w.Write([]byte(`{"org":"empty","status":"active","done":true,"stuck":[],"resources":[]}`))
		case "POST /v1/operator/orgs/acme/resources/r3/confirm":
			_, _ = w.Write([]byte(`{"resource":{"id":"r3","kind":"bucket","external_id":"org-old","state":"released"},"released":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()

	r := runRemote(t, dir, srv.URL, nil, "operator", "resources", "acme")
	if r.Code != 0 || !strings.Contains(r.Stdout, "cus_1") || !strings.Contains(r.Stdout, "invoices are kept") ||
		!strings.Contains(r.Stdout, "waits for whisk operator resources confirm") || !strings.Contains(r.Stdout, "Nothing is waiting") {
		t.Fatalf("list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "resources", "empty")
	if r.Code != 0 || !strings.Contains(r.Stdout, "empty has nothing recorded.") {
		t.Fatalf("empty: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "resources", "confirm", "acme", "r3")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Released bucket org-old.") {
		t.Fatalf("confirm: %+v", r)
	}
	if last := seen[len(seen)-1]; last != "POST /v1/operator/orgs/acme/resources/r3/confirm" {
		t.Errorf("confirm asked %s", last)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "resources")
	if r.Code == 0 {
		t.Errorf("resources without a business: %+v", r)
	}
}
