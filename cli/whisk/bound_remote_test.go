package whisk

import (
	"strings"
	"testing"
)

func TestSplitLocal(t *testing.T) {
	cases := []struct {
		paths, commit, local []string
	}{
		{nil, nil, nil},
		{[]string{"whisk.yaml", "src/app.ts"}, []string{"whisk.yaml", "src/app.ts"}, nil},
		{[]string{".whisk/dev/secrets.env", "whisk.yaml"}, []string{"whisk.yaml"}, []string{".whisk/dev/secrets.env"}},
		{[]string{"app/.whisk/dev/compose.yaml"}, nil, []string{"app/.whisk/dev/compose.yaml"}},
		{[]string{`".whisk/dev/odd name.env"`}, nil, []string{`".whisk/dev/odd name.env"`}},
		{[]string{"notes.txt -> .whisk/dev/secrets.env"}, nil, []string{"notes.txt -> .whisk/dev/secrets.env"}},
		{[]string{".whisk/app.json", ".whisk/devices.txt", "x.whisk/dev/y"}, []string{".whisk/app.json", ".whisk/devices.txt", "x.whisk/dev/y"}, nil},
	}
	for _, c := range cases {
		commit, local := splitLocal(c.paths)
		if strings.Join(commit, "|") != strings.Join(c.commit, "|") || strings.Join(local, "|") != strings.Join(c.local, "|") {
			t.Errorf("splitLocal(%v) = %v, %v; want %v, %v", c.paths, commit, local, c.commit, c.local)
		}
	}
}

// A binding that disagrees with the checkout's remote whisk is refused before anything is
// committed or pushed; --org and --app, or whisk use, settle it.
func TestDeployRefusesABindingTheRemoteDisagreesWith(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	other := "https://user:pw@git.whisk.run/02K/b7.git"

	cases := []struct {
		name   string
		args   []string
		remote string
		refuse bool
	}{
		{"remote of another app", []string{"deploy", "--force", "--commit", "--json"}, other, true},
		{"no remote yet", []string{"deploy", "--force", "--commit", "--json"}, "", false},
		{"the bound app's remote", []string{"deploy", "--force", "--commit", "--json"}, "https://git.whisk.run/01J/a1.git", false},
		{"explicit --org and --app", []string{"deploy", "--force", "--commit", "--json", "--org", "acme", "--app", "crm"}, other, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			git := &fakeGit{repo: true, dirty: []string{"src/app.ts"}, sha: "abc1234abc", branch: "main", remoteURL: c.remote, push: pushOK("d-new")}
			r := runRemote(t, dir, srv.URL, git, c.args...)
			if !c.refuse {
				if r.Code != 0 {
					t.Fatalf("deploy: %+v", r)
				}
				return
			}
			e, _ := r.JSON["error"].(map[string]any)
			if r.Code == 0 || e["code"] != "INVALID_REQUEST" || !strings.Contains(e["fix"].(string), "whisk use acme/crm") {
				t.Fatalf("not refused: %+v", r)
			}
			if strings.Contains(r.Stdout, "pw@") {
				t.Errorf("remote credentials printed: %s", r.Stdout)
			}
			if git.ran("commit") || git.ran("add") || git.ran("remote set-url") {
				t.Errorf("refused deploy still changed the repository: %v", git.calls)
			}
			if call, _ := git.pushCall(); call != nil {
				t.Errorf("refused deploy pushed: %v", call)
			}
		})
	}
}

// whisk use confirms a binding: an existing remote whisk is pointed at the app's repository,
// and a directory without one is left alone.
func TestUsePointsTheRemoteAtTheBoundApp(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	cases := []struct {
		remote  string
		setURL  bool
		updated bool
	}{
		{"https://git.whisk.run/02K/b7.git", true, true},
		{"https://git.whisk.run/01J/a1.git", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		dir := t.TempDir()
		git := &fakeGit{repo: true, remoteURL: c.remote}
		r := runRemote(t, dir, srv.URL, git, "use", "acme/crm", "--json")
		if r.Code != 0 {
			t.Fatalf("use: %+v", r)
		}
		if git.ran("remote set-url whisk https://git.whisk.run/01J/a1.git") != c.setURL || r.JSON["remote_updated"] != c.updated {
			t.Errorf("remote %q: calls %v, result %v", c.remote, git.calls, r.JSON)
		}
		if git.ran("remote add") {
			t.Errorf("use added a remote: %v", git.calls)
		}
	}
}

// whisk dev's files are never part of the deploy's commit; when they are the only change,
// nothing is committed.
func TestDeployLeavesWhiskDevOutOfTheCommit(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	git := &fakeGit{repo: true, dirty: []string{".whisk/dev/secrets.env"}, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("d-new")}
	r := runRemote(t, dir, srv.URL, git, "deploy", "--force")
	if r.Code != 0 {
		t.Fatalf("deploy: %+v", r)
	}
	if git.ran("add") || git.ran("commit") {
		t.Errorf("committed whisk dev's files: %v", git.calls)
	}
	if !strings.Contains(r.Stderr, "Leaving 1 file(s) under .whisk/dev/ out of the commit") {
		t.Errorf("no note about .whisk/dev:\n%s", r.Stderr)
	}

	// Without --commit, whisk dev's files alone do not count as uncommitted changes.
	git = &fakeGit{repo: true, dirty: []string{".whisk/dev/compose.yaml"}, sha: "abc1234abc", branch: "main", remoteURL: "https://git.whisk.run/01J/a1.git", push: pushOK("d-new")}
	if r := runRemote(t, dir, srv.URL, git, "deploy", "--force", "--commit=false"); r.Code != 0 {
		t.Fatalf("deploy --commit=false: %+v", r)
	}
}
