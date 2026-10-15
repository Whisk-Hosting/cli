package whisk

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/config"
)

// A path is joined to the app's address and never leaves it (CLI.md §5.9 "whisk call").
func TestCallTarget(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://site--acme.whisk.page", "/cms/api/pages?limit=5", "https://site--acme.whisk.page/cms/api/pages?limit=5"},
		{"https://shop.example.com/", "/cms/api/product-pages/4", "https://shop.example.com/cms/api/product-pages/4"},
		{"https://site--acme.whisk.page", "/a/../b", "https://site--acme.whisk.page/b"},
		{"https://site--acme.whisk.page", "https://evil.example/x", ""},
		{"https://site--acme.whisk.page", "//evil.example/x", ""},
		{"https://site--acme.whisk.page", `/\evil.example`, ""},
		{"https://site--acme.whisk.page", "cms/api", ""},
		{"https://site--acme.whisk.page", "", ""},
		{"", "/x", ""},
		{"ftp://site", "/x", ""},
	}
	for _, c := range cases {
		u, err := callTarget(c.base, c.path)
		got := ""
		if err == nil {
			got = u.String()
		}
		if got != c.want {
			t.Errorf("callTarget(%q, %q) = %q, %v; want %q", c.base, c.path, got, err, c.want)
		}
	}
}

