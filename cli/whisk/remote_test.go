package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
)

// fakeGit answers the git calls whisk deploy makes without a repository. It records every
// call and environment so tests can assert how the token travelled.
type fakeGit struct {
	repo      bool     // rev-parse --show-toplevel succeeds
	dirty     []string // git status lines
	sha       string
	branch    string
	remoteURL string // "" means the remote whisk does not exist yet
	push      gitcmd.Result
	// pushSchannel answers a push made with -c http.sslBackend=schannel when set.
	pushSchannel *gitcmd.Result
	calls        [][]string
	envs         [][]string
}

func (f *fakeGit) Run(ctx context.Context, dir string, env []string, args ...string) (gitcmd.Result, error) {
	f.calls = append(f.calls, args)
	f.envs = append(f.envs, env)
	// Skip -c key=value pairs to find the subcommand.
	sub := args
	for len(sub) >= 2 && sub[0] == "-c" {
		sub = sub[2:]
	}
	ok := gitcmd.Result{}
	fail := gitcmd.Result{ExitCode: 1}
	switch strings.Join(sub, " ") {
	case "rev-parse --show-toplevel":
		if f.repo {
			return gitcmd.Result{Stdout: dir + "\n"}, nil
		}
		return gitcmd.Result{ExitCode: 128, Stderr: "fatal: not a git repository"}, nil
	case "init --quiet":
		f.repo = true
		return ok, nil
	case "status --porcelain --untracked-files=all":
		lines := make([]string, len(f.dirty))
		for i, p := range f.dirty {
			lines[i] = "?? " + p
		}
		return gitcmd.Result{Stdout: strings.Join(lines, "\n")}, nil
	case "add --all -- :(top) :(top,exclude,glob)**/.whisk/dev/**":
		return ok, nil
	case "config user.email":
		return fail, nil
	case "commit --quiet -m whisk deploy", "commit --quiet -m Add a CSV export to the jobs page":
		f.dirty = nil
		return ok, nil
	case "rev-parse --verify --quiet HEAD":
		if f.sha == "" {
			return fail, nil
		}
		return gitcmd.Result{Stdout: f.sha + "\n"}, nil
	case "symbolic-ref --short --quiet HEAD":
		if f.branch == "" {
			return fail, nil
		}
		return gitcmd.Result{Stdout: f.branch + "\n"}, nil
	case "remote get-url whisk":
		if f.remoteURL == "" {
			return gitcmd.Result{ExitCode: 2, Stderr: "error: No such remote 'whisk'"}, nil
		}
		return gitcmd.Result{Stdout: f.remoteURL + "\n"}, nil
	}
	switch {
	case len(sub) >= 3 && sub[0] == "remote" && (sub[1] == "add" || sub[1] == "set-url"):
		f.remoteURL = sub[3]
		return ok, nil
	case len(sub) >= 1 && sub[0] == "push":
		if f.pushSchannel != nil && strings.Contains(strings.Join(args, " "), "http.sslBackend=schannel") {
			return *f.pushSchannel, nil
		}
		return f.push, nil
	}
	return gitcmd.Result{ExitCode: 1, Stderr: "fake git: unexpected " + strings.Join(args, " ")}, nil
}

func (f *fakeGit) pushCall() ([]string, []string) {
	for i, c := range f.calls {
		for _, a := range c {
			if a == "push" {
				return c, f.envs[i]
			}
		}
	}
	return nil, nil
}

func (f *fakeGit) ran(sub string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(strings.Join(c, " "), sub) {
			return true
		}
		// Also match past the -c key=value pairs, as Run does, so "commit" finds
		// "-c user.name=… commit …" and a caller may name either form.
		args := c
		for len(args) >= 2 && args[0] == "-c" {
			args = args[2:]
		}
		if strings.HasPrefix(strings.Join(args, " "), sub) {
			return true
		}
	}
	return false
}

// remoteState is what the fake control plane remembers between calls.
type remoteState struct {
	unset       []string
	scanPolls   int
	deploys     []map[string]any
	posted      []map[string]any // POST deploys bodies
	putGrants   []map[string]any
	declared    []map[string]any
	invited     []map[string]any
	deletedSecs []string
	failGets    int              // GETs of d-fail: the first sees it still building
	events      []map[string]any // POST events bodies
	previews    []string         // POST previews branches
}

func sse(w http.ResponseWriter, events ...map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	f, _ := w.(http.Flusher)
	for _, ev := range events {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f != nil {
			f.Flush()
		}
	}
}

