package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

type memStore struct{ m map[string]config.Credential }

func (s *memStore) Get(p string) (config.Credential, bool, error) { c, ok := s.m[p]; return c, ok, nil }
func (s *memStore) Set(p string, c config.Credential) error       { s.m[p] = c; return nil }
func (s *memStore) Delete(p string) error                         { delete(s.m, p); return nil }
func (s *memStore) Where() string                                 { return "memory" }

type result struct {
	Code   int
	Stdout string
	Stderr string
	JSON   map[string]any
}

func runCLI(t *testing.T, dir string, env map[string]string, store config.Store, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	e := Env{Stdout: &out, Stderr: &errb, Dir: dir, ConfigDir: t.TempDir(), Store: store, Getenv: func(k string) string { return env[k] }}
	code := Run(context.Background(), args, e)
	r := result{Code: code, Stdout: out.String(), Stderr: errb.String()}
	if strings.HasPrefix(strings.TrimSpace(r.Stdout), "{") {
		_ = json.Unmarshal([]byte(strings.SplitN(r.Stdout, "\n", 2)[0]), &r.JSON)
	}
	return r
}

func TestVersionAndAgentHelpers(t *testing.T) {
	dir := t.TempDir()
	r := runCLI(t, dir, nil, &memStore{}, "version", "--json")
	if r.Code != 0 || r.JSON["whisk_cli"] != "dev" || r.JSON["api"] != "v1" {
		t.Fatalf("version: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "errors", "NEEDS_HUMAN", "--json")
	if r.Code != 0 || r.JSON["code"] != "NEEDS_HUMAN" || r.JSON["fix"] == "" {
		t.Fatalf("errors: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "errors", "NOPE", "--json")
	if r.Code != 1 || r.JSON["error"] == nil {
		t.Fatalf("unknown code: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "schema")
	if r.Code != 0 || !strings.Contains(r.Stdout, `"$schema"`) {
		t.Fatalf("schema: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "skill")
	if r.Code != 0 || !strings.Contains(r.Stdout, "whisk") {
		t.Fatalf("skill: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "webhooks", "presets", "--json")
	if r.Code != 0 || r.JSON["presets"] == nil {
		t.Fatalf("presets: %+v", r)
	}
}

func TestCronListAndDoctor(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte("whisk: 1\nname: crm\nfunctions:\n  - name: nightly\n    cron: \"0 6 * * *\"\n    tz: Australia/Sydney\n    graph: workflows/nightly.graph.yaml\n"), 0o644))
	r := runCLI(t, dir, nil, &memStore{}, "cron", "list", "--json")
	if r.Code != 0 {
		t.Fatalf("cron list: %+v", r)
	}
	fns := r.JSON["functions"].([]any)
	if len(fns) != 1 || fns[0].(map[string]any)["tz"] != "Australia/Sydney" || len(fns[0].(map[string]any)["next"].([]any)) != 3 {
		t.Fatalf("cron list: %v", fns)
	}
	r = runCLI(t, dir, nil, &memStore{}, "doctor", "--json")
	if r.Code != 3 {
		t.Fatalf("doctor on a broken repo should exit 3: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "doctor")
	if r.Code != 3 || !strings.Contains(r.Stdout, "W040") || !strings.Contains(r.Stderr, "DOCTOR_FAILED") {
		t.Fatalf("doctor human output: %+v", r)
	}
}

func TestNotLoggedIn(t *testing.T) {
	r := runCLI(t, t.TempDir(), nil, &memStore{m: map[string]config.Credential{}}, "whoami", "--json")
	if r.Code != 4 {
		t.Fatalf("whoami without a token should exit 4: %+v", r)
	}
	if e := r.JSON["error"].(map[string]any); e["code"] != "AUTH_REQUIRED" {
		t.Fatalf("error: %v", e)
	}
}

// fakeAPI is enough of the control plane for login, whoami, use, init and apps.
func fakeAPI(t *testing.T) (*httptest.Server, *int32) {
	var polls int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/device/code", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"device_code": "dc-1", "user_code": "ABCD-EFGH", "verification_url": "https://whisk.run/device", "interval": 1, "expires_in": 60})
	})
	mux.HandleFunc("POST /v1/device/token", func(w http.ResponseWriter, r *http.Request) {
		// A poll must carry no idempotency key: the platform would replay the first "pending"
		// answer for the life of the code and the login would never finish.
		if r.Header.Get("Idempotency-Key") != "" {
			t.Errorf("the device poll sent an Idempotency-Key")
		}
		if atomic.AddInt32(&polls, 1) < 2 {
			writeJSON(w, 202, map[string]any{"status": "pending"})
			return
		}
		writeJSON(w, 200, map[string]any{"status": "approved", "token": "whsk_agent_test", "expires_at": "2026-10-07T00:00:00Z", "scopes": []string{"org:1", "deploy"},
			"org": map[string]any{"id": "01J", "slug": "acme", "name": "Acme", "plan": "team", "status": "active"}, "user": map[string]any{"id": "u1", "email": "ana@acme.example", "name": "Ana"}})
	})
	mux.HandleFunc("GET /v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer whsk_agent_test" {
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "AUTH_REQUIRED", "message": "no", "fix": "login"}})
			return
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "whisk-cli/dev (") || !strings.Contains(r.Header.Get("User-Agent"), "agent=claude") {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		writeJSON(w, 200, map[string]any{"user": map[string]any{"id": "u1", "email": "ana@acme.example", "name": "Ana"}, "orgs": []map[string]any{{"id": "01J", "slug": "acme", "role": "owner", "plan": "team", "status": "active"}}, "token": map[string]any{"kind": "agent", "scopes": []string{"deploy"}, "expires_at": "2026-10-07T00:00:00Z"}})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("mutating call without Idempotency-Key")
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, 201, map[string]any{"id": "a1", "org_id": "01J", "slug": body["slug"], "name": body["name"], "hostname": body["slug"] + "--acme.whisk.page", "region": "eu", "status": "active", "git_url": "https://git.whisk.run/acme/" + body["slug"] + ".git", "created_at": "2026-09-07T00:00:00Z"})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "a1", "slug": "crm", "hostname": "crm--acme.whisk.page", "status": "active", "region": "eu", "created_at": "2026-09-07T00:00:00Z"})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"slug": "crm", "hostname": "crm--acme.whisk.page", "status": "active", "region": "eu"}}, "next_cursor": ""})
	})
	mux.HandleFunc("DELETE /v1/tokens/self", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": r.URL.Path + " is not a resource", "fix": "check the path"}})
	})
	return httptest.NewServer(mux), &polls
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestLoginWhoamiUseInitApps(t *testing.T) {
	srv, _ := fakeAPI(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_AGENT": "claude"}
	dir := t.TempDir()

	r := runCLI(t, dir, env, store, "login", "--no-wait", "--json")
	if r.Code != 2 || r.JSON["error"].(map[string]any)["code"] != "NEEDS_HUMAN" {
		t.Fatalf("login --no-wait: %+v", r)
	}
	details := r.JSON["error"].(map[string]any)["details"].(map[string]any)
	if details["user_code"] != "ABCD-EFGH" || details["device_code"] != "dc-1" {
		t.Fatalf("details: %v", details)
	}
	r = runCLI(t, dir, env, store, "login", "--resume", "dc-1", "--json")
	if r.Code != 0 || store.m["default"].Token != "whsk_agent_test" || store.m["default"].Org != "acme" {
		t.Fatalf("login --resume: %+v store %+v", r, store.m)
	}
	r = runCLI(t, dir, env, store, "whoami", "--json")
	if r.Code != 0 || r.JSON["user"].(map[string]any)["email"] != "ana@acme.example" {
		t.Fatalf("whoami: %+v", r)
	}
	r = runCLI(t, dir, env, store, "use", "acme/crm", "--json")
	if r.Code != 0 || r.JSON["verified"] != true {
		t.Fatalf("use: %+v", r)
	}
	if b, ok, _ := config.LoadBinding(dir); !ok || b.App != "crm" || b.API != srv.URL {
		t.Fatalf("binding: %+v %v", b, ok)
	}
	r = runCLI(t, dir, env, store, "apps", "list", "--json")
	if r.Code != 0 || len(r.JSON["apps"].([]any)) != 1 {
		t.Fatalf("apps list: %+v", r)
	}
	r = runCLI(t, dir, env, store, "apps", "delete", "crm", "--json")
	if r.Code != 2 {
		t.Fatalf("apps delete without --yes should need a human: %+v", r)
	}

	fresh := filepath.Join(t.TempDir(), "my-crm")
	must(t, os.MkdirAll(fresh, 0o755))
	r = runCLI(t, fresh, env, store, "init", "--template", "ts", "--json")
	if r.Code != 0 || r.JSON["created"] != true || r.JSON["hostname"] != "my-crm--acme.whisk.page" {
		t.Fatalf("init --template: %+v", r)
	}
	if b, ok, _ := config.LoadBinding(fresh); !ok || b.App != "my-crm" || b.Org != "acme" {
		t.Fatalf("init binding: %+v %v", b, ok)
	}
	m, _ := os.ReadFile(filepath.Join(fresh, "whisk.yaml"))
	if !strings.Contains(string(m), "name: my-crm\n") {
		t.Fatalf("template manifest not renamed:\n%s", m)
	}
	r = runCLI(t, fresh, env, store, "init", "--json")
	if r.Code != 0 || r.JSON["created"] != false || !strings.Contains(r.Stdout, `"kept":["whisk.yaml"]`) {
		t.Fatalf("second init should keep files and the binding: %+v", r)
	}

	existing := filepath.Join(t.TempDir(), "legacy_app")
	must(t, os.MkdirAll(existing, 0o755))
	must(t, os.WriteFile(filepath.Join(existing, "package.json"), []byte(`{"dependencies":{"express":"4","prisma":"5"}}`), 0o644))
	must(t, os.WriteFile(filepath.Join(existing, ".gitignore"), []byte("node_modules\n"), 0o644))
	r = runCLI(t, existing, env, store, "init", "--no-create", "--json")
	if r.Code != 0 || r.JSON["name"] != "legacy-app" {
		t.Fatalf("init existing: %+v", r)
	}
	m, _ = os.ReadFile(filepath.Join(existing, "whisk.yaml"))
	gi, _ := os.ReadFile(filepath.Join(existing, ".gitignore"))
	if !strings.Contains(string(m), `migrate: "npx prisma migrate deploy"`) || !strings.Contains(string(gi), ".whisk/dev/") || !strings.HasPrefix(string(gi), "node_modules\n") {
		t.Fatalf("init existing files:\n%s\n%s", m, gi)
	}

	r = runCLI(t, dir, env, store, "logout", "--json")
	if r.Code != 0 || r.JSON["revoked"] != true || len(store.m) != 0 {
		t.Fatalf("logout: %+v", r)
	}
}

func TestTokenFromEnvironment(t *testing.T) {
	srv, _ := fakeAPI(t)
	defer srv.Close()
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude"}, &memStore{m: map[string]config.Credential{}}, "whoami", "--json")
	if r.Code != 0 || r.JSON["token_source"] != "WHISK_TOKEN" {
		t.Fatalf("whoami via WHISK_TOKEN: %+v", r)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// An unknown flag is an error for the human, not a crash: the printer has its streams before
// anything is parsed.
func TestUnknownFlagIsAnErrorNotACrash(t *testing.T) {
	r := runCLI(t, t.TempDir(), nil, nil, "--version")
	if r.Code != 1 || !strings.Contains(r.Stderr, "UNKNOWN_COMMAND") || !strings.Contains(r.Stderr, "unknown flag") || !strings.Contains(r.Stderr, "whisk update") {
		t.Errorf("code %d, stderr %q", r.Code, r.Stderr)
	}
}

// A command this build lacks points at whisk update, and --json still gets one JSON object.
func TestUnknownCommandPointsAtUpdate(t *testing.T) {
	r := runCLI(t, t.TempDir(), nil, nil, "nosuch", "url", "--json")
	var body struct {
		Error struct {
			Code, Message, Fix string
		}
	}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil {
		t.Fatalf("stdout is not JSON: %q", r.Stdout)
	}
	if r.Code != 1 || body.Error.Code != "UNKNOWN_COMMAND" || !strings.Contains(body.Error.Message, `unknown command "nosuch"`) || !strings.Contains(body.Error.Fix, "whisk update") {
		t.Errorf("code %d, body %+v", r.Code, body)
	}
	if r.Stderr != "" {
		t.Errorf("stderr %q", r.Stderr)
	}
}

// A stored token goes only to the API that issued it: WHISK_API pointing elsewhere (an
// environment file in a cloned repository, say) gets AUTH_REQUIRED and no request at all.
func TestStoredTokenStaysWithItsAPI(t *testing.T) {
	var hits int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	defer elsewhere.Close()
	store := &memStore{m: map[string]config.Credential{"default": {Token: "whsk_agent_stored", API: "https://api.whisk.run"}}}
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": elsewhere.URL}, store, "whoami", "--json")
	if r.Code == 0 || r.JSON["error"] == nil || r.JSON["error"].(map[string]any)["code"] != "AUTH_REQUIRED" {
		t.Fatalf("whoami with WHISK_API elsewhere: %+v", r)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("the stored token was sent to %s", elsewhere.URL)
	}
	srv, _ := fakeAPI(t)
	defer srv.Close()
	store.m["default"] = config.Credential{Token: "whsk_agent_test", API: srv.URL + "/"}
	if r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_AGENT": "claude"}, store, "whoami", "--json"); r.Code != 0 {
		t.Fatalf("whoami with WHISK_API matching the token's API: %+v", r)
	}
}
