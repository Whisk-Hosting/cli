package whisk

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whisk-run/contract/tokensig"

	"github.com/whisk-run/cli/internal/config"
)

// boundAPI is the control plane's half of device-bound tokens: it keeps the public key whisk
// login sends, binds the token to it, and answers every request with that token only when the
// request is signed by the matching key, as authenticate does (CONTROL-PLANE.md §4.4).
type boundAPI struct {
	mu        sync.Mutex
	publicKey string
	device    map[string]string
	// allOrgs approves logins for the whole account, which come back with no org; orgs are
	// the slugs whoami lists.
	allOrgs bool
	orgs    []string
	listed  []string
}

const boundToken = "whsk_agent_bound"

func (b *boundAPI) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/device/code", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["public_key"] == "" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "no key", "fix": "Run whisk update"}})
			return
		}
		b.mu.Lock()
		b.publicKey, b.device = in["public_key"], in
		b.mu.Unlock()
		writeJSON(w, 200, map[string]any{"device_code": "dc-b", "user_code": "BBBB-CCCC", "verification_url": "https://whisk.run/device", "interval": 1, "expires_in": 60})
	})
	mux.HandleFunc("POST /v1/device/token", func(w http.ResponseWriter, r *http.Request) {
		if b.allOrgs {
			writeJSON(w, 200, map[string]any{"status": "approved", "token": boundToken, "expires_at": "2026-11-02T00:00:00Z", "scopes": []string{"orgs:*", "apps:*", "deploy"},
				"user": map[string]any{"id": "u1", "email": "ana@acme.example"}})
			return
		}
		writeJSON(w, 200, map[string]any{"status": "approved", "token": boundToken, "expires_at": "2026-11-02T00:00:00Z", "scopes": []string{"org:1", "deploy"},
			"org": map[string]any{"id": "01J", "slug": "acme"}, "user": map[string]any{"id": "u1", "email": "ana@acme.example"}})
	})
	signed := func(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			b.mu.Lock()
			pk := b.publicKey
			b.mu.Unlock()
			pub, err := tokensig.ParsePublic(pk)
			if err != nil || r.Header.Get("Authorization") != "Bearer "+boundToken ||
				tokensig.Verify(pub, r.Header.Get(tokensig.Header), r.Method, r.URL.RequestURI(), body, boundToken, time.Now()) != nil {
				writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "TOKEN_SIGNATURE", "message": "not signed by this token's computer", "fix": "Run whisk login again on this computer."}})
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			h(w, r)
		}
	}
	mux.HandleFunc("GET /v1/whoami", signed(func(w http.ResponseWriter, r *http.Request) {
		orgs := []any{}
		for _, o := range b.orgs {
			orgs = append(orgs, map[string]any{"slug": o})
		}
		writeJSON(w, 200, map[string]any{"user": map[string]any{"email": "ana@acme.example"}, "orgs": orgs,
			"token": map[string]any{"kind": "agent", "scopes": []string{"deploy"}, "bound": true, "device_name": "ana-laptop"}})
	}))
	mux.HandleFunc("GET /v1/orgs/{org}/apps", signed(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.listed = append(b.listed, r.PathValue("org"))
		b.mu.Unlock()
		writeJSON(w, 200, map[string]any{"items": []any{}})
	}))
	mux.HandleFunc("POST /v1/tokens/git", signed(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "" {
			t.Error("the git password request carried an Idempotency-Key; its value would rest in the replay store")
		}
		writeJSON(w, 200, map[string]any{"username": "whisk", "password": "whsk_git_short", "expires_at": "2026-10-03T00:10:00Z"})
	}))
	return httptest.NewServer(mux)
}