func remoteAPI(t *testing.T, st *remoteState) *httptest.Server {
	mux := http.NewServeMux()
	deploy := func(id, status, sha string, extra map[string]any) map[string]any {
		d := map[string]any{"id": id, "app_id": "a1", "environment": "production", "build_id": "b1", "commit_sha": sha, "branch": "main", "status": status,
			"phases": []map[string]any{{"phase": "queued", "at": "2026-09-08T10:00:00Z"}}, "triggered_by": "agent:t1", "triggered_by_label": "Claude Code for Ana", "created_at": "2026-09-08T10:00:00Z"}
		for k, v := range extra {
			d[k] = v
		}
		return d
	}
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
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "a1", "org_id": "01J", "slug": "crm", "hostname": "crm--acme.whisk.page", "url": "https://crm--acme.whisk.page", "state": "running", "status": "active", "region": "eu",
			"git_url": "https://git.whisk.run/01J/a1.git", "current_deploy_id": "d-live", "unset_secrets": st.unset, "created_at": "2026-09-07T00:00:00Z"})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": st.deploys, "next_cursor": ""})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/validate", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"valid": true, "problems": []any{}, "plan": "free", "limits": map[string]any{"repo_bytes": 100 << 20}, "unavailable": []string{}})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch id {
		case "d-live":
			writeJSON(w, 200, deploy(id, "live", "abc1234abc", nil))
		case "d-fail":
			st.failGets++
			if st.failGets == 1 {
				writeJSON(w, 200, deploy(id, "building", "abc1234abc", nil))
				return
			}
			writeJSON(w, 200, deploy(id, "failed", "abc1234abc", map[string]any{"error": map[string]any{"code": "BUILD_FAILED", "message": "The build failed at step 5 of 9 (npm ci).", "fix": "Read the log excerpt, fix the cause, and run whisk deploy again.", "docs": "https://skill.whisk.run/errors/BUILD_FAILED", "details": map[string]any{"build_id": "b1", "step": "npm ci"}}}))
		case "d-new", "d-rb", "d-polled":
			writeJSON(w, 200, deploy(id, "queued", "abc1234abc", nil))
		case "d-preview":
			writeJSON(w, 200, deploy(id, "queued", "abc1234abc", map[string]any{"environment": "preview:feature-x", "branch": "feature-x"}))
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That deploy does not exist.", "fix": "whisk deploys list"}})
		}
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/previews", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.previews = append(st.previews, body["branch"])
		if body["branch"] == "too-many" {
			writeJSON(w, 402, map[string]any{"error": map[string]any{"code": "PLAN_LIMIT_PREVIEWS", "message": "crm already has 1 preview, the most the Free plan allows for one app.", "fix": "Remove one with whisk envs delete preview:<branch>.", "details": map[string]any{"limit": 1, "plan": "free", "previews": []string{"preview:feature-x"}}}})
			return
		}
		writeJSON(w, 202, map[string]any{
			"environment": map[string]any{"id": "e2", "app_id": "a1", "name": "preview:" + body["branch"], "hostname": "crm--acme--" + body["branch"] + ".whisk.page", "url": "https://crm--acme--" + body["branch"] + ".whisk.page", "status": "active", "created_at": "2026-09-08T00:00:00Z"},
			"deploy":      deploy("d-preview", "queued", "abc1234abc", map[string]any{"environment": "preview:" + body["branch"], "branch": body["branch"]}),
		})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys/{id}/events", auth(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		switch id {
		case "d-new", "d-rb":
			sse(w,
				map[string]any{"deploy_id": id, "status": "starting", "phase": map[string]any{"phase": "starting", "at": "2026-09-08T10:00:01Z"}, "done": false},
				map[string]any{"deploy_id": id, "status": "health_checking", "phase": map[string]any{"phase": "health_checking", "at": "2026-09-08T10:00:05Z"}, "done": false},
				map[string]any{"deploy_id": id, "status": "live", "phase": map[string]any{"phase": "live", "at": "2026-09-08T10:00:09Z"}, "done": true, "url": "https://crm--acme.whisk.page", "unset_secrets": st.unset, "start_ms": 7312},
			)
		case "d-preview":
			sse(w, map[string]any{"deploy_id": id, "status": "live", "phase": map[string]any{"phase": "live", "at": "2026-09-08T10:00:09Z"}, "done": true, "url": "https://crm--acme--feature-x.whisk.page", "unset_secrets": []string{}})
		case "d-old":
			sse(w, map[string]any{"deploy_id": id, "status": "superseded", "phase": map[string]any{"phase": "superseded", "at": "2026-09-08T10:00:03Z"}, "done": true})
		case "d-fail":
			sse(w,
				map[string]any{"deploy_id": id, "status": "building", "phase": map[string]any{"phase": "building", "at": "2026-09-08T10:00:01Z"}, "done": false},
				map[string]any{"deploy_id": id, "status": "failed", "phase": map[string]any{"phase": "failed", "at": "2026-09-08T10:00:20Z", "detail": "npm ci"}, "done": true,
					"error": map[string]any{"code": "BUILD_FAILED", "message": "The build failed at step 5 of 9 (npm ci).", "fix": "Read the log excerpt, fix the cause, and run whisk deploy again.", "docs": "https://skill.whisk.run/errors/BUILD_FAILED", "details": map[string]any{"build_id": "b1", "step": "npm ci"}}},
			)
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "no", "fix": "no"}})
		}
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/builds/b1/log", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 1; i <= 50; i++ {
			fmt.Fprintf(w, "line %d\n", i)
		}
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/deploys", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.posted = append(st.posted, body)
		writeJSON(w, 202, deploy("d-rb", "queued", "abc1234abc", map[string]any{"previous_deploy_id": "d-live"}))
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/events", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.events = append(st.events, body)
		writeJSON(w, 202, map[string]any{"id": fmt.Sprintf("ev-%d", len(st.events))})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/deploys/{id}/cancel", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, deploy(r.PathValue("id"), "cancelled", "abc1234abc", nil))
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/environments", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "e1", "name": "production", "hostname": "crm--acme.whisk.page", "url": "https://crm--acme.whisk.page", "has_database": true, "created_at": "2026-09-07T00:00:00Z"},
			{"id": "e2", "name": "preview:feature-x", "hostname": "crm--acme--feature-x.whisk.page", "url": "https://crm--acme--feature-x.whisk.page", "expires_at": "2026-09-15T00:00:00Z", "created_at": "2026-09-08T00:00:00Z"}}})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/apps/crm/environments/{env}", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	secret := map[string]any{"id": "s1", "name": "STRIPE_SECRET_KEY", "scope": "app", "app_id": "a1", "set": false, "version": 0, "previews": true, "consumers": []string{"a1"}, "created_at": "2026-09-07T00:00:00Z"}
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/secrets", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{secret}})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/secrets", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{}})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/secrets", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, has := body["value"]; has {
			t.Error("the CLI sent a secret value")
		}
		st.declared = append(st.declared, body)
		writeJSON(w, 201, map[string]any{"id": "s2", "name": body["name"], "scope": body["scope"], "app_id": body["app_id"], "set": false, "version": 0, "consumers": []string{}, "created_at": "2026-09-08T00:00:00Z"})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/secrets/{name}/versions", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("app") != "crm" {
			t.Errorf("versions without app=crm: %s", r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"version": 2, "length": 32, "created_by": "ana@acme.example", "created_at": "2026-09-08T00:00:00Z"}, {"version": 1, "length": 32, "created_by": "ana@acme.example", "created_at": "2026-09-07T00:00:00Z"}}})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/secrets/{name}/rollback", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["version"] != float64(1) || body["app_id"] != "crm" {
			t.Errorf("rollback body %v", body)
		}
		writeJSON(w, 200, map[string]any{"id": "s1", "name": r.PathValue("name"), "scope": "app", "set": true, "version": 3, "consumers": []string{"a1"}, "created_at": "2026-09-07T00:00:00Z"})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/secrets/{name}/share", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["app_id"] != "sales" || len(body) != 1 || r.URL.RawQuery != "" {
			t.Errorf("share body %v query %q", body, r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"secret": map[string]any{"id": "s3", "name": r.PathValue("name"), "scope": "org", "set": true, "version": 1, "consumers": []string{"a1", "a2"}, "created_at": "2026-09-07T00:00:00Z"}, "apps": []string{"crm"}})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/secrets/{name}/reads", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "" || r.URL.Query().Get("app") != "crm" {
			t.Errorf("reads query %s", r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"version": 2, "read_by_kind": "container", "read_by_id": "c1", "read_at": "2026-09-08T09:00:00Z"}}})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/secrets/{name}", auth(func(w http.ResponseWriter, r *http.Request) {
		st.deletedSecs = append(st.deletedSecs, r.PathValue("name")+"?"+r.URL.RawQuery)
		w.WriteHeader(204)
	}))

	domain := map[string]any{"id": "dom1", "hostname": "jobs.acme.example", "kind": "custom", "verified": false, "cert_status": "pending", "txt_record": "_whisk-verify.jobs.acme.example", "txt_value": "whisk-verify-9f3c", "cname_target": "crm--acme.whisk.page", "addresses": []string{"95.217.38.236"}, "created_at": "2026-09-08T00:00:00Z"}
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/domains", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{domain}})
	}))
	orgDomain := map[string]any{"id": "od1", "domain": "acme.example", "verified": false, "txt_record": "_whisk-verify.acme.example", "txt_value": "whisk-verify-77aa", "cname_record": "*.acme.example", "cname_target": "domains.whisk.page", "addresses": []string{"95.217.38.236"}, "apps": []any{}, "created_at": "2026-10-02T00:00:00Z"}
	mux.HandleFunc("GET /v1/orgs/acme/domains", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{orgDomain}})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/domains", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["domain"] != "acme.example" {
			t.Errorf("org domain body %v", body)
		}
		writeJSON(w, 201, orgDomain)
	}))
	mux.HandleFunc("POST /v1/orgs/acme/domains/od1/verify", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "od1", "domain": "acme.example", "verified": true, "apps": []map[string]string{{"app": "crm", "hostname": "crm.acme.example", "url": "https://crm.acme.example"}}, "created_at": "2026-10-02T00:00:00Z"})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/domains/od1", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/domains", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["hostname"] != "jobs.acme.example" {
			t.Errorf("domain body %v", body)
		}
		writeJSON(w, 201, domain)
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/domains/dom1/verify", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "DOMAIN_UNVERIFIED", "message": "The DNS records for jobs.acme.example are not in place yet.", "fix": "Add the records and verify again.", "details": map[string]any{"hostname": "jobs.acme.example", "missing": []string{"TXT"}}}})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/apps/crm/domains/dom1", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	members := []map[string]any{
		{"id": "m1", "user_id": "u1", "email": "ana@acme.example", "name": "Ana", "role": "owner", "kind": "member", "status": "active", "groups": []string{"g1"}, "created_at": "2026-09-07T00:00:00Z"},
		{"id": "m2", "user_id": "u2", "email": "bob@acme.example", "name": "Bob", "role": "developer", "kind": "member", "status": "active", "groups": []string{}, "created_at": "2026-09-07T00:00:00Z"},
	}
	mux.HandleFunc("GET /v1/orgs/acme/members", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": members})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/members", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.invited = append(st.invited, body)
		writeJSON(w, 201, map[string]any{"id": "m3", "user_id": "u3", "email": body["email"], "role": body["role"], "kind": body["kind"], "status": "invited", "groups": []string{}, "invite_url": "https://whisk.run/invite/tok123", "created_at": "2026-09-08T00:00:00Z"})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/members/m2", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	mux.HandleFunc("GET /v1/orgs/acme/groups", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "g1", "name": "finance", "members": []string{"u1"}, "created_at": "2026-09-07T00:00:00Z"}}})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/access", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"app_id": "a1", "grants": []map[string]any{{"subject_kind": "everyone", "audience": "team", "label": "Everyone in the org"}}})
	}))
	mux.HandleFunc("PUT /v1/orgs/acme/apps/crm/access", auth(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		grants, _ := body["grants"].([]any)
		for _, g := range grants {
			st.putGrants = append(st.putGrants, g.(map[string]any))
		}
		writeJSON(w, 200, map[string]any{"app_id": "a1", "grants": grants})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/deploy-keys", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 201, map[string]any{"token": map[string]any{"id": "t9", "kind": "deploy", "label": "ci", "scopes": []string{"app:a1", "deploy"}, "created_at": "2026-09-08T00:00:00Z"}, "value": "whsk_deploy_ONCE", "git_url": "https://git.whisk.run/01J/a1.git"})
	}))
	finding := map[string]any{"id": "CVE-2020-8203", "aliases": []string{"GHSA-p6mc-m468-83gw"}, "package": "lodash", "ecosystem": "npm", "version": "4.17.15", "fixed": "4.17.19", "severity": "high", "where": "source", "path": "package-lock.json", "tool": "osv-scanner"}
	scan := map[string]any{"id": "ps1", "status": "done", "trigger": "deploy", "commit_sha": "abc1234def", "counts": map[string]int{"high": 1, "low": 1, "fixable": 1, "attention": 1, "not_called": 1},
		"findings": []map[string]any{finding, {"id": "CVE-2023-42366", "package": "busybox", "version": "1.35.0-r17", "severity": "low", "where": "image", "tool": "trivy", "reach": "unknown"},
			{"id": "CVE-2022-32149", "package": "golang.org/x/text", "version": "0.3.0", "fixed": "0.3.8", "severity": "high", "where": "source", "path": "go.mod", "tool": "osv-scanner", "reach": "not_called"}},
		"created_at": "2026-10-05T09:00:00Z", "finished_at": "2026-10-05T09:01:00Z"}
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/packages", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"included": true, "scan": scan})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/packages/scan", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, map[string]any{"id": "ps2", "status": "queued", "trigger": "request", "counts": map[string]int{}, "created_at": "2026-10-05T10:00:00Z"})
	}))
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/packages/scans/ps2", auth(func(w http.ResponseWriter, r *http.Request) {
		st.scanPolls++
		if st.scanPolls < 2 {
			writeJSON(w, 200, map[string]any{"id": "ps2", "status": "running", "trigger": "request", "counts": map[string]int{}, "created_at": "2026-10-05T10:00:00Z"})
			return
		}
		writeJSON(w, 200, scan)
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": r.URL.Path, "fix": "check the path"}})
	})
	return httptest.NewServer(mux)
}

