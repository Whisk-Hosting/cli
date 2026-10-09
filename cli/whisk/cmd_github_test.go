package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

// githubState is what the fake GitHub routes remember between calls.
type githubState struct {
	linked   bool
	linkBody map[string]any
	synced   int
	unlinked int
	install  map[string]any
	// agent makes PUT answer NEEDS_HUMAN, as the platform answers an agent's credential.
	agent bool
}

var githubLinkJSON = map[string]any{
	"repo": "acme-co/crm", "html_url": "https://github.com/acme-co/crm", "status": "active",
	"synced_at": "2026-10-01T09:00:00Z", "created_at": "2026-09-30T09:00:00Z",
	"branches": []map[string]any{
		{"name": "main", "whisk": "abc1234abc", "github": "abc1234abc", "state": "in_sync"},
		{"name": "feature-x", "whisk": "def5678def", "github": "0123456789", "state": "waiting"},
		{"name": "spike", "whisk": "def5678def", "github": "fedcba9876", "state": "refused"},
	},
}

// githubAPI answers the GitHub routes of CONTROL-PLANE.md §6.26 for acme/crm.
func githubAPI(t *testing.T, st *githubState) *httptest.Server {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer whsk_agent_test" {
				writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "AUTH_REQUIRED", "message": "no token", "fix": "login"}})
				return
			}
			if missingKey(r) {
				t.Errorf("%s %s without Idempotency-Key", r.Method, r.URL.Path)
			}
			h(w, r)
		}
	}
	notLinked := func(w http.ResponseWriter) {
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "A GitHub link for this app does not exist.", "fix": "Check the name."}})
	}
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/github", auth(func(w http.ResponseWriter, r *http.Request) {
		if !st.linked {
			notLinked(w)
			return
		}
		writeJSON(w, 200, githubLinkJSON)
	}))
	mux.HandleFunc("PUT /v1/orgs/acme/apps/crm/github", auth(func(w http.ResponseWriter, r *http.Request) {
		if st.agent {
			url := "https://whisk.run/o/acme/apps/crm/settings"
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "NEEDS_HUMAN", "message": "Linking an app to GitHub is done by a person, signed in to Whisk and GitHub.",
				"fix": "Ask an owner or admin to open " + url + " and do it under GitHub there.", "details": map[string]any{"url": url}}})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&st.linkBody)
		st.linked = true
		writeJSON(w, 200, githubLinkJSON)
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/github/sync", auth(func(w http.ResponseWriter, r *http.Request) {
		if !st.linked {
			notLinked(w)
			return
		}
		st.synced++
		writeJSON(w, 202, map[string]any{"queued": true})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/apps/crm/github", auth(func(w http.ResponseWriter, r *http.Request) {
		if !st.linked {
			notLinked(w)
			return
		}
		st.linked = false
		st.unlinked++
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /v1/orgs/acme/github", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"set_up": true, "installations": []map[string]any{{
			"id": 7, "account": "acme-co", "account_type": "Organization", "settings_url": "https://github.com/organizations/acme-co/settings/installations/7",
			"repositories": []map[string]any{
				{"id": 1, "full_name": "acme-co/crm", "private": true, "html_url": "https://github.com/acme-co/crm", "linked_app": "crm"},
				{"id": 2, "full_name": "acme-co/site", "private": false, "html_url": "https://github.com/acme-co/site"},
			},
		}}})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/github/installations", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&st.install)
		writeJSON(w, 200, map[string]any{"install_url": "https://github.com/apps/whisk/installations/new?state=s1"})
	}))
	return httptest.NewServer(mux)
}

