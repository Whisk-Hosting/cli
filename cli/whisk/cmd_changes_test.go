package whisk

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/contract/custom"
)

// forkCheckout is a checkout of a fork as whisk clone leaves one, at a release of crm as Whisk
// made it, and git run there.
func forkCheckout(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Whisk", "GIT_AUTHOR_EMAIL=noreply@whisk.run", "GIT_COMMITTER_NAME=Whisk", "GIT_COMMITTER_EMAIL=noreply@whisk.run")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	put(t, dir, "src/index.ts", "export {};\n")
	git("add", "--all")
	git("commit", "--quiet", "-m", "CRM release 3")
	git("update-ref", "refs/remotes/whisk/upstream", "HEAD")
	return dir, git
}

func put(t *testing.T, dir, p, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(p))
	must(t, os.MkdirAll(filepath.Dir(full), 0o755))
	must(t, os.WriteFile(full, []byte(content), 0o644))
}

func TestChanges(t *testing.T) {
	if r := runCLI(t, t.TempDir(), nil, &memStore{}, "changes", "--json"); r.Code == 0 || !strings.Contains(r.Stdout, "does not follow one of Whisk's products") {
		t.Fatalf("outside a fork: %+v", r)
	}
	dir, git := forkCheckout(t)
	r := runCLI(t, dir, nil, &memStore{}, "changes")
	if r.Code != 0 || !strings.Contains(r.Stdout, "This copy is CRM release 3 as Whisk made it, with no changes of its own.") {
		t.Fatalf("a release as Whisk made it: %+v", r)
	}

	put(t, dir, "custom/server/warehouse.ts", "export default {};\n")
	put(t, dir, "custom/server/test/warehouse.test.ts", "it('tells the warehouse', () => {});\n")
	put(t, dir, "src/index.ts", "export const changed = true;\n")
	git("add", "custom/server/warehouse.ts")
	r = runCLI(t, dir, nil, &memStore{}, "changes", "--json")
	if r.Code != 0 || r.JSON["release"] != "CRM release 3" || !reflect.DeepEqual(r.JSON["uncovered"], []any{"custom/server/test/warehouse.test.ts", "custom/server/warehouse.ts", "src/index.ts"}) {
		t.Fatalf("files no record names: %+v", r)
	}

	// The draft names every such file, and the tests among them as its checks, and writes nothing
	// until it can say what was asked.
	r = runCLI(t, dir, nil, &memStore{}, "changes", "draft", "tell-the-warehouse", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "--asked is not there") {
		t.Fatalf("draft without --asked: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/changes")); !os.IsNotExist(err) {
		t.Fatalf("a refused draft wrote: %v", err)
	}
	r = runCLI(t, dir, nil, &memStore{}, "changes", "draft", "tell-the-warehouse", "--asked", " When a deal is won, tell the warehouse. ")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Wrote custom/changes/tell-the-warehouse.yaml: tell-the-warehouse, naming 3 files and checked by custom/server/test/warehouse.test.ts.") {
		t.Fatalf("draft: %+v", r)
	}
	src, err := os.ReadFile(filepath.Join(dir, "custom/changes/tell-the-warehouse.yaml"))
	must(t, err)
	want := "asked: When a deal is won, tell the warehouse.\ntouches:\n  - custom/server/test/warehouse.test.ts\n  - custom/server/warehouse.ts\n  - src/index.ts\nchecks:\n  - custom/server/test/warehouse.test.ts\n"
	if string(src) != want {
		t.Fatalf("record:\n%s\nwant:\n%s", src, want)
	}
	if c, ps := custom.ParseChange("tell-the-warehouse", src); len(ps) != 0 || c.Asked != "When a deal is won, tell the warehouse." || len(c.Touches) != 3 {
		t.Fatalf("record read back: %+v %+v", c, ps)
	}

	// A later file joins the record it is drafted into, which keeps what it said.
	put(t, dir, "custom/web/stock.tsx", "export default {};\n")
	r = runCLI(t, dir, nil, &memStore{}, "changes", "draft", "tell-the-warehouse", "--json")
	if r.Code != 0 || !reflect.DeepEqual(r.JSON["named"], []any{"custom/web/stock.tsx"}) {
		t.Fatalf("a later file: %+v", r)
	}
	r = runCLI(t, dir, nil, &memStore{}, "changes", "--json")
	changes, _ := r.JSON["changes"].([]any)
	if r.Code != 0 || len(changes) != 1 || len(r.JSON["uncovered"].([]any)) != 0 {
		t.Fatalf("listed: %+v", r)
	}
	if c := changes[0].(map[string]any); c["asked"] != "When a deal is won, tell the warehouse." || len(c["touches"].([]any)) != 4 {
		t.Fatalf("listed change: %+v", c)
	}
	r = runCLI(t, dir, nil, &memStore{}, "changes")
	if !strings.Contains(r.Stdout, "CRM release 3, with 1 change of the business's own:") || !strings.Contains(r.Stdout, "checked by custom/server/test/warehouse.test.ts") {
		t.Fatalf("listed as text: %+v", r)
	}

	for _, tc := range []struct {
		name string
		args []string
		says string
	}{
		{"a name that is not one", []string{"Tell_Warehouse"}, `Tell_Warehouse\" is not a name for a change.`},
		{"a record of nothing", []string{"regions"}, "Nothing differs from CRM release 3 that no record names, so a record of regions would name nothing."},
		{"a check that is not there", []string{"regions", "--check", "custom/server/test/regions.test.ts"}, "--check custom/server/test/regions.test.ts is not a file in this checkout."},
		{"a check that is not a test", []string{"regions", "--check", "src/index.ts"}, "--check src/index.ts is not a test"},
		{"a check that is a pattern", []string{"regions", "--check", "custom/**/*.test.ts"}, "is a pattern"},
		{"asked at length", []string{"regions", "--asked", strings.Repeat("a", custom.AskedMax+1)}, "--asked is over 1000 characters."},
	} {
		r := runCLI(t, dir, nil, &memStore{}, append([]string{"changes", "draft", "--json"}, tc.args...)...)
		if r.Code == 0 || !strings.Contains(r.Stdout, tc.says) {
			t.Errorf("%s: %+v", tc.name, r)
		}
	}
	put(t, dir, "src/regions.ts", "export {};\n")
	r = runCLI(t, dir, nil, &memStore{}, "changes", "draft", "regions", "--asked", "A region on every company.", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "None of the files is a test") {
		t.Fatalf("a draft with no test: %+v", r)
	}
	put(t, dir, "custom/changes/regions.yaml", "asked: Regions.\n")
	r = runCLI(t, dir, nil, &memStore{}, "changes", "draft", "regions", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "custom/changes/regions.yaml is there and cannot be read: it names nothing in touches") {
		t.Fatalf("a record that cannot be read: %+v", r)
	}
}

