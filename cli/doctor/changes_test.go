package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/contract/custom"
)

const warehouseRecord = "asked: When a deal is won, tell the warehouse.\ntouches:\n  - custom/server/warehouse.ts\n  - custom/server/test/\nchecks:\n  - custom/server/test/warehouse.test.ts\n"

// The rules of a fork's changes, from a checkout as Load reads it.
func TestForkRules(t *testing.T) {
	fork := func(changed []string, added ...string) *Fork {
		return &Fork{Release: "CRM release 7", Base: "0123456789abcdef0123", Changed: changed, Added: added}
	}
	files := func(paths ...string) []File {
		out := []File{{Path: "custom/changes/tell-the-warehouse.yaml", Content: []byte(warehouseRecord)}}
		for _, p := range paths {
			out = append(out, File{Path: p, Content: []byte("x")})
		}
		return out
	}
	ours := []string{"custom/server/warehouse.ts", "custom/server/test/warehouse.test.ts"}
	type found struct{ rule, file, message string }
	for _, tc := range []struct {
		name  string
		repo  Repo
		want  []found
		skips int
	}{
		{"a checkout that follows no release line", Repo{Files: files("migrations/0033_regions.sql")}, nil, 0},
		{"git could not read it", Repo{ForkErr: "git diff: fatal: bad object"}, nil, 4},
		{"a branch holding none of the releases", Repo{Fork: &Fork{}}, nil, 4},
		{"each change in a record", Repo{Fork: fork(append(ours, "custom/changes/tell-the-warehouse.yaml"), ours...), Files: files(ours...)}, nil, 0},
		{"a file in no record", Repo{Fork: fork(append(ours, "src/deals/DealShow.tsx")), Files: files(append(ours, "src/deals/DealShow.tsx")...)},
			[]found{{"W170", "src/deals/DealShow.tsx", "src/deals/DealShow.tsx differs from CRM release 7 and no change in custom/changes names it."}}, 0},
		{"many files in no record", Repo{Fork: fork([]string{"a.ts", "b.ts", "c.ts", "d.ts", "e.ts", "f.ts", "g.ts", "h.ts", "i.ts", "j.ts", "k.ts", "l.ts"}), Files: files("custom/server/test/warehouse.test.ts")},
			[]found{
				{"W170", "", "12 files differ from CRM release 7 and no change in custom/changes names them: a.ts, b.ts, c.ts, d.ts, e.ts, f.ts, g.ts, h.ts, i.ts, j.ts and 2 more."},
				{"W173", "custom/changes/tell-the-warehouse.yaml", "Everything custom/changes/tell-the-warehouse.yaml names is as CRM release 7 has it, so the release holds the change now or it was undone."}}, 0},
		{"a record that cannot be read, and a file that is not one",
			Repo{Fork: fork(nil), Files: []File{
				{Path: "custom/changes/regions.yaml", Content: []byte("asked: Regions on companies.\ntouches: [src/a.ts]\nchecks: [/tmp/a.test.ts]\nby: Jane\n")},
				{Path: "custom/changes/Notes.md", Content: []byte("# notes")}}},
			[]found{
				{"W171", "custom/changes/Notes.md", "custom/changes/Notes.md is not a record: custom/changes holds one file a change, named for it in lower-case letters, digits and dashes and ending .yaml."},
				{"W171", "custom/changes/regions.yaml", `custom/changes/regions.yaml checks has "/tmp/a.test.ts", which is not a path from the code's root, such as custom/server/warehouse.ts.`},
				{"W171", "custom/changes/regions.yaml", "custom/changes/regions.yaml has by, which a record does not take: it takes asked, touches and checks."}}, 0},
		{"a check that is not there, or not a test",
			Repo{Fork: fork([]string{"src/a.ts"}), Files: []File{
				{Path: "custom/changes/regions.yaml", Content: []byte("asked: Regions on companies.\ntouches: [src/a.ts]\nchecks: [src/a.ts, custom/server/test/regions.test.ts]\n")},
				{Path: "src/a.ts", Content: []byte("x")}}},
			[]found{
				{"W171", "custom/changes/regions.yaml", "custom/changes/regions.yaml names the check src/a.ts, which doctor does not read as a test: a check is a test the app's tests run."},
				{"W171", "custom/changes/regions.yaml", "custom/changes/regions.yaml names the check custom/server/test/regions.test.ts, which is not in the repository."}}, 0},
		{"Whisk's SQL changed, deleted, and SQL of the business's own beside it",
			Repo{Fork: fork(append([]string{"migrations/0007_changes.sql", "migrations/0009_old.sql", "migrations/0033_regions.sql", "custom/migrations/001_regions.sql"}, ours...), append([]string{"migrations/0033_regions.sql", "custom/migrations/001_regions.sql"}, ours...)...),
				Files: append(files(ours...), File{Path: "migrations/0007_changes.sql"}, File{Path: "migrations/0033_regions.sql"}, File{Path: "custom/migrations/001_regions.sql"},
					File{Path: "custom/changes/regions.yaml", Content: []byte("asked: Regions on companies.\ntouches: [migrations/, custom/migrations/001_regions.sql]\nchecks: [custom/server/test/warehouse.test.ts]\n")})},
			[]found{
				{"W172", "migrations/0007_changes.sql", "migrations/0007_changes.sql is Whisk's SQL and differs from CRM release 7; git checkout 0123456789ab -- migrations/0007_changes.sql gives it back as the release has it."},
				{"W172", "migrations/0009_old.sql", "migrations/0009_old.sql is Whisk's SQL and is gone from this copy; git checkout 0123456789ab -- migrations/0009_old.sql gives it back as CRM release 7 has it."},
				{"W172", "migrations/0033_regions.sql", "migrations/0033_regions.sql is SQL of the business's own outside custom/, which CRM release 7 does not have; git rm migrations/0033_regions.sql takes it out once what it does is in custom/migrations or custom/views."}}, 0},
		{"a record naming some files as the release has them",
			Repo{Fork: fork(ours, ours...), Files: append(files(ours...), File{Path: "custom/changes/regions.yaml",
				Content: []byte("asked: Regions on companies.\ntouches: [custom/server/test/, src/deals/DealShow.tsx, src/deals/]\nchecks: [custom/server/test/warehouse.test.ts]\n")})},
			[]found{{"W173", "custom/changes/regions.yaml", "custom/changes/regions.yaml names src/deals/DealShow.tsx and src/deals/, which are as CRM release 7 has them."}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []found
			skips := 0
			for _, rule := range []func(Repo, Context) outcome{w170, w171, w172, w173} {
				o := rule(tc.repo, Context{})
				for _, f := range o.findings {
					got = append(got, found{f.Rule, f.File, f.Message})
					if f.Fix == "" {
						t.Errorf("%s has no fix", f.Rule)
					}
				}
				skips += len(o.skipped)
			}
			if !reflect.DeepEqual(got, tc.want) || skips != tc.skips {
				t.Errorf("findings\n%q\nwant\n%q\nskips %d, want %d", got, tc.want, skips, tc.skips)
			}
		})
	}
}

