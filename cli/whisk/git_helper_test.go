package whisk

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/tokensig"
)

// helperGit is a global git config holding values for one key, recording every call.
type helperGit struct {
	values []string
	fail   bool
	calls  [][]string
}

func (g *helperGit) Run(ctx context.Context, dir string, env []string, args ...string) (gitcmd.Result, error) {
	g.calls = append(g.calls, args)
	switch {
	case len(args) == 4 && args[2] == "--get-all":
		if g.values == nil {
			return gitcmd.Result{ExitCode: 1}, nil
		}
		return gitcmd.Result{Stdout: strings.Join(g.values, "\n") + "\n"}, nil
	case g.fail:
		return gitcmd.Result{ExitCode: 255, Stderr: "error: could not lock config file"}, nil
	case args[2] == "--unset-all":
		g.values = []string{}
	case args[2] == "--add":
		g.values = append(g.values, args[4])
	}
	return gitcmd.Result{}, nil
}

func helperSession(git gitcmd.Runner, setup bool, errb *bytes.Buffer) *session {
	return &session{ctx: context.Background(), git: git,
		env:     Env{SetupGit: setup, Executable: "/usr/local/bin/whisk", Dir: "/"},
		printer: output.Printer{Out: &bytes.Buffer{}, Err: errb}}
}

// The released CLI makes itself git's one helper for the platform's git host in the global git
// config, so Git Credential Manager is never asked, and leaves a config that is already right
// untouched.
func TestSetupGitHelperMakesWhiskTheOnlyHelperForTheGitHost(t *testing.T) {
	want := []string{"", `!"/usr/local/bin/whisk" git-credential`}
	for _, start := range [][]string{nil, {"manager"}, {"", `!"/old/whisk" git-credential`}} {
		git := &helperGit{values: start}
		var errb bytes.Buffer
		helperSession(git, true, &errb).setupGitHelper("https://git.whisk.run/01J/a1.git")
		if !reflect.DeepEqual(git.values, want) {
			t.Fatalf("from %q: %q", start, git.values)
		}
		for _, c := range git.calls {
			if c[0] != "config" || c[1] != "--global" || c[len(c)-1] == "" && c[2] != "--add" {
				t.Fatalf("call %q", c)
			}
			if strings.Contains(strings.Join(c, " "), "git.whisk.run/01J") {
				t.Fatalf("scoped to the path, not the host: %q", c)
			}
		}
		if !strings.Contains(errb.String(), "never ask for a password") {
			t.Fatalf("said %q", errb.String())
		}
	}

	git := &helperGit{values: want}
	var errb bytes.Buffer
	helperSession(git, true, &errb).setupGitHelper("https://git.whisk.run/01J/a1.git")
	if len(git.calls) != 1 || errb.Len() != 0 {
		t.Fatalf("rewrote a config that was right: %q %q", git.calls, errb.String())
	}
}

// Tests, library callers and a local stack's plain http host leave the global git config alone,
// and a config git cannot write is said once without failing the command.
func TestSetupGitHelperLeavesConfigAlone(t *testing.T) {
	for _, tc := range []struct {
		setup bool
		url   string
	}{{false, "https://git.whisk.run/01J/a1.git"}, {true, "http://127.0.0.1:8081/01J/a1.git"}, {true, ""}, {true, "ftp://git.whisk.run/x"}} {
		git := &helperGit{}
		helperSession(git, tc.setup, &bytes.Buffer{}).setupGitHelper(tc.url)
		if len(git.calls) != 0 {
			t.Fatalf("%v %q: %q", tc.setup, tc.url, git.calls)
		}
	}
	git := &helperGit{fail: true}
	var errb bytes.Buffer
	helperSession(git, true, &errb).setupGitHelper("https://git.whisk.run/01J/a1.git")
	if !strings.Contains(errb.String(), "could not lock config file") || !strings.Contains(errb.String(), "git config --global --add credential.https://git.whisk.run.helper") {
		t.Fatalf("said %q", errb.String())
	}
}

func TestGitURLOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.whisk.run":         "https://git.whisk.run",
		"https://api.whisk.run/":        "https://git.whisk.run",
		"https://api.staging.whisk.run": "https://git.staging.whisk.run",
		"http://api.whisk.run":          "",
		"https://whisk.run":             "",
		"http://127.0.0.1:8080":         "",
		"https://api.whisk.run:8443":    "",
		"":                              "",
	} {
		if got := gitURLOf(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

// With no password to give, whisk git-credential tells git to stop, so neither Git Credential
// Manager nor git itself opens a sign-in window, and says why on stderr with the fix.
func TestGitCredentialStopsGitWhenThereIsNoSignIn(t *testing.T) {
	run := func(store *memStore) (string, string) {
		t.Helper()
		var out, errb bytes.Buffer
		e := Env{Stdin: strings.NewReader("protocol=https\nhost=git.whisk.run\n\n"), Stdout: &out, Stderr: &errb, Dir: t.TempDir(), ConfigDir: t.TempDir(), Store: store,
			Getenv: func(string) string { return "" }}
		if code := Run(context.Background(), []string{"git-credential", "get"}, e); code != 0 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
		return out.String(), errb.String()
	}
	out, errb := run(&memStore{m: map[string]config.Credential{}})
	if out != "quit=1\n" || !strings.Contains(errb, "whisk: AUTH_REQUIRED: ") || !strings.Contains(errb, "whisk: fix: Run whisk login") {
		t.Fatalf("no sign-in: %q %q", out, errb)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "TOKEN_EXPIRED", "message": "This token has expired.", "fix": "Run whisk login again."}})
	}))
	defer srv.Close()
	_, key, err := tokensig.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	expired := &memStore{m: map[string]config.Credential{"default": {Token: "whsk_old", API: srv.URL, PrivateKey: key}}}
	out, errb = run(expired)
	if out != "quit=1\n" || !strings.Contains(errb, "whisk: TOKEN_EXPIRED: ") || !strings.Contains(errb, "whisk: fix: ") {
		t.Fatalf("expired: %q %q", out, errb)
	}
	if strings.Contains(out+errb, "whsk_old") {
		t.Fatal("an expired token was handed out")
	}
}