func TestGitHubStatus(t *testing.T) {
	st := &githubState{}
	srv := githubAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "github", "--json")
	if r.Code != 0 || r.JSON["linked"] != false || !strings.HasSuffix(r.JSON["settings_url"].(string), "/o/acme/apps/crm/settings") {
		t.Fatalf("github not linked json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "github", "status")
	if r.Code != 0 || !strings.Contains(r.Stdout, "not linked to GitHub") || !strings.Contains(r.Stdout, "whisk github link owner/name") {
		t.Fatalf("github status not linked: %+v", r)
	}

	st.linked = true
	r = runRemote(t, dir, srv.URL, nil, "github")
	for _, want := range []string{"acme-co/crm", "https://github.com/acme-co/crm", "copying both ways", "in step", "catching up", "refused", "def5678"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("github status lacks %q:\n%s", want, r.Stdout)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "github", "status", "--json")
	gh, _ := r.JSON["github"].(map[string]any)
	if r.Code != 0 || r.JSON["linked"] != true || gh["repo"] != "acme-co/crm" || len(gh["branches"].([]any)) != 3 {
		t.Fatalf("github status json: %+v", r)
	}
}

func TestGitHubLink(t *testing.T) {
	st := &githubState{}
	srv := githubAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "github", "link", "not-a-repo", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "INVALID_REQUEST" {
		t.Fatalf("link bad repo: %+v", r)
	}

	st.agent = true
	r = runRemote(t, dir, srv.URL, nil, "github", "link", "acme-co/crm", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || e["details"].(map[string]any)["url"] != "https://whisk.run/o/acme/apps/crm/settings" || st.linked {
		t.Fatalf("link as an agent: %+v", r)
	}

	st.agent = false
	r = runRemote(t, dir, srv.URL, nil, "github", "link", "https://github.com/acme-co/crm.git")
	if r.Code != 0 || st.linkBody["repo"] != "acme-co/crm" || !strings.Contains(r.Stdout, "Linked crm to acme-co/crm") {
		t.Fatalf("link: %+v body %v", r, st.linkBody)
	}
}

func TestGitHubReposSyncUnlinkInstall(t *testing.T) {
	st := &githubState{linked: true}
	srv := githubAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "github", "repos")
	for _, want := range []string{"acme-co (organization)", "acme-co/crm", "private", "crm", "acme-co/site", "public", "settings/installations/7"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("github repos lacks %q:\n%s", want, r.Stdout)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "github", "repos", "--json")
	ins, _ := r.JSON["installations"].([]any)
	if r.Code != 0 || len(ins) != 1 || len(ins[0].(map[string]any)["repositories"].([]any)) != 2 {
		t.Fatalf("github repos json: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "github", "sync", "--json")
	if r.Code != 0 || r.JSON["queued"] != true || st.synced != 1 {
		t.Fatalf("github sync: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "github", "unlink")
	if r.Code != 0 || st.unlinked != 1 || !strings.Contains(r.Stdout, "Nothing changed on GitHub or in the app's code") {
		t.Fatalf("github unlink: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "github", "sync", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("sync after unlink: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "github", "install", "--json")
	if r.Code != 0 || r.JSON["install_url"] != "https://github.com/apps/whisk/installations/new?state=s1" || st.install["app"] != "crm" {
		t.Fatalf("github install: %+v body %v", r, st.install)
	}
}

func TestGitHubPassesNotSetUpThrough(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/github", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "GITHUB_NOT_SET_UP", "message": "Copying to GitHub is not available yet.", "fix": "Ask the operator to set up Whisk's GitHub App."}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	r := runRemote(t, boundDir(t, srv.URL), srv.URL, nil, "github", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "GITHUB_NOT_SET_UP" {
		t.Fatalf("not set up: %+v", r)
	}
}

func TestGitHubWords(t *testing.T) {
	cases := []struct {
		link api.GitHubLink
		want string
	}{
		{api.GitHubLink{Status: "active"}, "copying both ways"},
		{api.GitHubLink{Status: "broken", Problem: "GitHub no longer lets Whisk reach acme-co/crm."}, "broken: GitHub no longer lets Whisk reach acme-co/crm."},
		{api.GitHubLink{Status: "broken"}, "broken"},
	}
	for _, c := range cases {
		if got := githubStatusText(c.link); got != c.want {
			t.Errorf("githubStatusText(%+v) = %q, want %q", c.link, got, c.want)
		}
	}
	for state, want := range map[string]string{"in_sync": "in step", "waiting": "catching up", "refused": "refused", "": "-"} {
		if got := githubBranchText(state); got != want {
			t.Errorf("githubBranchText(%q) = %q, want %q", state, got, want)
		}
	}
	repos := map[string]string{
		"acme-co/crm":                        "acme-co/crm",
		"https://github.com/acme-co/crm":     "acme-co/crm",
		"https://github.com/acme-co/crm.git": "acme-co/crm",
		"git@github.com:acme-co/crm.git":     "acme-co/crm",
		"github.com/acme-co/crm/":            "acme-co/crm",
		"crm":                                "",
		"acme-co/crm/tree/main":              "",
		"/crm":                               "",
	}
	for in, want := range repos {
		got, ok := normalizeRepo(in)
		if ok != (want != "") || got != want {
			t.Errorf("normalizeRepo(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}
