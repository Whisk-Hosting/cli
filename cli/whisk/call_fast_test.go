package whisk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/config"
)

// runCallWith runs the CLI as an agent with a config directory the test keeps, so what one run
// remembers the next one reads.
func runCallWith(t *testing.T, cfgDir, stdin, dir, api string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"WHISK_API": api, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude"}
	e := Env{Stdout: &out, Stderr: &errb, Stdin: strings.NewReader(stdin), Dir: dir, ConfigDir: cfgDir, Store: &memStore{m: map[string]config.Credential{}}, Getenv: func(k string) string { return env[k] }}
	code := Run(context.Background(), args, e)
	r := result{Code: code, Stdout: out.String(), Stderr: errb.String()}
	if lines := jsonLines(r.Stdout); len(lines) > 0 {
		r.JSON = lines[len(lines)-1]
	}
	return r
}

// A remembered address is used for an hour, and never one saved in the future.
func TestCallAddressUsable(t *testing.T) {
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		a    callAddress
		want bool
	}{
		{"just saved", callAddress{URL: "https://crm--acme.whisk.page", Environment: "production", Saved: now}, true},
		{"59 minutes", callAddress{URL: "https://crm--acme.whisk.page", Environment: "production", Saved: now.Add(-59 * time.Minute)}, true},
		{"an hour", callAddress{URL: "https://crm--acme.whisk.page", Environment: "production", Saved: now.Add(-time.Hour)}, false},
		{"from the future", callAddress{URL: "https://crm--acme.whisk.page", Environment: "production", Saved: now.Add(time.Minute)}, false},
		{"no address", callAddress{Environment: "production", Saved: now}, false},
		{"no environment", callAddress{URL: "https://crm--acme.whisk.page", Saved: now}, false},
	}
	for _, c := range cases {
		if got := c.a.usable(now); got != c.want {
			t.Errorf("%s: usable = %v, want %v", c.name, got, c.want)
		}
	}
	a := callAddressPath("/c", "https://api.whisk.run", "acme", "crm", "")
	for _, other := range []string{
		callAddressPath("/c", "http://localhost:8080", "acme", "crm", ""),
		callAddressPath("/c", "https://api.whisk.run", "acme", "crm", "draft"),
		callAddressPath("/c", "https://api.whisk.run", "acme", "shop", ""),
		callAddressPath("/c", "https://api.whisk.run", "acmec", "rm", ""),
	} {
		if other == a {
			t.Errorf("two apps share a remembered address: %s", a)
		}
	}
}

// Only an answer that may mean the address is no longer the app's asks the platform again; a
// request that may have been made is never sent twice.
func TestCallAddressStale(t *testing.T) {
	failed := errors.New("PLATFORM_UNAVAILABLE")
	cases := []struct {
		name      string
		a         callAnswer
		unreached bool
		err       error
		want      bool
	}{
		{"ok", callAnswer{Status: 200}, false, nil, false},
		{"app error", callAnswer{Status: 400}, false, nil, false},
		{"not found", callAnswer{Status: 404}, false, nil, true},
		{"misdirected", callAnswer{Status: 421}, false, nil, true},
		{"server error", callAnswer{Status: 502}, false, nil, false},
		{"nothing there", callAnswer{}, true, failed, true},
		{"timed out after sending", callAnswer{}, false, failed, false},
	}
	for _, c := range cases {
		if got := callAddressStale(c.a, c.unreached, c.err); got != c.want {
			t.Errorf("%s: stale = %v, want %v", c.name, got, c.want)
		}
	}
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	read := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}
	if !unreached(dial) || !unreached(&net.DNSError{Err: "no such host", Name: "x"}) || unreached(read) || unreached(context.DeadlineExceeded) {
		t.Errorf("unreached: dial %v, read %v", unreached(dial), unreached(read))
	}
}

