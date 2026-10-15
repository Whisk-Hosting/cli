package whisk

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
)

// cloneGit answers the git calls whisk clone makes: the clone creates the directory, config
// and rev-parse run inside it.
type cloneGit struct {
	clone gitcmd.Result
	// cloneSchannel answers a clone made with -c http.sslBackend=schannel when set.
	cloneSchannel *gitcmd.Result
	sha           string
	calls         [][]string
	dirs          []string
	envs          [][]string
}

func (g *cloneGit) Run(ctx context.Context, dir string, env []string, args ...string) (gitcmd.Result, error) {
	g.calls, g.dirs, g.envs = append(g.calls, args), append(g.dirs, dir), append(g.envs, env)
	sub := args
	for len(sub) >= 2 && sub[0] == "-c" {
		sub = sub[2:]
	}
	switch {
	case len(sub) > 0 && sub[0] == "clone":
		res := g.clone
		if g.cloneSchannel != nil && strings.Contains(strings.Join(args, " "), "http.sslBackend=schannel") {
			res = *g.cloneSchannel
		}
		if res.OK() {
			if err := os.MkdirAll(filepath.Join(sub[len(sub)-1], ".git"), 0o755); err != nil {
				return gitcmd.Result{}, err
			}
		}
		return res, nil
	case len(sub) > 0 && sub[0] == "config":
		return gitcmd.Result{}, nil
	case strings.Join(sub, " ") == "rev-parse --verify --quiet HEAD":
		if g.sha == "" {
			return gitcmd.Result{ExitCode: 1}, nil
		}
		return gitcmd.Result{Stdout: g.sha + "\n"}, nil
	}
	return gitcmd.Result{ExitCode: 1, Stderr: "clone git: unexpected " + strings.Join(args, " ")}, nil
}

func (g *cloneGit) configs() [][]string {
	var out [][]string
	for _, c := range g.calls {
		if len(c) > 0 && c[0] == "config" {
			out = append(out, c)
		}
	}
	return out
}

func runCloneCLI(t *testing.T, goos, dir, api string, git gitcmd.Runner, args ...string) result {
	t.Helper()
	return runRemoteWith(t, goos, "", dir, api, git, args...)
}

func TestCloneBindsTheDirectoryAndSetsTheCredentialHelper(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	dir := t.TempDir()
	git := &cloneGit{sha: "abc1234abc"}
	r := runCloneCLI(t, "", dir, srv.URL, git, "clone", "acme/crm", "--json")
	if r.Code != 0 {
		t.Fatalf("clone: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
	target := filepath.Join(dir, "crm")
	if r.JSON["dir"] != target || r.JSON["commit_sha"] != "abc1234abc" || r.JSON["git_url"] != "https://git.whisk.run/01J/a1.git" {
		t.Fatalf("result: %v", r.JSON)
	}
	clone := git.calls[0]
	joined := strings.Join(clone, " ")
	if !strings.Contains(joined, "clone --quiet --origin whisk -- https://git.whisk.run/01J/a1.git "+target) {
		t.Fatalf("clone call: %v", clone)
	}
	if strings.Contains(joined, "whsk_agent_test") || !strings.Contains(strings.Join(git.envs[0], " "), gitcmd.TokenVar+"=whsk_agent_test") {
		t.Fatalf("the token must travel in the environment only: %v %v", clone, git.envs[0])
	}
	cfgs := git.configs()
	if len(cfgs) != 2 || cfgs[0][3] != "credential.https://git.whisk.run.helper" || cfgs[0][4] != "" || !strings.HasSuffix(cfgs[1][4], `" git-credential`) {
		t.Fatalf("config calls: %v", cfgs)
	}
	for i, c := range git.calls {
		if c[0] == "config" && git.dirs[i] != target {
			t.Fatalf("config ran in %s, not the clone", git.dirs[i])
		}
	}
	b, ok, err := config.LoadBinding(target)
	if err != nil || !ok || b.Org != "acme" || b.App != "crm" {
		t.Fatalf("binding: %v %v %v", b, ok, err)
	}
}

func TestCloneRefusesADirectoryInUse(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, "here"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "here", "notes.txt"), []byte("mine"), 0o644))
	git := &cloneGit{}
	r := runCloneCLI(t, "", dir, srv.URL, git, "clone", "acme/crm", "here", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "INVALID_REQUEST") || len(git.calls) != 0 {
		t.Fatalf("clone into a used directory: %d %s %v", r.Code, r.Stdout, git.calls)
	}
}

