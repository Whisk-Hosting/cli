package whisk

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/contract/status"
)

func TestStalled(t *testing.T) {
	at := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	phases := func(status string) []api.Phase {
		return []api.Phase{{Phase: "queued", At: at.Add(-time.Minute)}, {Phase: status, At: at}}
	}
	cases := []struct {
		name   string
		d      api.Deploy
		now    time.Time
		waited time.Duration
		want   bool
	}{
		{"building past the limit", api.Deploy{Status: "building", Phases: phases("building")}, at.Add(47 * time.Minute), 47 * time.Minute, true},
		{"building at the limit", api.Deploy{Status: "building", Phases: phases("building")}, at.Add(deployStallLimit), deployStallLimit, true},
		{"building inside the limit", api.Deploy{Status: "building", Phases: phases("building")}, at.Add(3 * time.Minute), 0, false},
		{"queued with no phases reads its creation", api.Deploy{Status: "queued", CreatedAt: at}, at.Add(time.Hour), time.Hour, true},
		{"starting is past the build, never this", api.Deploy{Status: "starting", Phases: phases("starting")}, at.Add(time.Hour), 0, false},
		{"live", api.Deploy{Status: "live", Phases: phases("live")}, at.Add(time.Hour), 0, false},
		{"no time at all", api.Deploy{Status: "building"}, at, 0, false},
	}
	for _, c := range cases {
		waited, got := stalled(c.d, c.now)
		if got != c.want || (got && waited != c.waited) {
			t.Errorf("%s: stalled = %v after %s, want %v after %s", c.name, got, waited, c.want, c.waited)
		}
	}
}

func TestStuckDeploy(t *testing.T) {
	at := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	d := func(id, env string, status status.Deploy, since time.Duration) api.Deploy {
		return api.Deploy{ID: id, Environment: env, Status: status, Phases: []api.Phase{{Phase: string(status), At: at.Add(-since)}}}
	}
	cases := []struct {
		name    string
		deploys []api.Deploy
		env     string
		want    string
	}{
		{"none", nil, "production", ""},
		{"a fresh build is not stuck", []api.Deploy{d("d1", "production", "building", time.Minute)}, "production", ""},
		{"the stuck one", []api.Deploy{d("d2", "production", "queued", time.Minute), d("d1", "production", "building", time.Hour)}, "production", "d1"},
		{"the longest of two", []api.Deploy{d("d2", "production", "building", 30*time.Minute), d("d1", "production", "building", 2*time.Hour)}, "production", "d1"},
		{"another environment's does not count", []api.Deploy{d("d1", "preview:x", "building", time.Hour)}, "production", ""},
		{"finished ones do not count", []api.Deploy{d("d1", "production", "failed", time.Hour), d("d0", "production", "live", time.Hour)}, "production", ""},
	}
	for _, c := range cases {
		got, _, ok := stuckDeploy(c.deploys, c.env, at)
		if (c.want == "") == ok || (ok && got.ID != c.want) {
			t.Errorf("%s: got %q (%v), want %q", c.name, got.ID, ok, c.want)
		}
	}
}

func TestDeployInProgressError(t *testing.T) {
	at := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	d := api.Deploy{ID: "01J9", Status: "building", Environment: "production", CommitSHA: "abc1234abc", Phases: []api.Phase{{Phase: "building", At: at}}}
	cases := []struct {
		name    string
		lead    string
		extra   map[string]any
		message string
	}{
		{"alone", "", nil, "Deploy 01J9 has been building for 47 minutes without finishing."},
		{"after a push", "git push lost its connection to the platform", map[string]any{"git": []string{"fatal: the remote end hung up unexpectedly"}},
			"git push lost its connection to the platform, and deploy 01J9 has been building for 47 minutes without finishing."},
	}
	for _, c := range cases {
		e := deployInProgress(d, 47*time.Minute, c.lead, c.extra)
		if e.Code != "DEPLOY_IN_PROGRESS" || e.Message != c.message {
			t.Errorf("%s: %s %q", c.name, e.Code, e.Message)
		}
		if !strings.Contains(e.Fix, "whisk deploys cancel 01J9") || !strings.Contains(e.Fix, "whisk deploys info 01J9") {
			t.Errorf("%s: fix %q", c.name, e.Fix)
		}
		if e.Details["deploy_id"] != "01J9" || e.Details["waited_seconds"] != 47*60 || e.Details["since"] != "2026-09-08T10:00:00Z" || e.Details["commit_sha"] != "abc1234abc" {
			t.Errorf("%s: details %v", c.name, e.Details)
		}
		if _, has := e.Details["git"]; has != (c.extra != nil) {
			t.Errorf("%s: git in details = %v", c.name, has)
		}
		if e.Docs != "https://skill.whisk.run/errors/DEPLOY_IN_PROGRESS" {
			t.Errorf("%s: docs %q", c.name, e.Docs)
		}
	}
}

