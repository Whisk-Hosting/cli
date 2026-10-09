package whisk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

// values decodes every JSON value on stdout; --json promises exactly one (CLI.md §4).
func values(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var out []map[string]any
	for {
		var v map[string]any
		err := dec.Decode(&v)
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("stdout is not JSON values: %v\n%s", err, stdout)
		}
		out = append(out, v)
	}
}

func TestValues(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{`{"a":1}` + "\n", 1},
		{`{"a":1}` + "\n" + `{"error":{}}` + "\n", 2},
	}
	for _, c := range cases {
		if got := len(values(t, c.in)); got != c.want {
			t.Errorf("%q: %d values, want %d", c.in, got, c.want)
		}
	}
}

// A failing doctor under --json prints one object, the error, with the report inside it.
func TestDoctorFailingPrintsOneObject(t *testing.T) {
	dir := t.TempDir()
	r := runCLI(t, dir, nil, &memStore{}, "doctor", "--json")
	got := values(t, r.Stdout)
	if r.Code != 3 || len(got) != 1 {
		t.Fatalf("want exit 3 and one object, got %d and %d objects:\n%s", r.Code, len(got), r.Stdout)
	}
	e := got[0]["error"].(map[string]any)
	details := e["details"].(map[string]any)
	if e["code"] != "DOCTOR_FAILED" || e["docs"] != "https://skill.whisk.run/errors/DOCTOR_FAILED" || len(details["findings"].([]any)) == 0 {
		t.Fatalf("error: %v", e)
	}
}

// init that wrote files but could not create the app prints one object: the error, carrying
// what was written under details.init.
func TestInitFailingPrintsOneObject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-crm")
	must(t, os.MkdirAll(dir, 0o755))
	r := runCLI(t, dir, nil, &memStore{m: map[string]config.Credential{}}, "init", "--template", "go", "--json")
	got := values(t, r.Stdout)
	if r.Code != 4 || len(got) != 1 {
		t.Fatalf("want exit 4 and one object, got %d and %d objects:\n%s", r.Code, len(got), r.Stdout)
	}
	e := got[0]["error"].(map[string]any)
	initRes, _ := e["details"].(map[string]any)["init"].(map[string]any)
	if e["code"] != "AUTH_REQUIRED" || initRes["created"] != false || !strings.Contains(strings.Join(anyStrings(initRes["wrote"]), " "), "whisk.yaml") {
		t.Fatalf("error: %v", e)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("the template should still be written: %v", err)
	}
}

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// logs --json is the global flag: one object with every line as data, lines [] when empty.
func TestLogsJSONPrintsOneObject(t *testing.T) {
	var items []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/logs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": items})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "logs", "--json")
	got := values(t, r.Stdout)
	if r.Code != 0 || len(got) != 1 {
		t.Fatalf("empty window: exit %d, %d objects:\n%s", r.Code, len(got), r.Stdout)
	}
	if lines, ok := got[0]["lines"].([]any); !ok || len(lines) != 0 {
		t.Fatalf("an empty window is lines: [], got %v", got[0]["lines"])
	}

	items = []map[string]any{
		{"at": "2026-10-06T09:00:00Z", "stream": "stdout", "env": "production", "line": `{"level":"info","msg":"request","path":"/notes"}`, "json": map[string]any{"level": "info", "msg": "request", "path": "/notes"}},
		{"at": "2026-10-06T09:00:01Z", "stream": "stderr", "env": "production", "line": "plain text"},
	}
	r = runRemote(t, dir, srv.URL, nil, "logs", "--json")
	got = values(t, r.Stdout)
	if r.Code != 0 || len(got) != 1 || len(got[0]["lines"].([]any)) != 2 || got[0]["app"] != "crm" {
		t.Fatalf("two lines: exit %d:\n%s", r.Code, r.Stdout)
	}
	first := got[0]["lines"].([]any)[0].(map[string]any)
	if first["json"].(map[string]any)["path"] != "/notes" || first["stream"] != "stdout" {
		t.Fatalf("first line: %v", first)
	}

	r = runRemote(t, dir, srv.URL, nil, "logs", "--raw")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"path":"/notes"`) || !strings.Contains(r.Stdout, "plain text") {
		t.Fatalf("--raw: %+v", r)
	}
}