func TestCallHeaders(t *testing.T) {
	h, err := callHeaders([]string{"Content-Type: text/plain", "X-Trace:  a:b "})
	if err != nil || h.Get("Content-Type") != "text/plain" || h.Get("X-Trace") != "a:b" {
		t.Fatalf("headers: %v %v", h, err)
	}
	for _, bad := range []string{"Authorization: Bearer x", "cookie: a=b", "Host: x", "no colon", ": empty", "Two words: x"} {
		if _, err := callHeaders([]string{bad}); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// The edge's refusals keep their code; anything else outside 2xx is CALL_FAILED with the answer.
func TestCallOutcome(t *testing.T) {
	u, _ := url.Parse("https://site--acme.whisk.page/cms/api/pages/4")
	edge := `{"error":{"code":"AGENT_CATEGORY","message":"x","fix":"y","docs":"https://skill.whisk.run/errors/AGENT_CATEGORY"}}`
	appOwn := `{"error":{"code":"AGENT_CATEGORY","message":"x","fix":"y"}}`
	cases := []struct {
		name string
		a    callAnswer
		want string
	}{
		{"ok", callAnswer{Status: 200}, ""},
		{"created", callAnswer{Status: 201}, ""},
		{"edge refusal", callAnswer{Status: 403, ContentType: "application/json", Body: []byte(edge)}, "AGENT_CATEGORY"},
		{"app's own error shaped alike", callAnswer{Status: 403, ContentType: "application/json", Body: []byte(appOwn)}, "CALL_FAILED"},
		{"app error", callAnswer{Status: 400, ContentType: "application/json", Body: []byte(`{"errors":[]}`)}, "CALL_FAILED"},
		{"html failure", callAnswer{Status: 502, ContentType: "text/html", Body: []byte("<h1>x</h1>")}, "CALL_FAILED"},
		{"redirect", callAnswer{Status: 302, Location: "/login"}, "CALL_FAILED"},
	}
	for _, c := range cases {
		err := callOutcome("PATCH", u, c.a)
		got := ""
		if err != nil {
			got = wrap(err).(interface{ Body() map[string]any }).Body()["error"].(map[string]any)["code"].(string)
		}
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// whisk call reaches the app's address with the credential, sends the body, prints the answer,
// and refuses before sending anything when the request or the credential cannot work.
func TestCallSendsToTheApp(t *testing.T) {
	var got struct {
		auth, ct, body, method, path string
		calls                        int
	}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"slug": "crm", "url": srv.URL + "/app"})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/environments", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []any{map[string]any{"name": "production", "url": srv.URL + "/app"}, map[string]any{"name": "preview:draft", "url": srv.URL + "/preview"}}})
	})
	mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.auth, got.ct, got.body, got.method, got.path = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b), r.Method, r.URL.RequestURI()
		got.calls++
		w.Header().Set("X-Whisk-Request-Id", "01REQ")
		switch r.URL.Path {
		case "/app/cms/api/pages/4":
			writeJSON(w, 200, map[string]any{"doc": map[string]any{"id": 4, "title": "Home"}})
		case "/app/refused":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "AGENT_CATEGORY", "message": "Claude may not change data.", "fix": "Ask the owner.", "docs": "https://skill.whisk.run/errors/AGENT_CATEGORY"}})
		case "/app/bad":
			writeJSON(w, 400, map[string]any{"errors": []any{map[string]any{"message": "title is required"}}})
		case "/app/slow":
			time.Sleep(500 * time.Millisecond)
			writeJSON(w, 200, map[string]any{})
		case "/app/moved":
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/preview/", func(w http.ResponseWriter, r *http.Request) {
		got.path = "preview:" + r.URL.Path
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "page.json"), []byte(`{"title":"New"}`), 0o644))

	r := runRemote(t, dir, srv.URL, nil, "call", "GET", "/cms/api/pages/4?depth=0", "--json")
	doc, _ := r.JSON["body"].(map[string]any)
	if r.Code != 0 || r.JSON["status"] != float64(200) || doc == nil || r.JSON["request_id"] != "01REQ" {
		t.Fatalf("GET: %+v", r)
	}
	if got.auth != "Bearer whsk_agent_test" || got.method != "GET" || got.path != "/app/cms/api/pages/4?depth=0" || got.ct != "" {
		t.Errorf("GET sent %+v", got)
	}
	if strings.Contains(r.Stdout, "whsk_agent_test") || strings.Contains(r.Stderr, "whsk_agent_test") {
		t.Errorf("the credential was printed: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "call", "patch", "/cms/api/pages/4", "--data", "@page.json")
	if r.Code != 0 || got.method != "PATCH" || got.body != `{"title":"New"}` || got.ct != "application/json" || !strings.Contains(r.Stdout, `"title": "Home"`) {
		t.Errorf("PATCH: %+v, sent %+v", r, got)
	}
	r = runRemoteIn(t, `{"title":"Piped"}`, dir, srv.URL, "call", "PATCH", "/cms/api/pages/4", "--data", "-", "--json")
	if r.Code != 0 || got.body != `{"title":"Piped"}` {
		t.Errorf("PATCH from stdin: %+v, sent %+v", r, got)
	}

	r = runRemote(t, dir, srv.URL, nil, "call", "PATCH", "/refused", "--data", "{}", "--json")
	if r.Code != 4 || errCode(r) != "AGENT_CATEGORY" {
		t.Errorf("the edge's refusal: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "call", "POST", "/bad", "--data", "{}", "--json")
	if r.Code != 1 || errCode(r) != "CALL_FAILED" || !strings.Contains(r.Stdout, "title is required") {
		t.Errorf("the app's refusal: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "call", "GET", "/moved", "--json")
	if errCode(r) != "CALL_FAILED" || !strings.Contains(r.Stdout, `"location":"/elsewhere"`) {
		t.Errorf("a redirect: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "call", "GET", "/slow", "--timeout", "100ms", "--json")
	if r.Code != 5 || errCode(r) != "PLATFORM_UNAVAILABLE" || !strings.Contains(r.Stdout, "had no answer within 100ms") {
		t.Errorf("past the deadline: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "call", "GET", "/x", "--env", "draft", "--json")
	if r.Code != 0 || got.path != "preview:/preview/x" {
		t.Errorf("--env: %+v, %q", r, got.path)
	}
	r = runRemote(t, dir, srv.URL, nil, "call", "GET", "/x", "--env", "nope", "--json")
	if errCode(r) != "NOT_FOUND" {
		t.Errorf("an unknown environment: %+v", r)
	}

	before := got.calls
	for _, args := range [][]string{
		{"call", "GET", "https://evil.example/x"},
		{"call", "FETCH", "/x"},
		{"call", "POST", "/x", "--data", "{not json"},
		{"call", "POST", "/x", "--data", "@missing.json"},
		{"call", "GET", "/x", "--header", "Authorization: Bearer other"},
		{"call", "GET", "/x", "--timeout", "0s"},
	} {
		if r := runRemote(t, dir, srv.URL, nil, append(args, "--json")...); errCode(r) != "INVALID_REQUEST" {
			t.Errorf("%v: %+v", args, r)
		}
	}
	if got.calls != before {
		t.Errorf("a refused call reached the app")
	}

	// A login bound to this computer, or a token that is not an agent's, is refused locally.
	bound := &memStore{m: map[string]config.Credential{"default": {Token: "whsk_agent_login", PrivateKey: "k"}}}
	r = runCLI(t, dir, map[string]string{"WHISK_API": srv.URL}, bound, "call", "GET", "/cms/api/pages/4", "--json")
	if r.Code != 4 || errCode(r) != "TOKEN_SCOPE" {
		t.Errorf("a bound login: %+v", r)
	}
	r = runCLI(t, dir, map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_deploy_x"}, &memStore{m: map[string]config.Credential{}}, "call", "GET", "/cms/api/pages/4", "--json")
	if r.Code != 4 || errCode(r) != "TOKEN_SCOPE" {
		t.Errorf("a deploy token: %+v", r)
	}
	if got.calls != before {
		t.Errorf("a refused credential reached the app")
	}
}