// boundDir is a directory bound to acme/crm.
func boundDir(t *testing.T, api string) string {
	dir := t.TempDir()
	must(t, config.SaveBinding(dir, config.Binding{Org: "acme", App: "crm", API: api}))
	return dir
}

func runRemote(t *testing.T, dir, api string, git gitcmd.Runner, args ...string) result {
	t.Helper()
	return runRemoteOn(t, "", dir, api, git, args...)
}

// runRemoteOn is runRemote with the CLI behaving as the operating system goos.
func runRemoteOn(t *testing.T, goos, dir, api string, git gitcmd.Runner, args ...string) result {
	t.Helper()
	return runRemoteWith(t, goos, "", dir, api, git, args...)
}

// runRemoteIn is runRemote with stdin.
func runRemoteIn(t *testing.T, stdin, dir, api string, args ...string) result {
	t.Helper()
	return runRemoteWith(t, "", stdin, dir, api, nil, args...)
}

func runRemoteWith(t *testing.T, goos, stdin, dir, api string, git gitcmd.Runner, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"WHISK_API": api, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude"}
	e := Env{Stdout: &out, Stderr: &errb, Stdin: strings.NewReader(stdin), Dir: dir, ConfigDir: t.TempDir(), Store: &memStore{m: map[string]config.Credential{}}, Git: git, GOOS: goos, Getenv: func(k string) string { return env[k] }}
	code := Run(context.Background(), args, e)
	r := result{Code: code, Stdout: out.String(), Stderr: errb.String()}
	lines := jsonLines(r.Stdout)
	if len(lines) > 0 {
		r.JSON = lines[len(lines)-1]
	}
	return r
}