// gitRepo is a repository in a new directory, and git run there as a user with no configuration.
func gitRepo(t *testing.T, files map[string]string) (string, func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := writeAll(t, files)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Whisk", "GIT_AUTHOR_EMAIL=noreply@whisk.run", "GIT_COMMITTER_NAME=Whisk", "GIT_COMMITTER_EMAIL=noreply@whisk.run")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet", "--initial-branch=main")
	return dir, git
}

// A checkout of a fork, as whisk clone leaves one: the release line on the remote whisk's branch
// upstream, and the business's branch holding a release of it.
func TestReadFork(t *testing.T) {
	dir, git := gitRepo(t, with(passing, map[string]string{"migrations/0001_init.sql": "create table deals (id int);\n", "src/old.ts": "export {};\n"}))
	ctx := context.Background()
	if f, err := ReadFork(ctx, dir); f != nil || err != nil {
		t.Fatalf("an app that follows no release line: %+v, %v", f, err)
	}
	git("add", "--all")
	git("commit", "--quiet", "-m", "first")
	git("update-ref", "refs/remotes/whisk/upstream", "HEAD")
	if f, err := ReadFork(ctx, dir); f != nil || err != nil {
		t.Fatalf("a branch named upstream that is not a release line: %+v, %v", f, err)
	}
	someone := exec.Command("git", "commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "Shop release 2")
	someone.Dir = dir
	someone.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Jane Doe", "GIT_AUTHOR_EMAIL=jane@example.com", "GIT_COMMITTER_NAME=Jane Doe", "GIT_COMMITTER_EMAIL=jane@example.com")
	theirs, err := someone.Output()
	if err != nil {
		t.Fatal(err)
	}
	git("update-ref", "refs/remotes/whisk/upstream", strings.TrimSpace(string(theirs)))
	if f, err := ReadFork(ctx, dir); f != nil || err != nil {
		t.Fatalf("a release someone but Whisk made: %+v, %v", f, err)
	}
	git("commit", "--quiet", "--allow-empty", "-m", "CRM release 1")
	release := git("rev-parse", "HEAD")
	git("update-ref", "refs/remotes/whisk/upstream", release)
	f, err := ReadFork(ctx, dir)
	if err != nil || f == nil || f.Release != "CRM release 1" || f.Base != release || len(f.Changed) != 0 {
		t.Fatalf("a release as Whisk made it: %+v, %v", f, err)
	}

	// The business's own changes, committed and not, and what git ignores.
	writeFile(t, dir, "custom/server/warehouse.ts", "export default {};\n")
	writeFile(t, dir, "src/index.ts", passing["src/index.ts"]+"// changed\n")
	git("add", "--all")
	git("commit", "--quiet", "-m", "tell the warehouse")
	writeFile(t, dir, "custom/web/stock.tsx", "export default {};\n")
	writeFile(t, dir, ".env", "API_KEY=value\n")
	if err := os.Remove(filepath.Join(dir, "src/old.ts")); err != nil {
		t.Fatal(err)
	}
	f, err = ReadFork(ctx, dir)
	if err != nil || f == nil || f.Release != "CRM release 1" || f.Base != release {
		t.Fatalf("a changed copy: %+v, %v", f, err)
	}
	if want := []string{"custom/server/warehouse.ts", "src/index.ts", "src/old.ts", "custom/web/stock.tsx"}; !reflect.DeepEqual(f.Changed, want) {
		t.Errorf("changed = %q, want %q", f.Changed, want)
	}
	if want := []string{"custom/server/warehouse.ts", "custom/web/stock.tsx"}; !reflect.DeepEqual(f.Added, want) {
		t.Errorf("added = %q, want %q", f.Added, want)
	}

	// A newer release on the line that the branch has not merged leaves the one it holds.
	two := git("commit-tree", release+"^{tree}", "-p", release, "-m", "CRM release 2")
	git("update-ref", "refs/remotes/whisk/upstream", two)
	if f, err = ReadFork(ctx, dir); err != nil || f.Release != "CRM release 1" {
		t.Fatalf("a release not taken yet: %+v, %v", f, err)
	}

	// A release the branch merged is the one it holds, though the remote-tracking upstream is still
	// where the last fetch left it, as git pull leaves it.
	git("merge", "--quiet", "--no-edit", two)
	git("update-ref", "refs/remotes/whisk/upstream", release)
	if f, err = ReadFork(ctx, dir); err != nil || f.Release != "CRM release 2" || f.Base != two {
		t.Fatalf("a release merged ahead of the remote-tracking branch: %+v, %v", f, err)
	}

	// A branch that shares nothing with the line holds none of its releases.
	git("update-ref", "refs/remotes/whisk/upstream", git("commit-tree", release+"^{tree}", "-m", "CRM release 3"))
	if f, err = ReadFork(ctx, dir); err != nil || f == nil || f.Base != "" {
		t.Fatalf("a branch apart from the line: %+v, %v", f, err)
	}
}

// The release a branch holds is the commit Whisk made with the highest number its history has,
// whatever order git printed them in and whatever the product was called then.
func TestNewestRelease(t *testing.T) {
	for _, c := range []struct {
		log, base, subject string
	}{
		{"", "", ""},
		{"a\x00Jane Doe\x00tell the warehouse", "", ""},
		{"a\x00Whisk\x00CRM release 3\nb\x00Whisk\x00CRM release 2", "a", "CRM release 3"},
		{"b\x00Whisk\x00CRM release 2\na\x00Whisk\x00Sales CRM release 3", "a", "Sales CRM release 3"},
		{"c\x00Jane Doe\x00CRM release 9\nb\x00Whisk\x00CRM release 2\nd\x00Whisk\x00Merge release 4 of CRM", "b", "CRM release 2"},
	} {
		if base, subject := newestRelease(c.log); base != c.base || subject != c.subject {
			t.Errorf("newestRelease(%q) = %q, %q; want %q, %q", c.log, base, subject, c.base, c.subject)
		}
	}
}

// A file of custom/changes that Load kept only the text of stands in as Whisk reads it: too big,
// or not UTF-8 text.
func TestRecordContent(t *testing.T) {
	for _, c := range []struct {
		name string
		f    File
		want string
	}{
		{"a record", File{Size: 10, Content: []byte(warehouseRecord)}, ""},
		{"too big", File{Size: 5 << 20}, "is over 64 KiB, and a record says what was asked in a few sentences"},
		{"binary", File{Size: 10, Binary: true}, "is not UTF-8 text"},
		{"UTF-16", File{Size: 10, UTF16: true, Content: []byte(warehouseRecord)}, "is not UTF-8 text"},
	} {
		_, ps := custom.ParseChange("tell-the-warehouse", recordContent(c.f))
		got := ""
		if len(ps) > 0 {
			got = ps[0].Message
		}
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// Through Run, as whisk deploy's gate does: a change with no record stops it until the record is
// there.
func TestForkRulesRun(t *testing.T) {
	dir, git := gitRepo(t, passing)
	git("add", "--all")
	git("commit", "--quiet", "-m", "CRM release 4")
	git("update-ref", "refs/remotes/whisk/upstream", "HEAD")
	writeFile(t, dir, "custom/server/warehouse.ts", "export default {};\n")
	writeFile(t, dir, "custom/server/test/warehouse.test.ts", "it('tells the warehouse', () => {});\n")
	rep, err := Run(context.Background(), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !has(rep, "W170") || rep.OK() {
		t.Fatalf("a change with no record: %+v", rep.Findings)
	}
	writeFile(t, dir, "custom/changes/tell-the-warehouse.yaml", warehouseRecord)
	if rep, err = Run(context.Background(), Options{Dir: dir}); err != nil || !rep.OK() || len(rep.Findings) != 0 {
		t.Fatalf("a change with its record: %+v, %v", rep.Findings, err)
	}
}

func writeFile(t *testing.T, dir, p, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
