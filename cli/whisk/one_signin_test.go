package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

// One sign-in per computer (CLI.md §2): a login for the whole account signs out every other
// sign-in for the same platform, revoking each on the platform, keeps the ones for another
// platform, and becomes the profile commands use when none is named.
func TestLoginForTheWholeAccountReplacesTheOthers(t *testing.T) {
	b := &boundAPI{allOrgs: true, orgs: []string{"acme"}}
	inner := b.server(t)
	defer inner.Close()
	var mu sync.Mutex
	revoked := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/tokens/self" {
			mu.Lock()
			revoked = append(revoked, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{
		"default":      {Token: "whsk_old_default", API: srv.URL, Org: "northwind"},
		"contoso":      {Token: "whsk_old_contoso", API: srv.URL, Org: "contoso"},
		"staging":      {Token: "whsk_staging", API: "https://api.staging.example"},
		"jane":         {Token: "whsk_old_jane", API: srv.URL + "/"},
		"jane:pending": {API: srv.URL, PrivateKey: "x"},
	}}
	cfgDir := t.TempDir()
	run := func(args ...string) result {
		t.Helper()
		var out, errb bytes.Buffer
		e := Env{Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: cfgDir, Store: store, Getenv: func(k string) string { return map[string]string{"WHISK_API": srv.URL}[k] }}
		code := Run(context.Background(), args, e)
		r := result{Code: code, Stdout: out.String(), Stderr: errb.String()}
		if strings.HasPrefix(strings.TrimSpace(r.Stdout), "{") {
			_ = json.Unmarshal([]byte(strings.SplitN(r.Stdout, "\n", 2)[0]), &r.JSON)
		}
		return r
	}

	r := run("login", "--profile", "jane", "--wait", "--json")
	if r.Code != 0 || r.JSON["all_orgs"] != true {
		t.Fatalf("login: %+v", r)
	}
	gone, _ := r.JSON["signed_out"].([]any)
	if len(gone) != 2 || gone[0] != "contoso" || gone[1] != "default" {
		t.Fatalf("signed out %v, want contoso and default", gone)
	}
	slices.Sort(revoked)
	if want := []string{"whsk_old_contoso", "whsk_old_default", "whsk_old_jane"}; !slices.Equal(revoked, want) {
		t.Fatalf("revoked %v, want %v", revoked, want)
	}
	names, _ := store.List()
	if want := []string{"jane", "staging"}; !slices.Equal(names, want) {
		t.Fatalf("profiles left %v, want %v", names, want)
	}
	cfg, err := config.Load(cfgDir)
	if err != nil || cfg.Profile != "jane" {
		t.Fatalf("the computer's profile is %q (%v), want jane", cfg.Profile, err)
	}
	if r := run("apps", "list", "--json"); r.Code != 0 {
		t.Fatalf("a command naming no profile uses the new sign-in: %+v", r)
	}
}

// A narrower login replaces only its own profile's sign-in.
func TestLoginForOneBusinessKeepsTheOthers(t *testing.T) {
	b := &boundAPI{}
	srv := b.server(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{"agency": {Token: "whsk_agency", API: srv.URL}}}
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, store, "login", "--wait", "--json")
	if gone, _ := r.JSON["signed_out"].([]any); r.Code != 0 || len(gone) != 0 || store.m["agency"].Token != "whsk_agency" {
		t.Fatalf("login: %+v", r)
	}
}

func TestReplacedBy(t *testing.T) {
	held := map[string]config.Credential{
		"default": {Token: "a", API: "https://api.whisk.run"},
		"other":   {Token: "b", API: "https://API.whisk.run/"},
		"dev":     {Token: "c", API: "http://localhost:8080"},
		"empty":   {API: "https://api.whisk.run"},
	}
	cases := []struct {
		name    string
		allOrgs bool
		current string
		want    []string
	}{
		{"whole account", true, "default", []string{"other"}},
		{"whole account into a new profile", true, "new", []string{"default", "other"}},
		{"one business", false, "default", []string{}},
	}
	for _, c := range cases {
		if got := replacedBy(c.allOrgs, c.current, "https://api.whisk.run", held); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// whoami and orgs name each business the person belongs to that the login does not cover, with
// what lets the agent in.
func TestWhoamiNamesBusinessesNotInTheLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"user": map[string]any{"email": "ana@acme.example"}, "orgs": []any{map[string]any{"slug": "acme"}},
			"token":        map[string]any{"kind": "agent", "scopes": []string{"orgs:*"}},
			"not_in_login": []any{map[string]any{"slug": "fabrikam", "name": "Fabrikam", "url": "https://whisk.run/me/logins/01J?add=fabrikam", "fix": "Ask the person to open https://whisk.run/me/logins/01J?add=fabrikam and tap Add Fabrikam."}}})
	}))
	defer srv.Close()
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_t"}
	store := &memStore{m: map[string]config.Credential{}}
	for _, cmd := range []string{"whoami", "orgs"} {
		r := runCLI(t, t.TempDir(), env, store, cmd, "--json")
		gaps, _ := r.JSON["not_in_login"].([]any)
		if r.Code != 0 || len(gaps) != 1 || gaps[0].(map[string]any)["url"] != "https://whisk.run/me/logins/01J?add=fabrikam" {
			t.Fatalf("%s --json: %+v", cmd, r)
		}
		r = runCLI(t, t.TempDir(), env, store, cmd)
		if r.Code != 0 || !strings.Contains(r.Stdout, "Not in this login:") || !strings.Contains(r.Stdout, "fabrikam (Fabrikam): Ask the person to open https://whisk.run/me/logins/01J?add=fabrikam") {
			t.Fatalf("%s: %+v", cmd, r)
		}
	}
}
