package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/output"
)

// The platform names the CLI it serves on every response; an older CLI says so, as
// cli_update in JSON (results and errors alike) and as one stderr line for a human.
func TestOutdatedCLIIsTold(t *testing.T) {
	was := Version
	t.Cleanup(func() { Version = was })
	Version = "pilot-old"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Whisk-CLI-Version", "pilot-new")
		if r.URL.Path == "/v1/whoami" {
			writeJSON(w, 200, map[string]any{"user": map[string]any{"email": "ana@acme.example"}, "orgs": []any{}})
			return
		}
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That app does not exist.", "fix": "Check the slug."}})
	}))
	defer srv.Close()
	dir := t.TempDir()

	r := runRemote(t, dir, srv.URL, nil, "whoami", "--json")
	n, _ := r.JSON["cli_update"].(map[string]any)
	if r.Code != 0 || n["current"] != "pilot-old" || n["latest"] != "pilot-new" || !strings.Contains(n["fix"].(string), "whisk update") {
		t.Fatalf("whoami --json: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "status", "--org", "acme", "--app", "nope", "--json")
	if _, ok := r.JSON["error"]; !ok || r.JSON["cli_update"] == nil {
		t.Fatalf("error --json carries no cli_update: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "whoami")
	if !strings.Contains(r.Stderr, "whisk pilot-new is available (this is pilot-old). Run whisk update.") {
		t.Fatalf("human stderr: %q", r.Stderr)
	}
	r = runRemote(t, dir, srv.URL, nil, "whoami", "--quiet")
	if strings.Contains(r.Stderr, "whisk update") {
		t.Fatalf("--quiet still printed the notice: %q", r.Stderr)
	}

	Version = "pilot-new"
	if r = runRemote(t, dir, srv.URL, nil, "whoami", "--json"); r.JSON["cli_update"] != nil {
		t.Fatalf("current CLI reported as outdated: %+v", r.JSON)
	}
}

func TestOutdated(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		want            bool
	}{
		{"pilot-a", "pilot-b", true},
		{"pilot-a", "pilot-a", false},
		{"pilot-a", "", false},
		{"dev", "pilot-b", false},
		{"", "pilot-b", false},
	} {
		if got := output.Outdated(c.current, c.latest); got != c.want {
			t.Errorf("Outdated(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}