// whisk login makes a key pair, sends the public key with the computer's name and system, and
// keeps the private key with the token. Requests are signed with it; the same token without the
// key, from WHISK_TOKEN or with another computer's key, is refused.
func TestLoginBindsTheTokenToThisComputer(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL}

	r := runCLI(t, t.TempDir(), env, store, "login", "--wait", "--json")
	if r.Code != 0 || r.JSON["bound"] != true {
		t.Fatalf("login: %+v", r)
	}
	cred := store.m["default"]
	key, err := tokensig.ParsePrivate(cred.PrivateKey)
	if err != nil {
		t.Fatalf("no private key stored with the token: %v", err)
	}
	if tokensig.EncodePublic(key.Public().(ed25519.PublicKey)) != b.publicKey {
		t.Fatal("the stored private key does not match the public key sent")
	}
	if b.device["os"] == "" || !strings.Contains(b.device["os"], "/") {
		t.Fatalf("the system was not sent as os/arch: %v", b.device)
	}
	if b.device["agent"] != "" {
		t.Fatalf("an agent name was sent though none was given: %v", b.device)
	}

	r = runCLI(t, t.TempDir(), env, store, "whoami", "--json")
	if r.Code != 0 || r.JSON["bound_to_this_device"] != true {
		t.Fatalf("whoami with the stored key: %+v", r)
	}

	stolen := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": boundToken}, &memStore{m: map[string]config.Credential{}}, "whoami", "--json")
	if stolen.Code == 0 || stolen.JSON["error"].(map[string]any)["code"] != "TOKEN_SIGNATURE" {
		t.Fatalf("the token alone was accepted: %+v", stolen)
	}

	_, otherKey, _ := tokensig.NewKey()
	copied := &memStore{m: map[string]config.Credential{"default": {Token: boundToken, API: srv.URL, PrivateKey: otherKey}}}
	r = runCLI(t, t.TempDir(), env, copied, "whoami", "--json")
	if r.Code == 0 || r.JSON["error"].(map[string]any)["code"] != "TOKEN_SIGNATURE" {
		t.Fatalf("the token with another computer's key was accepted: %+v", r)
	}
}

// git cannot sign, so whisk git-credential trades the bound token for a short git password
// through a signed request and hands git that, never the token.
func TestGitCredentialTradesABoundToken(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL}
	if r := runCLI(t, t.TempDir(), env, store, "login", "--wait", "--json"); r.Code != 0 {
		t.Fatalf("login: %+v", r)
	}
	var out, errb bytes.Buffer
	e := Env{Stdin: strings.NewReader("protocol=https\nhost=git.whisk.run\n\n"), Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: t.TempDir(), Store: store,
		Getenv: func(k string) string { return env[k] }}
	if code := Run(context.Background(), []string{"git-credential", "get"}, e); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if out.String() != "username=whisk\npassword=whsk_git_short\n" {
		t.Fatalf("answered %q", out.String())
	}

	out.Reset()
	e.Stdin = strings.NewReader("protocol=http\nhost=git.whisk.run\n\n")
	if code := Run(context.Background(), []string{"git-credential", "get"}, e); code != 0 || out.String() != "" {
		t.Fatalf("answered plain http: %d %q", code, out.String())
	}
}

// --resume collects a token only on the computer that kept the key for that code.
func TestResumeNeedsTheKeptKey(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL}
	r := runCLI(t, t.TempDir(), env, store, "login", "--resume", "dc-b", "--json")
	if r.Code == 0 || r.JSON["error"].(map[string]any)["code"] != "INVALID_REQUEST" {
		t.Fatalf("resume without a kept key: %+v", r)
	}
	if r := runCLI(t, t.TempDir(), env, store, "login", "--no-wait", "--json"); r.Code != 2 {
		t.Fatalf("login --no-wait: %+v", r)
	}
	if r := runCLI(t, t.TempDir(), env, store, "login", "--resume", "dc-other", "--json"); r.Code == 0 {
		t.Fatalf("resume of another code: %+v", r)
	}
	r = runCLI(t, t.TempDir(), env, store, "login", "--resume", "dc-b", "--json")
	if r.Code != 0 || store.m["default"].PrivateKey == "" {
		t.Fatalf("resume: %+v %+v", r, store.m)
	}
	if _, waiting := store.m[config.PendingProfile("default")]; waiting {
		t.Fatal("the waiting key was kept after the token was collected")
	}
	if r := runCLI(t, t.TempDir(), env, store, "whoami", "--json"); r.Code != 0 {
		t.Fatalf("whoami after resume: %+v", r)
	}
}

