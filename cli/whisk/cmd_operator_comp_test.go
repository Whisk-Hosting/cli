package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk operator comp starts, lists and ends a comp with what it was given (CLI.md §5.14).
func TestOperatorComp(t *testing.T) {
	var bodies []map[string]any
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "PUT /v1/operator/orgs/acme/comp":
			_, _ = w.Write([]byte(`{"id":"o1","slug":"acme","name":"Acme","plan":"team","status":"active","comp":{"plan":"team","ends_at":"2027-03-31T23:59:59Z","reason":"design partner","by":"op@whisk.test"}}`))
		case "DELETE /v1/operator/orgs/acme/comp":
			_, _ = w.Write([]byte(`{"id":"o1","slug":"acme","name":"Acme","plan":"free","status":"active"}`))
		case "GET /v1/operator/orgs":
			_, _ = w.Write([]byte(`{"items":[{"id":"o1","slug":"acme","name":"Acme","plan":"team","comp":{"plan":"team","ends_at":null,"reason":"design partner"}},{"id":"o2","slug":"beta","name":"Beta","plan":"free"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()

	r := runRemote(t, dir, srv.URL, nil, "operator", "comp", "start", "acme", "team", "--until", "2027-03-31", "--reason", "design partner")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Acme is on team free of charge until 2027-03-31.") {
		t.Fatalf("start: %+v", r)
	}
	if b := bodies[len(bodies)-1]; b["plan"] != "team" || b["ends_at"] != "2027-03-31" || b["reason"] != "design partner" {
		t.Errorf("start sent %v", b)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "comp", "start", "acme", "team")
	if r.Code == 0 {
		t.Errorf("start without a reason: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "comp", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "acme") || !strings.Contains(r.Stdout, "with no end") || strings.Contains(r.Stdout, "beta") {
		t.Fatalf("list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "comp", "end", "acme")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Acme is back on free.") {
		t.Fatalf("end: %+v", r)
	}
	if last := seen[len(seen)-1]; last != "DELETE /v1/operator/orgs/acme/comp" {
		t.Errorf("end asked %s", last)
	}
}

// whisk operator holds lists the abuse holds with what found them (CLI.md §5.14).
func TestOperatorHolds(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"h1","org":"scam","org_name":"Scam","person":"x@example.com","ips":["203.0.113.9"],"devices":1,"kind":"phishing_page","detail":{"brand":"paypal"},"status":"held","created_at":"2026-10-05T01:00:00Z"}]}`))
	}))
	defer srv.Close()
	r := runRemote(t, t.TempDir(), srv.URL, nil, "operator", "holds", "--status", "held")
	if r.Code != 0 || !strings.Contains(r.Stdout, "password form naming paypal") || !strings.Contains(r.Stdout, "203.0.113.9") {
		t.Fatalf("holds: %+v", r)
	}
	if asked != "/v1/operator/holds?status=held" {
		t.Errorf("asked %s", asked)
	}
}
