package whisk

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk operator errors lists, shows, resolves, ignores and reopens the groups of Whisk's own
// error log (CLI.md §5.14).
func TestOperatorErrors(t *testing.T) {
	var seen, bodies []string
	group := `{"id":"g1","title":"request failed: timeout (/v1/orgs/{org}/apps)","source":"whiskd.service","status":"%s","count":4,"first_seen":"2026-10-06T10:00:00Z","last_seen":"2026-10-06T11:00:00Z","sample":"{\"level\":\"ERROR\"}","note":"%s","returned":1}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/operator/errors" && r.URL.Query().Get("source") == "nothing":
			_, _ = w.Write([]byte(`{"items":[],"next_cursor":"","open":0,"scanned_to":"2026-10-06T11:00:00Z"}`))
		case r.URL.Path == "/v1/operator/errors":
			_, _ = w.Write([]byte(`{"items":[` + strings.NewReplacer("%s", "open").Replace(group) + `],"next_cursor":"123_g1","open":1,"scanned_to":"2026-10-06T11:00:00Z"}`))
		case r.URL.Path == "/v1/operator/errors/g1" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(strings.NewReplacer("%s", "open").Replace(group)))
		case r.URL.Path == "/v1/operator/errors/g1":
			_, _ = w.Write([]byte(`{"id":"g1","status":"resolved","note":"Fixed the timeout."}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()

	r := runRemote(t, dir, srv.URL, nil, "operator", "errors", "--status", "all", "--source", "whiskd.service")
	if r.Code != 0 || !strings.Contains(r.Stdout, "g1") || !strings.Contains(r.Stdout, "request failed: timeout") || !strings.Contains(r.Stdout, "--cursor 123_g1") {
		t.Fatalf("list: %+v", r)
	}
	if !strings.Contains(seen[0], "status=all") || !strings.Contains(seen[0], "source=whiskd.service") {
		t.Errorf("list asked %s", seen[0])
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "errors", "--source", "nothing")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No open errors") {
		t.Fatalf("empty: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "errors", "show", "g1")
	if r.Code != 0 || !strings.Contains(r.Stdout, "4 times") || !strings.Contains(r.Stdout, "Came back:  1 times") || !strings.Contains(r.Stdout, `"level"`) {
		t.Fatalf("show: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "errors", "resolve", "g1", "--note", "Fixed the timeout.")
	if r.Code != 0 || !strings.Contains(r.Stdout, "g1 is resolved") {
		t.Fatalf("resolve: %+v", r)
	}
	if last := bodies[len(bodies)-1]; !strings.Contains(last, `"status":"resolved"`) || !strings.Contains(last, "Fixed the timeout.") {
		t.Errorf("resolve sent %s", last)
	}
	for use, status := range map[string]string{"ignore": "ignored", "reopen": "open"} {
		runRemote(t, dir, srv.URL, nil, "operator", "errors", use, "g1")
		if last := bodies[len(bodies)-1]; !strings.Contains(last, `"status":"`+status+`"`) {
			t.Errorf("%s sent %s", use, last)
		}
	}
}
