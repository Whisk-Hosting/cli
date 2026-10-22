package whisk

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
)

// twoCopies is an app's repository on "Whisk" (a bare repository whose post-receive names a
// deploy, reached through an insteadOf rule for the API's git_url) with two working copies of
// it: other, a teammate's, and mine, bound to acme/crm, both at the same first commit.
type twoCopies struct {
	bare, other, mine string
}

const behindGitURL = "https://git.whisk.run/01J/a1.git"

func newTwoCopies(t *testing.T, api string) twoCopies {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	c := twoCopies{bare: filepath.Join(root, "app.git"), other: filepath.Join(root, "other"), mine: filepath.Join(root, "mine")}
	gitIn(t, root, "init", "--quiet", "--bare", "--initial-branch=main", c.bare)
	hook := filepath.Join(c.bare, "hooks", "post-receive")
	must(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'whisk: deploy d-new'\n"), 0o755))
	for _, dir := range []string{c.other, c.mine} {
		gitIn(t, root, "init", "--quiet", "--initial-branch=main", dir)
		gitIn(t, dir, "config", "user.name", "Pat Smith")
		gitIn(t, dir, "config", "user.email", "pat@example.com")
		gitIn(t, dir, "config", "url."+c.bare+".insteadOf", behindGitURL)
		gitIn(t, dir, "remote", "add", "whisk", behindGitURL)
	}
	writeCommit(t, c.other, "app.txt", "jobs\n", "First version")
	gitIn(t, c.other, "push", "--quiet", "whisk", "main")
	gitIn(t, c.mine, "pull", "--quiet", "whisk", "main")
	must(t, config.SaveBinding(c.mine, config.Binding{Org: "acme", App: "crm", API: api}))
	must(t, os.WriteFile(filepath.Join(c.mine, ".git", "info", "exclude"), []byte(".whisk/\n"), 0o644))
	return c
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	res, err := gitcmd.Exec{}.Run(context.Background(), dir, nil, args...)
	if err != nil || !res.OK() {
		t.Fatalf("git %s in %s: %v %s", strings.Join(args, " "), dir, err, res.Stderr)
	}
	return res.Out()
}

func writeCommit(t *testing.T, dir, file, content, message string) string {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
	gitIn(t, dir, "add", file)
	gitIn(t, dir, "commit", "--quiet", "-m", message)
	return gitIn(t, dir, "rev-parse", "HEAD")
}

// A teammate pushed a change to another file: whisk deploy rebases this copy's commit onto it
// and pushes, so Whisk's main has both changes and the teammate's is kept.
func TestDeployCatchesUpWithATeammatesPush(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	c := newTwoCopies(t, srv.URL)
	theirs := writeCommit(t, c.other, "export.txt", "csv\n", "Add a CSV export")
	gitIn(t, c.other, "push", "--quiet", "whisk", "main")
	writeCommit(t, c.mine, "app.txt", "jobs and invoices\n", "Show invoices beside jobs")

	r := runRemote(t, c.mine, srv.URL, gitcmd.Exec{}, "deploy", "--org", "acme", "--app", "crm", "--force", "--no-wait", "--json")
	if r.Code != 0 {
		t.Fatalf("deploy should catch up and go through: %+v", r)
	}
	head := gitIn(t, c.mine, "rev-parse", "HEAD")
	if cu, _ := r.JSON["caught_up"].(map[string]any); cu["remote_commit"] != theirs || cu["behind"] != 1.0 || cu["rebased"] != 1.0 || cu["commit_sha"] != head {
		t.Errorf("the summary should say what was rebased onto %s: %v", theirs, r.JSON["caught_up"])
	}
	if got := gitIn(t, c.bare, "rev-parse", "main"); got != head {
		t.Errorf("Whisk's main is %s, want this copy's rebased HEAD %s", got, head)
	}
	if r.JSON["commit_sha"] != head {
		t.Errorf("the summary should name the rebased commit %s: %v", head, r.JSON)
	}
	gitIn(t, c.bare, "merge-base", "--is-ancestor", theirs, "main")
	if files := gitIn(t, c.bare, "ls-tree", "--name-only", "main"); !strings.Contains(files, "export.txt") || !strings.Contains(files, "app.txt") {
		t.Errorf("Whisk's main should have both changes: %s", files)
	}

	// Once more in human mode, which says on stderr what it rebased.
	gitIn(t, c.other, "pull", "--quiet", "--rebase", "whisk", "main")
	writeCommit(t, c.other, "export.txt", "csv and pdf\n", "Add a PDF export")
	gitIn(t, c.other, "push", "--quiet", "whisk", "main")
	writeCommit(t, c.mine, "app.txt", "jobs, invoices and quotes\n", "Show quotes too")
	human := runRemote(t, c.mine, srv.URL, gitcmd.Exec{}, "deploy", "--org", "acme", "--app", "crm", "--force", "--no-wait")
	if human.Code != 0 || !strings.Contains(human.Stderr, "main on Whisk had 1 new commit(s) from someone else; rebased this copy's 1 commit(s) onto them, now ") {
		t.Errorf("a person should be told what was rebased: %+v", human)
	}
}

// Both changed the same line: the rebase conflicts, is aborted, and the answer is PUSH_BEHIND
// naming the file, with this copy exactly as it was and nothing pushed.
func TestDeployBehindWithAConflictLeavesTheCopyAlone(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	c := newTwoCopies(t, srv.URL)
	writeCommit(t, c.other, "app.txt", "jobs by date\n", "Sort jobs by date")
	gitIn(t, c.other, "push", "--quiet", "whisk", "main")
	theirs := gitIn(t, c.other, "rev-parse", "HEAD")
	mine := writeCommit(t, c.mine, "app.txt", "jobs by customer\n", "Group jobs by customer")

	r := runRemote(t, c.mine, srv.URL, gitcmd.Exec{}, "deploy", "--org", "acme", "--app", "crm", "--force", "--no-wait", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 3 || e == nil || e["code"] != "PUSH_BEHIND" {
		t.Fatalf("a conflicting catch-up should be PUSH_BEHIND, exit 3: %+v", r)
	}
	d := e["details"].(map[string]any)
	if d["reason"] != "conflict" || d["remote_commit"] != theirs || d["behind"] != 1.0 || d["ahead"] != 1.0 {
		t.Errorf("details: %v", d)
	}
	if cs, _ := d["conflicts"].([]any); len(cs) != 1 || cs[0] != "app.txt" {
		t.Errorf("details.conflicts = %v, want [app.txt]", d["conflicts"])
	}
	if fix := e["fix"].(string); !strings.Contains(fix, "git rebase whisk/main") || !strings.Contains(fix, "Never force-push") {
		t.Errorf("fix: %s", fix)
	}
	if got := gitIn(t, c.mine, "rev-parse", "HEAD"); got != mine {
		t.Errorf("this copy's HEAD moved to %s", got)
	}
	if _, err := os.Stat(filepath.Join(c.mine, ".git", "rebase-merge")); err == nil {
		t.Error("the rebase was left in progress")
	}
	if got := gitIn(t, c.bare, "rev-parse", "main"); got != theirs {
		t.Errorf("Whisk's main moved to %s", got)
	}
}

// This copy's commit is already on another remote: rebasing it would fork that history, so the
// answer is PUSH_BEHIND telling the agent to merge.
func TestDeployBehindWithSharedCommitsDoesNotRebase(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	c := newTwoCopies(t, srv.URL)
	writeCommit(t, c.other, "export.txt", "csv\n", "Add a CSV export")
	gitIn(t, c.other, "push", "--quiet", "whisk", "main")
	mine := writeCommit(t, c.mine, "app.txt", "jobs and invoices\n", "Show invoices beside jobs")
	github := filepath.Join(t.TempDir(), "github.git")
	gitIn(t, c.mine, "init", "--quiet", "--bare", github)
	gitIn(t, c.mine, "remote", "add", "origin", github)
	gitIn(t, c.mine, "push", "--quiet", "origin", "main")

	r := runRemote(t, c.mine, srv.URL, gitcmd.Exec{}, "deploy", "--org", "acme", "--app", "crm", "--force", "--no-wait", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 3 || e == nil || e["code"] != "PUSH_BEHIND" {
		t.Fatalf("shared commits should be PUSH_BEHIND: %+v", r)
	}
	if d := e["details"].(map[string]any); d["reason"] != "shared" {
		t.Errorf("details: %v", d)
	}
	if !strings.Contains(e["fix"].(string), "git merge whisk/main") {
		t.Errorf("fix: %s", e["fix"])
	}
	if got := gitIn(t, c.mine, "rev-parse", "HEAD"); got != mine {
		t.Errorf("this copy's HEAD moved to %s", got)
	}
}

// --branch main from another branch: only the checked-out branch is brought up to date.
func TestDeployBehindOnABranchNotCheckedOut(t *testing.T) {
	srv := remoteAPI(t, &remoteState{})
	defer srv.Close()
	c := newTwoCopies(t, srv.URL)
	writeCommit(t, c.other, "export.txt", "csv\n", "Add a CSV export")
	gitIn(t, c.other, "push", "--quiet", "whisk", "main")
	writeCommit(t, c.mine, "app.txt", "jobs and invoices\n", "Show invoices beside jobs")
	gitIn(t, c.mine, "switch", "--quiet", "-c", "try-colours")

	r := runRemote(t, c.mine, srv.URL, gitcmd.Exec{}, "deploy", "--org", "acme", "--app", "crm", "--force", "--no-wait", "--json", "--branch", "main")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 3 || e == nil || e["code"] != "PUSH_BEHIND" || e["details"].(map[string]any)["reason"] != "not_checked_out" {
		t.Fatalf("want PUSH_BEHIND not_checked_out: %+v", r)
	}
	if !strings.Contains(e["fix"].(string), "git switch main") {
		t.Errorf("fix: %s", e["fix"])
	}
}

func TestPushBehindWords(t *testing.T) {
	for _, tc := range []struct {
		b       behind
		message string
		fix     string
	}{
		{behind{Branch: "main", Reason: "raced", Remote: "3f9c2a1b7d0e4c55", Behind: 2},
			"Someone else pushed to main on Whisk since this copy last pulled: it is at 3f9c2a1, with 2 commit(s) this copy does not have. It moved again while whisk deploy was catching up.",
			"Run whisk deploy again; it brings this copy up to date itself. Never force-push: it deletes the other person's work."},
		{behind{Branch: "main", Reason: "fetch_failed"},
			"Someone else pushed to main on Whisk since this copy last pulled.",
			"Run git pull --rebase whisk main, resolve any conflicts so both changes still do what they were meant to, run the tests, then run whisk deploy again. Never force-push: it deletes the other person's work."},
		{behind{Branch: "main", Src: "HEAD", Reason: "not_checked_out", Remote: "3f9c2a1b7d0e", Behind: 1},
			"Someone else pushed to main on Whisk since this copy last pulled: it is at 3f9c2a1, with 1 commit(s) this copy does not have. whisk deploy brings only the checked-out branch up to date, and main is not checked out.",
			"Run git switch main, then whisk deploy again; it brings the branch up to date itself. Never force-push: it deletes the other person's work."},
		{behind{Branch: "try-colours", Reason: "rebase_failed", Remote: "3f9c2a1b7d0e", Behind: 1},
			"Someone else pushed to try-colours on Whisk since this copy last pulled: it is at 3f9c2a1, with 1 commit(s) this copy does not have. Rebasing onto them stopped before it finished, so nothing here was changed.",
			"Run git rebase whisk/try-colours and follow what git says, run the tests, then run whisk deploy again. Never force-push: it deletes the other person's work."},
	} {
		e := pushBehind(tc.b)
		if e.Code != "PUSH_BEHIND" || e.Message != tc.message || e.Fix != tc.fix {
			t.Errorf("%s:\n got %q\n     %q\nwant %q\n     %q", tc.b.Reason, e.Message, e.Fix, tc.message, tc.fix)
		}
	}
}

func TestBehind(t *testing.T) {
	for summary, want := range map[string]bool{
		"[rejected] (fetch first)":                      true,
		"[rejected] (non-fast-forward)":                 true,
		"[remote rejected] (pre-receive hook declined)": false,
		"[rejected] (stale info)":                       false,
		"[new branch]":                                  false,
	} {
		if got := gitcmd.Behind(summary); got != want {
			t.Errorf("Behind(%q) = %v, want %v", summary, got, want)
		}
	}
}

func TestParseLeftRight(t *testing.T) {
	if l, r, ok := parseLeftRight("2\t1\n"); !ok || l != 2 || r != 1 {
		t.Errorf("parseLeftRight = %d %d %v", l, r, ok)
	}
	if _, _, ok := parseLeftRight("fatal: bad revision"); ok {
		t.Error("parseLeftRight accepted git's error")
	}
}