// The address is asked for once and remembered; when the app moves, the remembered address
// answers 404, the platform is asked again and the request goes to the new address. A real 404
// from the app at an address that has not changed is not sent twice.
func TestCallRemembersTheAddress(t *testing.T) {
	var lookups, oldHits, newHits atomic.Int32
	var moved atomic.Bool
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		path := "/old"
		if moved.Load() {
			path = "/new"
		}
		writeJSON(w, 200, map[string]any{"slug": "crm", "url": srv.URL + path})
	})
	mux.HandleFunc("/old/", func(w http.ResponseWriter, r *http.Request) {
		oldHits.Add(1)
		if moved.Load() || r.URL.Path == "/old/missing" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{"at": "old"})
	})
	mux.HandleFunc("/new/", func(w http.ResponseWriter, r *http.Request) {
		newHits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		writeJSON(w, 201, map[string]any{"at": "new"})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	dir, cfg := boundDir(t, srv.URL), t.TempDir()

	for range 3 {
		if r := runCallWith(t, cfg, "", dir, srv.URL, "call", "GET", "/x", "--json"); r.Code != 0 {
			t.Fatalf("call: %+v", r)
		}
	}
	if lookups.Load() != 1 || oldHits.Load() != 3 {
		t.Fatalf("three calls asked for the address %d times and reached the app %d times; want 1 and 3", lookups.Load(), oldHits.Load())
	}

	r := runCallWith(t, cfg, "", dir, srv.URL, "call", "GET", "/missing", "--json")
	if errCode(r) != "CALL_FAILED" || lookups.Load() != 2 || oldHits.Load() != 4 {
		t.Fatalf("the app's own 404: %+v, %d lookups, %d hits", r, lookups.Load(), oldHits.Load())
	}

	moved.Store(true)
	r = runCallWith(t, cfg, "", dir, srv.URL, "call", "POST", "/notes", "--data", `{"body":"hi"}`, "--json")
	if r.Code != 0 || r.JSON["status"] != float64(201) || !strings.HasPrefix(r.JSON["url"].(string), srv.URL+"/new/") {
		t.Fatalf("after the app moved: %+v", r)
	}
	if lookups.Load() != 3 || newHits.Load() != 1 {
		t.Fatalf("after the app moved: %d lookups, %d requests at the new address; want 3 and 1", lookups.Load(), newHits.Load())
	}
	if r := runCallWith(t, cfg, "", dir, srv.URL, "call", "GET", "/x", "--json"); r.Code != 0 || lookups.Load() != 3 || newHits.Load() != 2 {
		t.Fatalf("the new address was not remembered: %+v, %d lookups", r, lookups.Load())
	}
}

// A batch line is checked as whisk call checks its arguments.
func TestParseCallBatchLine(t *testing.T) {
	cases := []struct {
		raw, method, body, ct, err string
	}{
		{`{"method":"get","path":"/api/me"}`, "GET", "", "", ""},
		{`{"method":"POST","path":"/api/contacts","data":{ "first_name": "Ann" }}`, "POST", `{"first_name":"Ann"}`, "application/json", ""},
		{`{"method":"PUT","path":"/f","data":"plain text","headers":{"Content-Type":"text/plain"}}`, "PUT", "plain text", "text/plain", ""},
		{`{"method":"POST","path":"/f","data":"quoted"}`, "POST", `"quoted"`, "application/json", ""},
		{`{"method":"POST","path":"/f","data":null}`, "POST", "", "", ""},
		{`{"method":"FETCH","path":"/x"}`, "", "", "", "INVALID_REQUEST"},
		{`{"method":"GET","path":"/x","headers":{"Authorization":"Bearer x"}}`, "", "", "", "INVALID_REQUEST"},
		{`{"method":"GET","path":"/x","body":"{}"}`, "", "", "", "INVALID_REQUEST"},
		{`not json`, "", "", "", "INVALID_REQUEST"},
	}
	for _, c := range cases {
		it := parseCallBatchLine(3, []byte(c.raw))
		if c.err != "" {
			if it.Err == nil || !strings.Contains(callBatchResult(it, callAnswer{}, it.Err)["error"].(map[string]any)["code"].(string), c.err) {
				t.Errorf("%s: err %v, want %s", c.raw, it.Err, c.err)
			}
			continue
		}
		if it.Err != nil || it.Method != c.method || string(it.Body) != c.body || it.Header.Get("Content-Type") != c.ct {
			t.Errorf("%s: %+v (%v), body %q", c.raw, it, it.Err, it.Body)
		}
	}
}

// --batch sends every line over one run, prints one answer per line in the batch's order, and
// fails with the lines that failed while making the rest.
func TestCallBatch(t *testing.T) {
	var lookups atomic.Int32
	var mu sync.Mutex
	made := map[string]bool{}
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		writeJSON(w, 200, map[string]any{"slug": "crm", "url": srv.URL + "/app"})
	})
	mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/app/bad" {
			writeJSON(w, 400, map[string]any{"errors": []any{"name is required"}})
			return
		}
		mu.Lock()
		made[r.Method+" "+r.URL.Path+" "+string(b)] = true
		mu.Unlock()
		// Earlier lines answer later, so the order printed is the batch's, not the answers'.
		if strings.HasSuffix(r.URL.Path, "/1") {
			time.Sleep(50 * time.Millisecond)
		}
		writeJSON(w, 201, map[string]any{"path": r.URL.Path})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	batch := strings.Join([]string{
		`{"method":"POST","path":"/c/1","data":{"n":1}}`,
		``,
		`{"method":"POST","path":"/c/2","data":{"n":2}}`,
		`{"method":"POST","path":"/bad","data":{}}`,
		`{"method":"NOPE","path":"/c/4"}`,
		`{"method":"POST","path":"/c/5","data":{"n":5}}`,
	}, "\n")
	r := runCallWith(t, t.TempDir(), batch, dir, srv.URL, "call", "--batch", "-", "--parallel", "4", "--json")
	lines := jsonLines(r.Stdout)
	if r.Code == 0 || errCode(r) != "CALL_FAILED" || len(lines) != 6 {
		t.Fatalf("batch: %+v", r)
	}
	wantLines := []float64{1, 3, 4, 5, 6}
	for i, want := range wantLines {
		if lines[i]["line"] != want {
			t.Errorf("answer %d is for line %v, want %v", i, lines[i]["line"], want)
		}
	}
	if lines[0]["status"] != float64(201) || lines[2]["error"] == nil || lines[3]["error"] == nil || lines[4]["status"] != float64(201) {
		t.Errorf("answers: %v", lines)
	}
	details, _ := r.JSON["error"].(map[string]any)["details"].(map[string]any)
	if details["failed"] != float64(2) || len(details["lines"].([]any)) != 2 {
		t.Errorf("summary: %v", r.JSON)
	}
	for _, m := range []string{`POST /app/c/1 {"n":1}`, `POST /app/c/2 {"n":2}`, `POST /app/c/5 {"n":5}`} {
		if !made[m] {
			t.Errorf("%s was not made: %v", m, made)
		}
	}
	if lookups.Load() != 1 {
		t.Errorf("the batch asked for the address %d times", lookups.Load())
	}

	r = runCallWith(t, t.TempDir(), `{"method":"GET","path":"/c/9"}`, dir, srv.URL, "call", "--batch", "-", "--json")
	if r.Code != 0 || r.JSON["requests"] != float64(1) || r.JSON["failed"] != float64(0) {
		t.Errorf("a batch that all worked: %+v", r)
	}
	for _, args := range [][]string{
		{"call", "--batch", "-", "--parallel", "9"},
		{"call", "--batch", "-", "GET", "/x"},
		{"call", "--batch", "missing.ndjson"},
		{"call", "GET"},
	} {
		if r := runCallWith(t, t.TempDir(), "", dir, srv.URL, append(args, "--json")...); errCode(r) != "INVALID_REQUEST" {
			t.Errorf("%v: %+v", args, r)
		}
	}
	if r := runCallWith(t, t.TempDir(), "\n\n", dir, srv.URL, "call", "--batch", "-", "--json"); errCode(r) != "INVALID_REQUEST" {
		t.Errorf("an empty batch: %+v", r)
	}
}