func TestSpanWordsCLI(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Second: "45 seconds",
		time.Minute:      "1 minute",
		47 * time.Minute: "47 minutes",
		3 * time.Hour:    "3 hours",
		72 * time.Hour:   "3 days",
	}
	for d, want := range cases {
		if got := spanWords(d); got != want {
			t.Errorf("spanWords(%s) = %q, want %q", d, got, want)
		}
	}
}

// runRemoteAt is runRemote with the CLI's clock set to now.
func runRemoteAt(t *testing.T, now time.Time, dir, apiURL string, git gitcmd.Runner, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"WHISK_API": apiURL, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude"}
	e := Env{Stdout: &out, Stderr: &errb, Stdin: strings.NewReader(""), Dir: dir, ConfigDir: t.TempDir(), Store: &memStore{m: map[string]config.Credential{}}, Git: git,
		Getenv: func(k string) string { return env[k] }, Now: func() time.Time { return now }}
	code := Run(context.Background(), args, e)
	r := result{Code: code, Stdout: out.String(), Stderr: errb.String()}
	if lines := jsonLines(r.Stdout); len(lines) > 0 {
		r.JSON = lines[len(lines)-1]
	}
	return r
}

// A building deploy of the pushed commit is followed while it is fresh, and named as stuck,
// with its id, how long it has sat and how to cancel it, once it is past the limit.
func TestDeployUpToDateWithADeployStillBuilding(t *testing.T) {
	building := map[string]any{"id": "d-rb", "commit_sha": "abc1234abc", "environment": "production", "status": "building", "branch": "main", "created_at": "2026-09-08T09:59:00Z",
		"phases": []any{map[string]any{"phase": "queued", "at": "2026-09-08T09:59:00Z"}, map[string]any{"phase": "building", "at": "2026-09-08T10:00:00Z"}}}
	upToDate := func() *fakeGit {
		return &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git",
			push: gitcmd.Result{Stdout: "To x\n=\tmain:refs/heads/main\t[up to date]\nDone\n"}}
	}

	st := &remoteState{deploys: []map[string]any{building}}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemoteAt(t, time.Date(2026, 9, 8, 10, 47, 0, 0, time.UTC), dir, srv.URL, upToDate(), "deploy", "--force", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 1 || e["code"] != "DEPLOY_IN_PROGRESS" {
		t.Fatalf("stuck deploy: %+v", r)
	}
	if e["message"] != "Deploy d-rb has been building for 47 minutes without finishing." || !strings.Contains(e["fix"].(string), "whisk deploys cancel d-rb") {
		t.Fatalf("stuck deploy error: %v", e)
	}
	if len(st.posted) != 0 {
		t.Fatalf("a stuck deploy must not be deployed again behind the agent's back: %v", st.posted)
	}

	r = runRemoteAt(t, time.Date(2026, 9, 8, 10, 3, 0, 0, time.UTC), dir, srv.URL, upToDate(), "deploy", "--force")
	if r.Code != 0 || !strings.Contains(r.Stderr, "deploy d-rb (building for 3 minutes); following it") || !strings.Contains(r.Stdout+r.Stderr, "https://crm--acme.whisk.page") {
		t.Fatalf("fresh deploy should be followed to live: %+v", r)
	}
}

// A push that loses its connection while the environment has a deploy stuck building names
// that deploy; without one it stays PLATFORM_UNAVAILABLE.
func TestDeployPushHungUpNamesAStuckDeploy(t *testing.T) {
	hungUp := gitcmd.Result{ExitCode: 128, Stderr: "error: RPC failed; curl 56 Recv failure: Connection was reset\nsend-pack: unexpected disconnect while reading sideband packet\nfatal: the remote end hung up unexpectedly\n"}
	stuck := map[string]any{"id": "d-stuck", "commit_sha": "0ld0ld0ld0", "environment": "production", "status": "building", "branch": "main", "created_at": "2026-09-08T09:00:00Z",
		"phases": []any{map[string]any{"phase": "building", "at": "2026-09-08T09:13:00Z"}}}
	at := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)

	st := &remoteState{deploys: []map[string]any{stuck}}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: hungUp}

	r := runRemoteAt(t, at, dir, srv.URL, git, "deploy", "--force", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 1 || e["code"] != "DEPLOY_IN_PROGRESS" {
		t.Fatalf("hung-up push with a stuck deploy: %+v", r)
	}
	details := e["details"].(map[string]any)
	if e["message"] != "git push lost its connection to the platform, and deploy d-stuck has been building for 47 minutes without finishing." ||
		details["deploy_id"] != "d-stuck" || details["git"] == nil || !strings.Contains(e["fix"].(string), "whisk deploys cancel d-stuck") {
		t.Fatalf("error: %v", e)
	}

	st.deploys = nil
	r = runRemoteAt(t, at, dir, srv.URL, git, "deploy", "--force", "--json")
	e, _ = r.JSON["error"].(map[string]any)
	if r.Code != 5 || e["code"] != "PLATFORM_UNAVAILABLE" || !strings.Contains(e["message"].(string), "the remote end hung up") {
		t.Fatalf("hung-up push alone: %+v", r)
	}
}