func TestDrafted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		c         custom.Change
		uncovered []string
		asked     string
		checks    []string
		want      custom.Change
	}{
		{"a new record takes the tests among its files", custom.Change{Name: "a"}, []string{"src/a.ts", "src/test/a.ts", "src/b.test.ts"}, "", nil,
			custom.Change{Name: "a", Touches: []string{"src/a.ts", "src/test/a.ts", "src/b.test.ts"}, Checks: []string{"src/test/a.ts", "src/b.test.ts"}}},
		{"given checks win", custom.Change{Name: "a"}, []string{"src/a.ts", "src/b.test.ts"}, "Do a.", []string{"src/c.test.ts"},
			custom.Change{Name: "a", Asked: "Do a.", Touches: []string{"src/a.ts", "src/b.test.ts"}, Checks: []string{"src/c.test.ts"}}},
		{"a record there keeps what it says", custom.Change{Name: "a", Asked: "Do a.", Touches: []string{"src/a.ts"}, Checks: []string{"src/a.test.ts"}}, []string{"src/b.ts", "src/b.test.ts"}, "", nil,
			custom.Change{Name: "a", Asked: "Do a.", Touches: []string{"src/a.ts", "src/b.ts", "src/b.test.ts"}, Checks: []string{"src/a.test.ts"}}},
		{"and says what was asked anew", custom.Change{Name: "a", Asked: "Do a.", Touches: []string{"src/a.ts"}, Checks: []string{"src/a.test.ts"}}, nil, "Do a, and b.", []string{"src/a.test.ts"},
			custom.Change{Name: "a", Asked: "Do a, and b.", Touches: []string{"src/a.ts"}, Checks: []string{"src/a.test.ts"}}},
	} {
		got := drafted(tc.c, tc.uncovered, tc.asked, tc.checks)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: drafted = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