func jsonLines(s string) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "{") {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func pushOK(deployID string) gitcmd.Result {
	return gitcmd.Result{Stdout: "To https://git.whisk.run/01J/a1.git\n*\tmain:refs/heads/main\t[new branch]\nDone\n", Stderr: "remote: whisk: scanning 1 commit\nremote: whisk: deploy " + deployID + "\n"}
}

func TestDeployFreshDirectoryToLiveWithUnsetSecrets(t *testing.T) {
	st := &remoteState{unset: []string{"STRIPE_SECRET_KEY", "SLACK_WEBHOOK_URL"}}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: false, dirty: []string{"whisk.yaml", "src/app.ts"}, sha: "abc1234abc", branch: "master", push: pushOK("d-new")}

	r := runRemote(t, dir, srv.URL, git, "deploy", "--force")
	if r.Code != 0 {
		t.Fatalf("deploy: %+v", r)
	}
	for _, sub := range []string{"init --quiet", "add --all -- :(top) :(top,exclude,glob)**/.whisk/dev/**", "-c user.name=whisk -c user.email=whisk@localhost commit --quiet -m whisk deploy", "remote add whisk https://git.whisk.run/01J/a1.git"} {
		if !git.ran(sub) {
			t.Errorf("git %s did not run; calls: %v", sub, git.calls)
		}
	}
	args, env := git.pushCall()
	if args == nil || strings.Join(args[len(args)-4:], " ") != "push --porcelain whisk master:refs/heads/main" {
		t.Fatalf("push args: %v", args)
	}
	if strings.Contains(strings.Join(args, " "), "whsk_agent_test") {
		t.Fatalf("token on the git command line: %v", args)
	}
	if !contains(env, "WHISK_GIT_TOKEN=whsk_agent_test") || !contains(args, "credential.helper=") {
		t.Fatalf("token not handed over through the environment: env %v args %v", env, args)
	}
	for _, c := range git.calls {
		if len(c) > 0 && c[0] == "config" && len(c) > 1 && c[1] != "user.email" {
			t.Fatalf("git config written: %v", c)
		}
	}
	dash := srv.URL + "/o/acme/apps/crm/secrets"
	if !strings.Contains(r.Stdout, "Live: https://crm--acme.whisk.page") || !strings.Contains(r.Stdout, "NEEDS_HUMAN: 2 secret(s) need a value") || !strings.Contains(r.Stdout, "STRIPE_SECRET_KEY, SLACK_WEBHOOK_URL") || !strings.Contains(r.Stdout, "  "+dash) {
		t.Fatalf("human output:\n%s\n%s", r.Stdout, r.Stderr)
	}
	if !strings.Contains(r.Stderr, "starting") || !strings.Contains(r.Stderr, "health_checking") {
		t.Fatalf("phases not shown on stderr:\n%s", r.Stderr)
	}
	// How long the app took to answer its health check, under the live line (CLI.md §5.4).
	if !strings.Contains(r.Stdout, "Live: https://crm--acme.whisk.page\nStarted in 7.3 s\n") {
		t.Fatalf("start time not shown under the live line:\n%s", r.Stdout)
	}

	// The same deploy as JSON: one event per line, then the summary.
	git2 := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("d-new")}
	r = runRemote(t, dir, srv.URL, git2, "deploy", "--force", "--json")
	if r.Code != 0 {
		t.Fatalf("deploy --json: %+v", r)
	}
	lines := jsonLines(r.Stdout)
	if len(lines) != 4 || lines[0]["phase"].(map[string]any)["phase"] != "starting" || lines[2]["done"] != true {
		t.Fatalf("json lines: %v", lines)
	}
	sum := lines[3]
	if sum["status"] != "live" || sum["url"] != "https://crm--acme.whisk.page" || sum["whisk_cli"] != "dev" || sum["deploy_id"] != "d-new" || sum["start_ms"] != 7312.0 {
		t.Fatalf("summary: %v", sum)
	}
	nh := sum["needs_human"].(map[string]any)
	if nh["code"] != "NEEDS_HUMAN" || nh["details"].(map[string]any)["url"] != dash+"?set=STRIPE_SECRET_KEY,SLACK_WEBHOOK_URL" || len(nh["details"].(map[string]any)["names"].([]any)) != 2 {
		t.Fatalf("needs_human: %v", nh)
	}
	if strings.TrimSpace(r.Stderr) != "" {
		t.Fatalf("stderr in json mode: %q", r.Stderr)
	}
	if git2.ran("init") || git2.ran("commit") || git2.ran("remote add") || git2.ran("remote set-url") {
		t.Fatalf("a clean bound repo should only push: %v", git2.calls)
	}
}

// A deploy a newer push replaced did not fail: the newer one carries the change on, so the CLI
// says so and exits 0 instead of telling the agent to fix a cause that does not exist.
func TestDeployReplacedByANewerOneIsNotAFailure(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("d-old")}

	r := runRemote(t, dir, srv.URL, git, "deploy", "--force")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Replaced by a newer deploy.") {
		t.Fatalf("deploy: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	lines := jsonLines(r.Stdout)
	if sum := lines[len(lines)-1]; r.Code != 0 || sum["status"] != "superseded" || sum["deploy_id"] != "d-old" {
		t.Fatalf("json summary: %+v %v", r, sum)
	}
}

// A deploy from a feature branch says it is a preview, that production is unchanged, and how
// to deploy production instead, at the push and again when it is live.
func TestDeployFromABranchSaysItIsAPreview(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "feature-x", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("d-preview")}

	r := runRemote(t, dir, srv.URL, git, "deploy", "--force")
	if r.Code != 0 {
		t.Fatalf("deploy: %+v", r)
	}
	note := "This is a preview of branch feature-x; production is unchanged. To put it live, merge feature-x into main and run whisk deploy from main."
	if !strings.Contains(r.Stderr, note) {
		t.Errorf("no preview note at the push:\n%s", r.Stderr)
	}
	if !strings.Contains(r.Stdout, "Preview live: https://crm--acme--feature-x.whisk.page") || !strings.Contains(r.Stdout, note) {
		t.Errorf("result does not say preview:\n%s", r.Stdout)
	}
	if strings.Contains(r.Stdout, "Started in") {
		t.Errorf("a preview records no start time, so none is shown:\n%s", r.Stdout)
	}

	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	lines := jsonLines(r.Stdout)
	sum := lines[len(lines)-1]
	if r.Code != 0 || sum["environment"] != "preview:feature-x" || sum["note"] != note {
		t.Fatalf("json summary: %v", sum)
	}

	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--no-wait", "--json")
	if r.Code != 0 || r.JSON["environment"] != "preview:feature-x" || r.JSON["note"] != note {
		t.Fatalf("no-wait: %+v", r.JSON)
	}
}

// A push only stores a branch, so a preview deploy starts the preview after the push
// (CONTROL-PLANE.md §6.3), and envs start does the same without a push.
func TestPreviewsStartOnRequest(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "feature-x", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("")}
	if r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--no-wait", "--json"); r.Code != 0 || r.JSON["deploy_id"] != "d-preview" {
		t.Fatalf("deploy: %+v", r)
	}
	if len(st.previews) != 1 || st.previews[0] != "feature-x" {
		t.Fatalf("previews started: %v", st.previews)
	}
	r := runRemote(t, dir, srv.URL, git, "envs", "start", "preview:fix-2", "--no-wait", "--json")
	if r.Code != 0 || r.JSON["environment"] != "preview:fix-2" || r.JSON["url"] != "https://crm--acme--fix-2.whisk.page" {
		t.Fatalf("envs start: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "envs", "start", "too-many", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code != 3 || e["code"] != "PLAN_LIMIT_PREVIEWS" {
		t.Fatalf("over the limit exits 3: %+v", r)
	}
}

func TestPreviewNote(t *testing.T) {
	if previewNote("production") != "" || previewNote("") != "" {
		t.Error("production has no note")
	}
	if !strings.Contains(previewNote("preview:fix-1"), "branch fix-1; production is unchanged") {
		t.Errorf("note = %q", previewNote("preview:fix-1"))
	}
}

func TestDeployBuildFailure(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("d-fail")}

	r := runRemote(t, dir, srv.URL, git, "deploy", "--force")
	if r.Code != 6 {
		t.Fatalf("failed deploy should exit 6: %+v", r)
	}
	if !strings.Contains(r.Stderr, "building") || !strings.Contains(r.Stderr, "failed  npm ci") {
		t.Fatalf("phases from the stream not shown:\n%s", r.Stderr)
	}
	st.failGets = 0
	if !strings.Contains(r.Stderr, "BUILD_FAILED: The build failed at step 5") || !strings.Contains(r.Stderr, "Read the log excerpt") || !strings.Contains(r.Stderr, "line 50") || strings.Contains(r.Stderr, "line 10\n") {
		t.Fatalf("failure output:\n%s", r.Stderr)
	}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	if r.Code != 6 {
		t.Fatalf("failed deploy --json: %+v", r)
	}
	e := r.JSON["error"].(map[string]any)
	if e["code"] != "BUILD_FAILED" || e["fix"] == "" {
		t.Fatalf("error: %v", e)
	}
	log := e["details"].(map[string]any)["log"].([]any)
	if len(log) != 40 || log[39] != "line 50" || log[0] != "line 11" {
		t.Fatalf("log excerpt: %d lines, %v…", len(log), log[:2])
	}
}

