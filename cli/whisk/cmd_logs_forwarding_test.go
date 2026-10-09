package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// logDestinationsAPI answers the log forwarding routes of CONTROL-PLANE.md §5.1 for acme.
func logDestinationsAPI(included bool, items []map[string]any, tested *int) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/logs/destinations", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": items, "included": included, "limit": 3})
	})
	mux.HandleFunc("POST /v1/orgs/acme/logs/destinations/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "ld1" {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That log destination does not exist.", "fix": "Check the id."}})
			return
		}
		*tested++
		writeJSON(w, 200, map[string]any{"id": "ld1", "sent": true})
	})
	return httptest.NewServer(mux)
}

func TestLogForwarding(t *testing.T) {
	tested := 0
	items := []map[string]any{{
		"id": "ld1", "kind": "datadog", "site": "datadoghq.eu", "where": "Datadog (datadoghq.eu)", "headers": []string{"DD-API-KEY"},
		"status": "failing", "lines_sent": 1200, "last_sent_at": "2026-10-03T09:00:00Z", "last_error": "answered 403 forbidden",
		"created_at": "2026-10-01T09:00:00Z",
	}}
	srv := logDestinationsAPI(true, items, &tested)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "logs", "forwarding")
	for _, want := range []string{"ld1", "Datadog (datadoghq.eu)", "failing", "1200", "answered 403 forbidden"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("forwarding lacks %q: %+v", want, r)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "logs", "forwarding", "--json")
	if r.Code != 0 || r.JSON["included"] != true || len(r.JSON["destinations"].([]any)) != 1 || !strings.HasSuffix(r.JSON["dashboard"].(string), "/o/acme/settings#logs") {
		t.Fatalf("forwarding json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "logs", "forwarding", "test", "ld1")
	if r.Code != 0 || tested != 1 || !strings.Contains(r.Stdout, "took the test line") {
		t.Fatalf("test: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "logs", "forwarding", "test", "nope", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "NOT_FOUND") {
		t.Fatalf("test of an unknown destination: %+v", r)
	}
}

func TestLogForwardingNotOnPlan(t *testing.T) {
	tested := 0
	srv := logDestinationsAPI(false, []map[string]any{}, &tested)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	r := runRemote(t, dir, srv.URL, nil, "logs", "forwarding")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Business plan") || !strings.Contains(r.Stdout, "/o/acme/billing") {
		t.Fatalf("not on the plan: %+v", r)
	}
}

func TestLogForwardingNone(t *testing.T) {
	tested := 0
	srv := logDestinationsAPI(true, []map[string]any{}, &tested)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	r := runRemote(t, dir, srv.URL, nil, "logs", "forwarding")
	if r.Code != 0 || !strings.Contains(r.Stdout, "not forwarded anywhere") || !strings.Contains(r.Stdout, "/o/acme/settings#logs") {
		t.Fatalf("none: %+v", r)
	}
}