func TestCloneOfAnEmptyApp(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	r := runCloneCLI(t, "", t.TempDir(), srv.URL, &cloneGit{}, "clone", "acme/crm")
	if r.Code != 0 || !strings.Contains(r.Stdout, "no code yet") {
		t.Fatalf("empty clone: %d %s %s", r.Code, r.Stdout, r.Stderr)
	}
}

func TestCloneFailures(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	auth := &cloneGit{clone: gitcmd.Result{ExitCode: 128, Stderr: "fatal: Authentication failed for 'https://git.whisk.run/01J/a1.git/'"}}
	if r := runCloneCLI(t, "", t.TempDir(), srv.URL, auth, "clone", "acme/crm", "--json"); !strings.Contains(r.Stdout, "AUTH_REQUIRED") {
		t.Fatalf("auth: %s", r.Stdout)
	}
	// On Windows a clone git's OpenSSL refuses is retried through schannel, and the clone keeps
	// using the Windows store.
	tls := gitcmd.Result{ExitCode: 128, Stderr: "fatal: unable to access 'https://git.whisk.run/01J/a1.git/': SSL certificate problem: unable to get local issuer certificate"}
	win := &cloneGit{clone: tls, cloneSchannel: &gitcmd.Result{}, sha: "abc1234abc"}
	if r := runCloneCLI(t, "windows", t.TempDir(), srv.URL, win, "clone", "acme/crm", "--json"); r.Code != 0 {
		t.Fatalf("windows schannel: %d %s", r.Code, r.Stdout)
	}
	if cfgs := win.configs(); len(cfgs) != 3 || !reflect.DeepEqual(cfgs[2], []string{"config", "--local", "http.sslBackend", "schannel"}) {
		t.Fatalf("schannel config: %v", cfgs)
	}
	linux := &cloneGit{clone: tls}
	if r := runCloneCLI(t, "linux", t.TempDir(), srv.URL, linux, "clone", "acme/crm", "--json"); !strings.Contains(r.Stdout, "GIT_TLS_UNTRUSTED") || !strings.Contains(r.Stdout, "whisk clone again") {
		t.Fatalf("linux tls: %s", r.Stdout)
	}
}

func TestGitCredential(t *testing.T) {
	r := runRemoteIn(t, "protocol=https\nhost=git.whisk.run\npath=acme/crm.git\n\n", t.TempDir(), "http://unused", "git-credential", "get")
	if r.Code != 0 || r.Stdout != "username=whisk\npassword=whsk_agent_test\n" {
		t.Fatalf("get: %d %q %s", r.Code, r.Stdout, r.Stderr)
	}
	r = runRemoteIn(t, "protocol=http\nhost=git.whisk.run\n\n", t.TempDir(), "http://unused", "git-credential", "get")
	if r.Code != 0 || r.Stdout != "" {
		t.Fatalf("http must not get the token: %q", r.Stdout)
	}
	r = runRemoteIn(t, "protocol=https\nhost=git.whisk.run\nusername=whisk\npassword=x\n\n", t.TempDir(), "http://unused", "git-credential", "store")
	if r.Code != 0 || r.Stdout != "" {
		t.Fatalf("store: %d %q", r.Code, r.Stdout)
	}
}

func TestCloneConfigQuotesThePath(t *testing.T) {
	got := cloneConfig("https://git.whisk.run", `C:\Program Files\whisk\whisk.exe`, false)
	want := [][]string{
		{"--add", "credential.https://git.whisk.run.helper", ""},
		{"--add", "credential.https://git.whisk.run.helper", `!"C:/Program Files/whisk/whisk.exe" git-credential`},
	}
	if filepath.Separator != '\\' {
		// ToSlash only rewrites the host's own separator.
		want[1][2] = `!"C:\Program Files\whisk\whisk.exe" git-credential`
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cloneConfig: %v", got)
	}
}