func TestDeployWithoutSidebandPollsForTheDeploy(t *testing.T) {
	st := &remoteState{deploys: []map[string]any{{"id": "d-polled", "commit_sha": "abc1234abc", "environment": "production", "status": "queued", "branch": "main", "created_at": "2026-09-08T10:00:00Z", "phases": []any{}}}}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git",
		push: gitcmd.Result{Stdout: "To x\n \tmain:refs/heads/main\t1111111..abc1234\nDone\n"}}
	r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--no-wait", "--json")
	if r.Code != 0 || r.JSON["deploy_id"] != "d-polled" || r.JSON["status"] != "queued" || r.JSON["environment"] != "production" {
		t.Fatalf("deploy --no-wait: %+v", r)
	}
}

func TestDeployUpToDateRedeploysTheCommit(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git",
		push: gitcmd.Result{Stdout: "To x\n=\tmain:refs/heads/main\t[up to date]\nDone\n"}}
	r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--no-wait", "--json")
	if r.Code != 0 || r.JSON["deploy_id"] != "d-rb" {
		t.Fatalf("up-to-date deploy: %+v", r)
	}
	if len(st.posted) != 1 || st.posted[0]["commit_sha"] != "abc1234abc" || st.posted[0]["environment"] != "production" {
		t.Fatalf("posted: %v", st.posted)
	}
}

func TestDeployPreReceiveRejection(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git",
		push: gitcmd.Result{ExitCode: 1, Stdout: "To x\n!\tmain:refs/heads/main\t[remote rejected] (pre-receive hook declined)\nDone\n",
			Stderr: "remote: whisk: SECRET_IN_COMMIT: A value that looks like a Stripe secret key is in src/config.ts line 12.\nremote: whisk: fix: Remove the value, declare STRIPE_SECRET_KEY under secrets in whisk.yaml, rewrite the commit.\nerror: failed to push some refs\n"}}
	r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	if r.Code != 3 {
		t.Fatalf("rejected push should exit 3: %+v", r)
	}
	e := r.JSON["error"].(map[string]any)
	if e["code"] != "SECRET_IN_COMMIT" || !strings.HasPrefix(e["fix"].(string), "Remove the value") || e["docs"] != "https://skill.whisk.run/errors/SECRET_IN_COMMIT" {
		t.Fatalf("error: %v", e)
	}

	git.push = gitcmd.Result{ExitCode: 128, Stderr: "fatal: unable to access 'https://git.whisk.run/': Could not resolve host: git.whisk.run\n"}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	if r.Code != 5 || r.JSON["error"].(map[string]any)["code"] != "PLATFORM_UNAVAILABLE" {
		t.Fatalf("network failure should exit 5: %+v", r)
	}
}

// A push stored but refused its deploy (post-receive printed the refusal) fails at once with
// that refusal, instead of polling 30 seconds for a deploy that never comes.
func TestDeployRefusedAfterThePushFailsAtOnce(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git",
		push: gitcmd.Result{Stdout: "To x\n \tmain:refs/heads/main\t1111111..abc1234\nDone\n",
			Stderr: "remote: whisk: FREE_APP_TAKEN: Acme Labs has no free app: each person and each work email domain gets one, and Acme Ltd already has it.\nremote: whisk: fix: Start the free Starter trial or choose a paid plan for Acme Labs on its billing page. Preview deployments remain available.\n"}}
	start := time.Now()
	r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("refusal took %s", took)
	}
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code == 0 || e["code"] != "FREE_APP_TAKEN" || !strings.HasPrefix(e["fix"].(string), "Start the free Starter trial") {
		t.Fatalf("refused deploy: %+v", r)
	}
}

func TestDeployCertificateUntrusted(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	untrusted := gitcmd.Result{ExitCode: 128, Stderr: "fatal: unable to access 'https://git.whisk.run/01J/a1.git/': SSL certificate problem: unable to get local issuer certificate\n"}
	newGit := func() *fakeGit {
		return &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: untrusted}
	}

	// Elsewhere than Windows there is no retry: the error names the host and is not "retry".
	git := newGit()
	r := runRemoteOn(t, "linux", dir, srv.URL, git, "deploy", "--force", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 1 || e["code"] != "GIT_TLS_UNTRUSTED" || e["details"].(map[string]any)["host"] != "git.whisk.run" {
		t.Fatalf("linux: %+v", r)
	}
	if git.ran("-c http.sslBackend=schannel") {
		t.Fatal("linux retried with schannel")
	}

	// On Windows a push that schannel can make succeeds, and says how to make it stick.
	git = newGit()
	ok := pushOK("d-new")
	git.pushSchannel = &ok
	r = runRemoteOn(t, "windows", dir, srv.URL, git, "deploy", "--force", "--no-wait")
	if r.Code != 0 || !strings.Contains(r.Stdout+r.Stderr, "d-new") || !strings.Contains(r.Stderr, "git config http.sslBackend schannel") {
		t.Fatalf("windows retry: %+v", r)
	}
	last := len(git.calls) - 1
	call, env := git.calls[last], git.envs[last]
	if !strings.Contains(strings.Join(call, " "), "http.sslBackend=schannel") || !strings.Contains(strings.Join(env, " "), gitcmd.TokenVar+"=") {
		t.Fatalf("schannel push: %v %v", call, env)
	}

	// On Windows, when schannel fails too, the fix is not schannel again.
	git = newGit()
	git.pushSchannel = &untrusted
	r = runRemoteOn(t, "windows", dir, srv.URL, git, "deploy", "--force", "--json")
	e, _ = r.JSON["error"].(map[string]any)
	if r.Code != 1 || e["code"] != "GIT_TLS_UNTRUSTED" || strings.Contains(e["fix"].(string), "sslBackend") {
		t.Fatalf("windows both fail: %+v", r)
	}
}

func TestDeployGatesOnDoctorAndUncommittedChanges(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte("whisk: 1\nname: crm\nfunctions:\n  - name: nightly\n    graph: workflows/nightly.graph.yaml\n"), 0o644))
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", push: pushOK("d-new")}
	r := runRemote(t, dir, srv.URL, git, "deploy", "--json")
	if r.Code != 3 || r.JSON["error"].(map[string]any)["code"] != "DOCTOR_FAILED" || len(git.calls) != 0 {
		t.Fatalf("doctor gate: %+v calls %v", r, git.calls)
	}

	git = &fakeGit{repo: true, dirty: []string{"src/app.ts"}, sha: "abc1234abc", branch: "main", push: pushOK("d-new")}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--json")
	if r.Code != 1 || git.ran("push") {
		t.Fatalf("uncommitted changes without --commit: %+v", r)
	}
	e := r.JSON["error"].(map[string]any)
	if e["code"] != "INVALID_REQUEST" || !strings.Contains(e["fix"].(string), "whisk deploy -m") || e["details"].(map[string]any)["uncommitted"].([]any)[0] != "src/app.ts" {
		t.Fatalf("error: %v", e)
	}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "--commit", "--no-wait", "--json")
	if r.Code != 0 || !git.ran("commit") || !git.ran("push") || committedWith(git) != "whisk deploy" {
		t.Fatalf("deploy --commit: %+v calls %v", r, git.calls)
	}

	// -m commits the changes with what changed, without --commit.
	git = &fakeGit{repo: true, dirty: []string{"src/app.ts"}, sha: "abc1234abc", branch: "main", push: pushOK("d-new")}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "-m", "  Add a CSV export to the jobs page ", "--no-wait", "--json")
	if r.Code != 0 || !git.ran("push") || committedWith(git) != "Add a CSV export to the jobs page" {
		t.Fatalf("deploy -m: %+v calls %v", r, git.calls)
	}
	// With nothing to commit, -m commits nothing and the last commit's message is shown.
	git = &fakeGit{repo: true, sha: "abc1234abc", branch: "main", push: pushOK("d-new")}
	r = runRemote(t, dir, srv.URL, git, "deploy", "--force", "-m", "Ignored", "--no-wait", "--json")
	if r.Code != 0 || git.ran("commit") || !git.ran("push") {
		t.Fatalf("deploy -m with nothing to commit: %+v calls %v", r, git.calls)
	}
}