// Off a terminal (an agent's shell, whisk mcp) plain login does not wait: it answers at once
// with the URL, the code and the device code in its one JSON object, keeps the key and the
// code's expiry, and --resume collects the token afterwards.
func TestLoginOffATerminalHandsBackTheCode(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL}
	r := runCLI(t, t.TempDir(), env, store, "login", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	details, _ := e["details"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || details["user_code"] != "BBBB-CCCC" || details["url"] != "https://whisk.run/device" || details["device_code"] != "dc-b" {
		t.Fatalf("login off a terminal: %+v", r)
	}
	if strings.Count(strings.TrimSpace(r.Stdout), "\n") != 0 {
		t.Fatalf("more than one JSON object on stdout: %q", r.Stdout)
	}
	pending, ok := store.m[config.PendingProfile("default")]
	if !ok || pending.PrivateKey == "" || pending.ExpiresAt.IsZero() {
		t.Fatalf("the waiting key or its expiry was not kept: %+v", pending)
	}
	r = runCLI(t, t.TempDir(), env, store, "login", "--resume", "dc-b", "--json")
	if r.Code != 0 || r.JSON["bound"] != true || !strings.Contains(r.Stderr, "Waiting for the human") {
		t.Fatalf("resume: %+v", r)
	}
}

// --wait waits off a terminal, and says what it waits for on stderr even with --json.
func TestLoginWaitSaysWhatItWaitsFor(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, store, "login", "--wait", "--json")
	if r.Code != 0 || !strings.Contains(r.Stderr, "BBBB-CCCC") || !strings.Contains(r.Stderr, "--resume dc-b") {
		t.Fatalf("login --wait: %+v", r)
	}
	if _, waiting := store.m[config.PendingProfile("default")]; waiting {
		t.Fatal("the waiting key was kept after the token was collected")
	}
}

// The coding agent names itself with --agent, or WHISK_AGENT, and the name goes with the code
// request so the approval page shows it.
func TestLoginSendsTheAgentsName(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_AGENT": "Codex"}
	r := runCLI(t, t.TempDir(), env, &memStore{m: map[string]config.Credential{}}, "login", "--wait", "--agent", "Claude Code", "--json")
	if r.Code != 0 || b.device["agent"] != "Claude Code" {
		t.Fatalf("login --agent sent %v: %+v", b.device, r)
	}
}

func TestAgentName(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		flag string
		env  map[string]string
		want string
	}{
		{"Claude Code", map[string]string{"WHISK_AGENT": "Codex"}, "Claude Code"},
		{"  ", map[string]string{"WHISK_AGENT": " Codex "}, "Codex"},
		{"", map[string]string{"CLAUDECODE": "1"}, "Claude Code"},
		{"", map[string]string{"WHISK_AGENT": "Cursor", "CLAUDECODE": "1"}, "Cursor"},
		{"", map[string]string{}, ""},
	}
	for _, c := range cases {
		if got := agentName(c.flag, env(c.env)); got != c.want {
			t.Errorf("agentName(%q, %v) = %q, want %q", c.flag, c.env, got, c.want)
		}
	}
}

func TestLoginWaits(t *testing.T) {
	cases := []struct {
		terminal, noWait, wait, want bool
	}{
		{true, false, false, true},
		{false, false, false, false},
		{false, false, true, true},
		{true, true, false, false},
		{true, true, true, false},
	}
	for _, c := range cases {
		if got := loginWaits(c.terminal, c.noWait, c.wait); got != c.want {
			t.Errorf("loginWaits(%v, %v, %v) = %v", c.terminal, c.noWait, c.wait, got)
		}
	}
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	if got := resumeDeadline(time.Time{}, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Errorf("no kept expiry: %v", got)
	}
	if kept := now.Add(3 * time.Minute); !resumeDeadline(kept, now).Equal(kept) {
		t.Error("the kept expiry was not used")
	}
}

// A login approved for all the person's businesses stores no org: a command that names none
// acts on their only org, and with several asks for --org, naming them.
func TestLoginForTheWholeAccount(t *testing.T) {
	b := &boundAPI{allOrgs: true, orgs: []string{"acme"}}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL}

	r := runCLI(t, t.TempDir(), env, store, "login", "--wait", "--json")
	if r.Code != 0 || r.JSON["all_orgs"] != true || r.JSON["org"] != nil || store.m["default"].Org != "" {
		t.Fatalf("login: %+v %+v", r, store.m["default"])
	}
	if r := runCLI(t, t.TempDir(), env, store, "apps", "list", "--json"); r.Code != 0 || len(b.listed) != 1 || b.listed[0] != "acme" {
		t.Fatalf("apps list with one org: %+v %v", r, b.listed)
	}
	if r := runCLI(t, t.TempDir(), env, store, "apps", "list", "--org", "other", "--json"); r.Code != 0 || b.listed[1] != "other" {
		t.Fatalf("apps list --org: %+v %v", r, b.listed)
	}

	b.orgs = []string{"acme", "sober"}
	r = runCLI(t, t.TempDir(), env, store, "apps", "list", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code == 0 || e["code"] != "INVALID_REQUEST" || !strings.Contains(e["message"].(string), "acme, sober") {
		t.Fatalf("apps list with two orgs: %+v", r)
	}
}
