package doctor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

// W011 covers whisk dev's folder: a tracked file under .whisk/dev/ is an error, an ignored
// one is not, and --fix adds the ignore line and reports the git rm.
func TestW011WhiskDev(t *testing.T) {
	cases := []struct {
		name  string
		repo  Repo
		files []File
		want  bool
		note  string
	}{
		{"tracked secrets.env in a repository", Repo{IsGit: true, GitOK: true}, []File{{Path: ".whisk/dev/secrets.env", Tracked: true}}, true, "run: git rm -r --cached .whisk/dev"},
		{"tracked compose.yaml", Repo{IsGit: true, GitOK: true}, []File{{Path: ".whisk/dev/compose.yaml", Tracked: true}}, true, "run: git rm -r --cached .whisk/dev"},
		{"untracked, not ignored", Repo{IsGit: true, GitOK: true}, []File{{Path: ".whisk/dev/secrets.env"}}, false, ""},
		{"no repository, not ignored", Repo{}, []File{{Path: ".whisk/dev/secrets.env", Tracked: true}}, true, ""},
		{"the binding is not whisk dev's", Repo{IsGit: true, GitOK: true}, []File{{Path: ".whisk/app.json", Tracked: true}}, false, ""},
		{"a nested folder of the same name", Repo{IsGit: true, GitOK: true}, []File{{Path: "docs/.whisk/devnotes", Tracked: true}}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.repo
			r.Files = c.files
			o := w011(r, Context{})
			if (len(o.findings) > 0) != c.want {
				t.Fatalf("findings %+v, want %v", o.findings, c.want)
			}
			edits, notes := planFixes(r, o.findings)
			if !c.want {
				return
			}
			joined := strings.Join(notes, "\n")
			if c.note != "" && !strings.Contains(joined, c.note) {
				t.Errorf("notes %v lack %q", notes, c.note)
			}
			if c.note == "" && strings.Contains(joined, "git rm") {
				t.Errorf("git rm suggested outside a repository: %v", notes)
			}
			if len(edits) != 1 || edits[0].Path != ".gitignore" || !strings.Contains(string(edits[0].Content), ".whisk/dev/\n") {
				t.Errorf("edits %+v", edits)
			}
		})
	}
}

// End to end without a repository: whisk dev's folder, not ignored, fails doctor; --fix
// ignores it and the finding goes away.
func TestW011WhiskDevFix(t *testing.T) {
	files := with(passing, map[string]string{".whisk/dev/secrets.env": "API_KEY=value\n"})
	if rep := runDoctor(t, files, false); !has(rep, "W011") {
		t.Fatalf("W011 did not trigger: %+v", rep.Findings)
	}
	dir := writeAll(t, files)
	rep, err := Run(context.Background(), Options{Dir: dir, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	if has(rep, "W011") {
		t.Errorf("W011 still reported after --fix: %+v", rep.Findings)
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(gi), ".whisk/dev/") || !strings.HasPrefix(string(gi), passing[".gitignore"]) {
		t.Errorf(".gitignore after fix:\n%s", gi)
	}
}

func TestW006BindingAgainstRemote(t *testing.T) {
	bound := &config.Binding{Org: "acme", App: "crm"}
	own := "https://git.whisk.run/01J/a1.git"
	tracked := []File{{Path: ".whisk/app.json", Tracked: true}}
	cases := []struct {
		name    string
		binding *config.Binding
		files   []File
		remote  string
		gitURL  string
		finding bool
		skipped bool
	}{
		{"committed binding, remote of another app", bound, tracked, "https://tok:pw@git.whisk.run/02K/b7.git", own, true, false},
		{"committed binding, matching remote", bound, tracked, own, own, false, false},
		{"committed binding, no remote", bound, tracked, "", own, false, false},
		{"binding written here, not tracked", bound, []File{{Path: ".whisk/app.json"}}, "https://git.whisk.run/02K/b7.git", own, false, false},
		{"not bound", nil, nil, "https://git.whisk.run/02K/b7.git", own, false, false},
		{"platform not asked", bound, tracked, "https://git.whisk.run/02K/b7.git", "", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Repo{Binding: c.binding, Files: c.files, WhiskRemote: c.remote, IsGit: true, GitOK: true}
			o := w006(r, Context{BoundGitURL: c.gitURL})
			if (len(o.findings) > 0) != c.finding || (len(o.skipped) > 0) != c.skipped {
				t.Fatalf("findings %+v skipped %v", o.findings, o.skipped)
			}
			for _, f := range o.findings {
				if f.Level != "warning" || f.Fix == "" || f.File != ".whisk/app.json" || strings.Contains(f.Message, "pw@") || !strings.Contains(f.Message, "acme/crm") {
					t.Errorf("finding %+v", f)
				}
			}
		})
	}
}

// Through Run with a real repository: the remote is read from git and the platform is asked
// only when the committed binding and a remote both exist.
func TestW006Run(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := writeAll(t, passing)
	if err := config.SaveBinding(dir, config.Binding{Org: "acme", App: "fixture"}); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "--quiet")
	git("add", "--all")
	asked := 0
	ask := func(url string, err error) func(context.Context) (string, error) {
		return func(context.Context) (string, error) { asked++; return url, err }
	}
	run := func(f func(context.Context) (string, error)) Report {
		rep, err := Run(context.Background(), Options{Dir: dir, BoundGitURL: f})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	if rep := run(ask("https://git.whisk.run/01J/a1.git", nil)); has(rep, "W006") || asked != 0 {
		t.Fatalf("no remote: asked %d, findings %+v", asked, rep.Findings)
	}
	git("remote", "add", "whisk", "https://git.whisk.run/02K/b7.git")
	if rep := run(ask("https://git.whisk.run/01J/a1.git", nil)); !has(rep, "W006") || asked != 1 {
		t.Fatalf("mismatched remote: asked %d, findings %+v", asked, rep.Findings)
	}
	if rep := run(ask("https://git.whisk.run/02K/b7.git", nil)); has(rep, "W006") {
		t.Fatalf("matching remote reported: %+v", rep.Findings)
	}
	rep := run(ask("", errors.New("offline")))
	if has(rep, "W006") || !strings.Contains(strings.Join(rep.Skipped, "\n"), "W006") {
		t.Fatalf("offline: findings %+v skipped %v", rep.Findings, rep.Skipped)
	}
}