// committedWith is the -m value of the fake git's commit call.
func committedWith(g *fakeGit) string {
	for _, c := range g.calls {
		for i, a := range c {
			if a == "commit" {
				for j := i; j+1 < len(c); j++ {
					if c[j] == "-m" {
						return c[j+1]
					}
				}
			}
		}
	}
	return ""
}

func TestClip(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"Show overdue invoices in red", 10, "Show over…"},
		{"ünïcödé wörds", 5, "ünïc…"},
	} {
		if got := clip(c.in, c.n); got != c.want {
			t.Errorf("clip(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestPlanPush(t *testing.T) {
	cases := []struct {
		env, branch, current string
		want                 pushPlan
		fails                bool
	}{
		{"", "", "main", pushPlan{"main", "main", "production"}, false},
		{"", "", "master", pushPlan{"master", "main", "production"}, false},
		{"", "", "", pushPlan{"HEAD", "main", "production"}, false},
		{"", "", "feature-x", pushPlan{"feature-x", "feature-x", "preview:feature-x"}, false},
		{"production", "", "feature-x", pushPlan{}, true},
		{"production", "feature-x", "main", pushPlan{}, true},
		{"production", "main", "feature-x", pushPlan{"main", "main", "production"}, false},
		{"production", "", "", pushPlan{"HEAD", "main", "production"}, false},
		{"preview", "", "main", pushPlan{}, true},
		{"preview", "", "", pushPlan{}, true},
		{"preview", "fix-1", "main", pushPlan{"fix-1", "fix-1", "preview:fix-1"}, false},
		{"staging", "", "main", pushPlan{}, true},
	}
	for _, c := range cases {
		got, err := planPush(c.env, c.branch, c.current)
		if e, ok := err.(*output.Error); ok && c.env == "production" && e.Code != "PRODUCTION_NEEDS_MAIN" {
			t.Errorf("planPush(%q,%q,%q) code %s, want PRODUCTION_NEEDS_MAIN", c.env, c.branch, c.current, e.Code)
		}
		if (err != nil) != c.fails || got != c.want {
			t.Errorf("planPush(%q,%q,%q) = %+v, %v; want %+v fails=%v", c.env, c.branch, c.current, got, err, c.want, c.fails)
		}
	}
}

func TestDashboardFrom(t *testing.T) {
	cases := map[string]string{
		"https://api.whisk.run":         "https://whisk.run",
		"https://api.whisk.run/":        "https://whisk.run",
		"https://api.staging.whisk.run": "https://staging.whisk.run",
		"http://localhost:8080":         "http://localhost:8080",
		"http://127.0.0.1:9999":         "http://127.0.0.1:9999",
	}
	for in, want := range cases {
		if got := dashboardFrom(in); got != want {
			t.Errorf("dashboardFrom(%q) = %q want %q", in, got, want)
		}
	}
}

func TestRollbackDeploysAndStatus(t *testing.T) {
	st := &remoteState{unset: []string{"XERO_CLIENT_SECRET"}}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{}

	r := runRemote(t, dir, srv.URL, git, "rollback", "--no-wait", "--json")
	if r.Code != 0 || r.JSON["deploy_id"] != "d-rb" {
		t.Fatalf("rollback: %+v", r)
	}
	if len(st.posted) != 1 || len(st.posted[0]) != 0 {
		t.Fatalf("rollback must post an empty body: %v", st.posted)
	}
	r = runRemote(t, dir, srv.URL, git, "rollback", "d-live", "--env", "preview:feature-x", "--json")
	if r.Code != 0 || r.JSON["status"] != "live" {
		t.Fatalf("rollback to id, followed: %+v", r)
	}
	if b := st.posted[1]; b["deploy_id"] != "d-live" || b["environment"] != "preview:feature-x" {
		t.Fatalf("rollback body: %v", b)
	}

	r = runRemote(t, dir, srv.URL, git, "deploys", "list", "--json")
	if r.Code != 0 || r.JSON["deploys"] == nil {
		t.Fatalf("deploys list: %+v", r)
	}
	st.failGets = 1 // the fake serves d-fail as still building on its first read (for the follow test)
	r = runRemote(t, dir, srv.URL, git, "deploys", "info", "d-fail")
	if r.Code != 0 || !strings.Contains(r.Stdout, "BUILD_FAILED") || !strings.Contains(r.Stdout, "Claude Code for Ana") {
		t.Fatalf("deploys info: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "deploys", "cancel", "d-rb", "--json")
	if r.Code != 0 || r.JSON["deploy"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("deploys cancel: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, git, "status", "--json")
	if r.Code != 0 || r.JSON["state"] != "running" || r.JSON["deploy"].(map[string]any)["id"] != "d-live" || r.JSON["needs_human"] == nil {
		t.Fatalf("status: %+v", r)
	}
	if pk, _ := r.JSON["packages"].(map[string]any); pk == nil || pk["counts"].(map[string]any)["attention"] != float64(1) {
		t.Errorf("status carries the package summary: %v", r.JSON["packages"])
	}
	r = runRemote(t, dir, srv.URL, git, "status")
	if r.Code != 0 || !strings.Contains(r.Stdout, "running") || !strings.Contains(r.Stdout, "by Claude Code for Ana") || !strings.Contains(r.Stdout, "NEEDS_HUMAN") {
		t.Fatalf("status human: %+v", r)
	}
	if !strings.Contains(r.Stdout, "packages: 1 known vulnerability to fix now; run whisk scan") {
		t.Errorf("status human names the packages to fix: %s", r.Stdout)
	}
	r = runRemote(t, dir, srv.URL, git, "open", "app", "--json")
	if r.Code != 0 || r.JSON["url"] != "https://crm--acme.whisk.page" {
		t.Fatalf("open app: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "open", "--json")
	if r.Code != 0 || r.JSON["url"] != srv.URL+"/o/acme/apps/crm" {
		t.Fatalf("open dashboard: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "open", "nope", "--json")
	if r.Code != 1 {
		t.Fatalf("open nope: %+v", r)
	}
}

func TestSecretsCommands(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{}
	appURL := srv.URL + "/o/acme/apps/crm/secrets"

	r := runRemote(t, dir, srv.URL, git, "secrets", "set", "STRIPE_SECRET_KEY", "--json")
	if r.Code != 2 {
		t.Fatalf("secrets set must exit 2: %+v", r)
	}
	e := r.JSON["error"].(map[string]any)
	if e["code"] != "NEEDS_HUMAN" || e["details"].(map[string]any)["url"] != appURL+"?set=STRIPE_SECRET_KEY" || e["details"].(map[string]any)["name"] != "STRIPE_SECRET_KEY" {
		t.Fatalf("set error: %v", e)
	}
	// Several names get one link that opens the paste form with a line for each.
	r = runRemote(t, dir, srv.URL, git, "secrets", "set", "STRIPE_SECRET_KEY", "XERO_CLIENT_SECRET", "--json")
	e = r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["details"].(map[string]any)["url"] != appURL+"?set=STRIPE_SECRET_KEY,XERO_CLIENT_SECRET" || len(e["details"].(map[string]any)["names"].([]any)) != 2 || !strings.Contains(e["message"].(string), "need values") {
		t.Fatalf("set several: %v", e)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "set", "STRIPE_SECRET_KEY")
	if r.Code != 2 || !strings.Contains(r.Stderr, "NEEDS_HUMAN") || !strings.Contains(r.Stderr, appURL) {
		t.Fatalf("set human: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "set", "SHARED", "--scope", "org", "--json")
	if r.Code != 2 || r.JSON["error"].(map[string]any)["details"].(map[string]any)["url"] != srv.URL+"/o/acme/secrets?set=SHARED" {
		t.Fatalf("set --scope org: %+v", r)
	}
	// Sharing with previews is a person's too: the block names the page and what to turn on or off.
	r = runRemote(t, dir, srv.URL, git, "secrets", "set", "STRIPE_SECRET_KEY", "--previews", "--json")
	e = r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || e["details"].(map[string]any)["url"] != appURL || e["details"].(map[string]any)["previews"] != true || !strings.Contains(e["fix"].(string), "turn Share with previews on for STRIPE_SECRET_KEY") {
		t.Fatalf("set --previews: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "set", "STRIPE_SECRET_KEY", "XERO_CLIENT_SECRET", "--previews=false", "--json")
	e = r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["details"].(map[string]any)["previews"] != false || !strings.Contains(e["fix"].(string), "Share with previews off for STRIPE_SECRET_KEY, XERO_CLIENT_SECRET") || !strings.Contains(e["message"].(string), "are shared with previews") {
		t.Fatalf("set --previews=false: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "link", "STRIPE_SECRET_KEY", "--json")
	if r.Code != 0 || r.JSON["url"] != appURL {
		t.Fatalf("link: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "list", "--json")
	if r.Code != 0 || len(r.JSON["secrets"].([]any)) != 1 {
		t.Fatalf("list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "STRIPE_SECRET_KEY") || !strings.Contains(r.Stdout, "unset") || !strings.Contains(r.Stdout, "PREVIEWS") || !strings.Contains(r.Stdout, "shared") {
		t.Fatalf("list human: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "declare", "XERO_CLIENT_SECRET", "--json")
	if r.Code != 0 || len(st.declared) != 1 || st.declared[0]["scope"] != "app" || st.declared[0]["app_id"] != "crm" || st.declared[0]["name"] != "XERO_CLIENT_SECRET" {
		t.Fatalf("declare: %+v declared %v", r, st.declared)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "declare", "SHARED", "--scope", "org", "--json")
	if r.Code != 0 || st.declared[1]["scope"] != "org" || st.declared[1]["app_id"] != nil {
		t.Fatalf("declare org: %v", st.declared)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "versions", "STRIPE_SECRET_KEY", "--json")
	if r.Code != 0 || len(r.JSON["versions"].([]any)) != 2 {
		t.Fatalf("versions: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "rollback", "STRIPE_SECRET_KEY", "--to", "1", "--json")
	if r.Code != 0 || r.JSON["secret"].(map[string]any)["version"] != float64(3) {
		t.Fatalf("rollback: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "rollback", "STRIPE_SECRET_KEY", "--json")
	if r.Code != 1 {
		t.Fatalf("rollback without --to: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "share", "PROSPECT_TOKEN", "--from", "sales", "--json")
	if r.Code != 0 || r.JSON["secret"].(map[string]any)["scope"] != "org" || r.JSON["apps"].([]any)[0] != "crm" {
		t.Fatalf("share: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "share", "PROSPECT_TOKEN", "--json")
	if r.Code != 1 {
		t.Fatalf("share without --from: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "reads", "STRIPE_SECRET_KEY", "--since", "7d", "--json")
	if r.Code != 0 || len(r.JSON["reads"].([]any)) != 1 {
		t.Fatalf("reads: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "reads", "STRIPE_SECRET_KEY", "--since", "yesterday", "--json")
	if r.Code != 1 {
		t.Fatalf("reads bad since: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "secrets", "delete", "STRIPE_SECRET_KEY", "--json")
	if r.Code != 0 || len(st.deletedSecs) != 1 || st.deletedSecs[0] != "STRIPE_SECRET_KEY?app=crm" {
		t.Fatalf("delete: %+v %v", r, st.deletedSecs)
	}
}

func TestDomainsAndEnvs(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{}

	r := runRemote(t, dir, srv.URL, git, "domains", "add", "jobs.acme.example")
	if r.Code != 0 || !strings.Contains(r.Stdout, "TXT") || !strings.Contains(r.Stdout, "_whisk-verify.jobs.acme.example") || !strings.Contains(r.Stdout, "whisk-verify-9f3c") || !strings.Contains(r.Stdout, "CNAME") || !strings.Contains(r.Stdout, "crm--acme.whisk.page") || !strings.Contains(r.Stdout, "A (IPv4) or AAAA (IPv6) records to 95.217.38.236") {
		t.Fatalf("domains add: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "add", "jobs.acme.example", "--json")
	recs := r.JSON["records"].([]any)
	if r.Code != 0 || len(recs) != 2 || recs[0].(map[string]any)["value"] != "whisk-verify-9f3c" || recs[1].(map[string]any)["type"] != "CNAME" {
		t.Fatalf("domains add json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "verify", "jobs.acme.example", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "DOMAIN_UNVERIFIED" {
		t.Fatalf("domains verify: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "verify", "other.example", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("domains verify unknown: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "list", "--json")
	if r.Code != 0 || len(r.JSON["domains"].([]any)) != 1 {
		t.Fatalf("domains list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "business", "add", "acme.example")
	if r.Code != 0 || !strings.Contains(r.Stdout, "*.acme.example") || !strings.Contains(r.Stdout, "domains.whisk.page") || !strings.Contains(r.Stdout, "whisk-verify-77aa") || !strings.Contains(r.Stdout, "95.217.38.236") {
		t.Fatalf("domains business add: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "business", "show", "--json")
	if recs := r.JSON["records"].([]any); r.Code != 0 || len(recs) != 2 || recs[1].(map[string]any)["name"] != "*.acme.example" {
		t.Fatalf("domains business show: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "business", "verify")
	if r.Code != 0 || !strings.Contains(r.Stdout, "https://crm.acme.example") {
		t.Fatalf("domains business verify: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "business", "remove", "--json")
	if r.Code != 0 || r.JSON["removed"] != true {
		t.Fatalf("domains business remove: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "domains", "remove", "jobs.acme.example", "--json")
	if r.Code != 0 || r.JSON["removed"] != true {
		t.Fatalf("domains remove: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, git, "envs", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "preview:feature-x") || !strings.Contains(r.Stdout, "production") {
		t.Fatalf("envs list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "envs", "delete", "preview:feature-x", "--json")
	if r.Code != 0 || r.JSON["deleted"] != true {
		t.Fatalf("envs delete: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "envs", "delete", "production", "--json")
	if r.Code != 1 {
		t.Fatalf("envs delete production must refuse: %+v", r)
	}
}

func TestMembersAccessDeployKeys(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{}

	r := runRemote(t, dir, srv.URL, git, "members", "invite", "cara@acme.example", "--role", "developer", "--guest")
	if r.Code != 0 || !strings.Contains(r.Stdout, "https://whisk.run/invite/tok123") || !strings.Contains(r.Stdout, "(guest)") {
		t.Fatalf("members invite: %+v", r)
	}
	if st.invited[0]["role"] != "developer" || st.invited[0]["kind"] != "guest" {
		t.Fatalf("invite body: %v", st.invited)
	}
	r = runRemote(t, dir, srv.URL, git, "members", "invite", "dan@acme.example", "--json")
	if r.Code != 0 || r.JSON["invite_url"] != "https://whisk.run/invite/tok123" {
		t.Fatalf("members invite json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "members", "list", "--json")
	if r.Code != 0 || len(r.JSON["members"].([]any)) != 2 {
		t.Fatalf("members list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "members", "remove", "Bob@acme.example", "--json")
	if r.Code != 0 || r.JSON["removed"] != true {
		t.Fatalf("members remove: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "members", "remove", "zed@acme.example", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("members remove unknown: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, git, "access", "show")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Everyone in the org") {
		t.Fatalf("access show: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "access", "grant", "--group", "finance", "--audience", "customer", "--json")
	if r.Code != 0 || r.JSON["changed"] != true {
		t.Fatalf("access grant: %+v", r)
	}
	if len(st.putGrants) != 2 || st.putGrants[0]["subject_kind"] != "everyone" || st.putGrants[1]["subject_kind"] != "group" || st.putGrants[1]["subject_id"] != "g1" || st.putGrants[1]["audience"] != "customer" {
		t.Fatalf("PUT grants: %v", st.putGrants)
	}
	st.putGrants = nil
	r = runRemote(t, dir, srv.URL, git, "access", "grant", "--user", "bob@acme.example", "--json")
	if r.Code != 0 || len(st.putGrants) != 2 || st.putGrants[1]["subject_id"] != "u2" || st.putGrants[1]["audience"] != "team" {
		t.Fatalf("access grant user: %+v %v", r, st.putGrants)
	}
	r = runRemote(t, dir, srv.URL, git, "access", "grant", "--everyone", "--json")
	if r.Code != 0 || r.JSON["changed"] != false {
		t.Fatalf("access grant existing: %+v", r)
	}
	st.putGrants = nil
	r = runRemote(t, dir, srv.URL, git, "access", "revoke", "--everyone", "--json")
	if r.Code != 0 || len(st.putGrants) != 0 || r.JSON["removed"] != float64(1) {
		t.Fatalf("access revoke: %+v %v", r, st.putGrants)
	}
	r = runRemote(t, dir, srv.URL, git, "access", "revoke", "--user", "bob@acme.example", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("access revoke absent: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "access", "grant", "--group", "finance", "--user", "bob@acme.example", "--json")
	if r.Code != 1 {
		t.Fatalf("access grant two subjects: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, git, "deploy-keys", "create", "--label", "ci", "--json")
	if r.Code != 0 || r.JSON["value"] != "whsk_deploy_ONCE" || r.JSON["git_url"] != "https://git.whisk.run/01J/a1.git" {
		t.Fatalf("deploy-keys create: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, git, "deploy-keys", "create")
	if r.Code != 0 || !strings.Contains(r.Stdout, "whsk_deploy_ONCE") || !strings.Contains(r.Stdout, "shown once") {
		t.Fatalf("deploy-keys create human: %+v", r)
	}
}

func TestRemoteCommandsNeedABinding(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := t.TempDir()
	for _, args := range [][]string{{"deploy", "--force"}, {"status"}, {"deploys", "list"}, {"domains", "list"}, {"access", "show"}} {
		r := runRemote(t, dir, srv.URL, &fakeGit{}, append(args, "--json")...)
		if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "INVALID_REQUEST" {
			t.Fatalf("%v without a binding: %+v", args, r)
		}
	}
	r := runRemote(t, dir, srv.URL, &fakeGit{}, "members", "list", "--org", "acme", "--json")
	if r.Code != 0 {
		t.Fatalf("members list --org: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, &fakeGit{}, "secrets", "list", "--org", "acme", "--json")
	if r.Code != 0 || r.JSON["app"] != "" {
		t.Fatalf("secrets list --org falls back to org scope: %+v", r)
	}
}

func TestRunsOfFunction(t *testing.T) {
	runs := []api.Run{{ID: "r1", Function: "a"}, {ID: "r2", Function: "b"}, {ID: "r3", Function: "a"}}
	if got := runsOfFunction(runs, ""); len(got) != 3 {
		t.Errorf("no name keeps all: %v", got)
	}
	if got := runsOfFunction(runs, "a"); len(got) != 2 || got[0].ID != "r1" || got[1].ID != "r3" {
		t.Errorf("a: %v", got)
	}
}

// A cron reads in its declared zone, with the next run in that zone (CLI.md §5.8).
func TestCronTextKeepsTheZone(t *testing.T) {
	next := time.Date(2026, 9, 29, 17, 0, 0, 0, time.UTC) // 06:00 NZDT on the 30th
	cases := []struct {
		trigger api.Trigger
		want    string
	}{
		{api.Trigger{Cron: "*/10 5-21 * * 1-5", TZ: "Pacific/Auckland", NextRun: &next}, "*/10 5-21 * * 1-5 Pacific/Auckland (next 2026-09-30 06:00 NZDT)"},
		{api.Trigger{Cron: "0 * * * *", NextRun: &next}, "0 * * * * UTC (next 2026-09-29 17:00 UTC)"},
		{api.Trigger{Cron: "0 * * * *", TZ: "Pacific/Auckland"}, "0 * * * * Pacific/Auckland"},
		{api.Trigger{Cron: "0 * * * *", TZ: "Nowhere/Else", NextRun: &next}, "0 * * * * Nowhere/Else (next 2026-09-29 17:00Z)"},
		{api.Trigger{Cron: "*/5 * * * *", RunsAs: "0,10,20,30,40,50 * * * *", NextRun: &next}, "*/5 * * * * UTC (runs as 0,10,20,30,40,50 * * * * on this plan) (next 2026-09-29 17:00 UTC)"},
	}
	for _, c := range cases {
		if got := cronText(c.trigger); got != c.want {
			t.Errorf("cronText = %q, want %q", got, c.want)
		}
	}
	daily := api.Trigger{Cron: "0 6 * * *", TZ: "Pacific/Auckland", NextRun: &next}
	if got := triggerText([]api.Trigger{{Event: "po.created"}, daily}); got != "on po.created, 0 6 * * * Pacific/Auckland (next 2026-09-30 06:00 NZDT)" {
		t.Errorf("triggerText = %q", got)
	}
}

func TestPickOrg(t *testing.T) {
	cases := []struct {
		name string
		orgs []string
		want string
		code string
	}{
		{"none", nil, "", "FORBIDDEN_ROLE"},
		{"one", []string{"acme"}, "acme", ""},
		{"several", []string{"acme", "sober"}, "", "INVALID_REQUEST"},
	}
	for _, c := range cases {
		got, err := pickOrg(c.orgs)
		code := ""
		if e, ok := err.(*output.Error); ok {
			code = e.Code
		}
		if got != c.want || code != c.code {
			t.Errorf("%s: %q %v, want %q %s", c.name, got, err, c.want, c.code)
		}
	}
}
