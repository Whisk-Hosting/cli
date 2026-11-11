package whisk

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/custom"
)

// mergeRepos is a fork's repository on Whisk, standing in as the remote whisk, and a checkout of
// it on the business's branch, which holds release 2 and a change of the business's own.
type mergeRepos struct {
	remote, dir string
	t           *testing.T
}

func (r mergeRepos) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Whisk", "GIT_AUTHOR_EMAIL=noreply@whisk.run", "GIT_COMMITTER_NAME=Whisk", "GIT_COMMITTER_EMAIL=noreply@whisk.run")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// release puts release n of the CRM on the remote's upstream branch, with files as given.
func (r mergeRepos) release(n int, files map[string]string) {
	r.t.Helper()
	for p, body := range files {
		put(r.t, r.remote, p, body)
	}
	r.git(r.remote, "add", "--all")
	r.git(r.remote, "commit", "--quiet", "-m", custom.LineSubject("CRM", n))
}

const wonV2 = "export const won = (deal) => {\n  record(deal);\n  return deal;\n};\n"

func newMergeRepos(t *testing.T, apiURL string) mergeRepos {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	r := mergeRepos{remote: t.TempDir(), dir: boundDir(t, apiURL), t: t}
	r.git(r.remote, "init", "--quiet", "--initial-branch=upstream")
	r.release(1, map[string]string{"src/index.ts": "export {};\n", "src/deals/won.ts": "export const won = (deal) => deal;\n"})
	r.release(2, map[string]string{"src/deals/won.ts": wonV2})

	r.git(r.dir, "init", "--quiet", "--initial-branch=main")
	r.git(r.dir, "remote", "add", "whisk", r.remote)
	r.git(r.dir, "fetch", "--quiet", "whisk", "+refs/heads/upstream:refs/remotes/whisk/upstream")
	r.git(r.dir, "reset", "--quiet", "--hard", "refs/remotes/whisk/upstream")
	put(t, r.dir, ".gitignore", ".whisk/\n")
	put(t, r.dir, "custom/server/warehouse.ts", "export const tell = () => {};\n")
	put(t, r.dir, "custom/server/test/warehouse.test.ts", "it('tells the warehouse', () => {});\n")
	put(t, r.dir, "src/deals/won.ts", "export const won = (deal) => {\n  record(deal);\n  return deal;\n};\ntell();\n")
	put(t, r.dir, "custom/changes/tell-the-warehouse.yaml", "asked: When a deal is won, tell the warehouse.\n"+
		"touches: [custom/server/warehouse.ts, custom/server/test/warehouse.test.ts, src/deals/won.ts]\nchecks: [custom/server/test/warehouse.test.ts]\n")
	put(t, r.dir, "custom/web/region.tsx", "export default {};\n")
	put(t, r.dir, "custom/changes/company-region.yaml", "asked: Show each company's region.\ntouches: [custom/web/region.tsx]\nchecks: [custom/server/test/warehouse.test.ts]\n")
	r.git(r.dir, "add", "--all")
	r.git(r.dir, "commit", "--quiet", "-m", "tell the warehouse, show the region")
	return r
}

// A release that changed a file of a change merges in, naming that change with what it asked for
// and its checks and carrying the other over; asked again, there is nothing to merge.
func TestManagedMerge(t *testing.T) {
	srv := forkAPI(t, &forkState{release: 2, changed: true})
	defer srv.Close()
	r := newMergeRepos(t, srv.URL)
	put(t, r.dir, "src/index.ts", "export {};\nexport const ours = 1;\n")
	put(t, r.dir, "custom/server/warehouse.ts", "export const tell = (deal) => {\n  post(deal.won_at);\n};\n")
	r.git(r.dir, "commit", "--quiet", "--all", "-m", "a change with no record, and the warehouse told when")
	r.release(3, map[string]string{"migrations/0003_stages.sql": "alter table deals rename column won_at to closed_at;\n",
		"src/deals/won.ts": "// Release 3: a won deal is recorded once.\n" + wonV2, "src/index.ts": "// Release 3\nexport {};\n",
		"moved/deal-won-at.yaml": "moved:\n  - from: deals.won_at\n    to: deals.closed_at\n  - from: record\nvalues: Every date is kept.\nfix: Read closed_at where you read won_at.\n"})

	r.git(r.dir, "switch", "--quiet", "--detach")
	if got := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm"); got.Code == 0 || !strings.Contains(got.Stderr, "This checkout is on no branch: its HEAD is detached.") {
		t.Errorf("detached: %+v", got)
	}
	r.git(r.dir, "switch", "--quiet", "main")

	got := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm", "--json")
	if got.Code != 0 || got.JSON["merged"] != true || got.JSON["release"] != "CRM release 3" || got.JSON["from"] != "CRM release 2" {
		t.Fatalf("merge: %+v", got)
	}
	// The note names record too, which only Whisk's lines of won.ts use: the business's own lines
	// are the ones looked through.
	met := []any{map[string]any{"name": "tell-the-warehouse", "asked": "When a deal is won, tell the warehouse.", "files": []any{"src/deals/won.ts"},
		"moved": []any{map[string]any{"id": "deal-won-at", "moved": []any{map[string]any{"from": "deals.won_at", "to": "deals.closed_at"}, map[string]any{"from": "record"}},
			"values": "Every date is kept.", "fix": "Read closed_at where you read won_at.",
			"uses": []any{map[string]any{"file": "custom/server/warehouse.ts", "line": float64(2), "text": "post(deal.won_at);"}}}},
		"checks": []any{"custom/server/test/warehouse.test.ts"}}}
	if !reflect.DeepEqual(got.JSON["met"], met) || !reflect.DeepEqual(got.JSON["carried"], []any{"company-region"}) ||
		!reflect.DeepEqual(got.JSON["checks"], []any{"custom/server/test/warehouse.test.ts"}) || !reflect.DeepEqual(got.JSON["unrecorded"], []any{"src/index.ts"}) {
		t.Errorf("met %v, carried %v, checks %v, unrecorded %v", got.JSON["met"], got.JSON["carried"], got.JSON["checks"], got.JSON["unrecorded"])
	}
	if subject := r.git(r.dir, "log", "-1", "--format=%s"); subject != "Merge release 3 of CRM" || custom.IsLineSubject(subject) {
		t.Errorf("the merge commit says %q", subject)
	}
	if body, _ := os.ReadFile(filepath.Join(r.dir, "src/deals/won.ts")); !strings.HasPrefix(string(body), "// Release 3") || !strings.HasSuffix(string(body), "tell();\n") {
		t.Errorf("won.ts after the merge:\n%s", body)
	}

	again := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm")
	if again.Code != 0 || !strings.Contains(again.Stdout, "This branch holds CRM release 3, the newest release: there is nothing to merge.") {
		t.Errorf("merging again: %+v", again)
	}
}

// A conflict leaves the merge to settle and names the change each file belongs to; nothing
// uncommitted, and no merge under way, is ever merged into.
func TestManagedMergeConflict(t *testing.T) {
	srv := forkAPI(t, &forkState{release: 2, changed: true})
	defer srv.Close()
	r := newMergeRepos(t, srv.URL)
	put(t, r.dir, "src/index.ts", "export const ours = 1;\n")
	put(t, r.dir, "custom/web/region.tsx", "export default { read: (c) => c.region };\n")
	r.git(r.dir, "commit", "--quiet", "--all", "-m", "a change with no record, and the region read")
	r.release(3, map[string]string{"src/deals/won.ts": "export const won = (deal) => {\n  record(deal);\n  return deal;\n};\nnotify();\n",
		"src/index.ts": "export const theirs = 1;\n", "moved/company-area.yaml": "moved:\n  - from: companies.region\n    to: companies.area\nfix: Read area where you read region.\n"})

	put(t, r.dir, "notes.txt", "a note\n")
	if got := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm", "--json"); got.Code == 0 || !strings.Contains(got.Stdout, "1 file(s) have uncommitted changes: notes.txt.") {
		t.Fatalf("uncommitted: %+v", got)
	}
	must(t, os.Remove(filepath.Join(r.dir, "notes.txt")))

	human := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm")
	if human.Code == 0 || !strings.Contains(human.Stderr, "Merging CRM release 3 left 2 file(s) to settle: src/deals/won.ts, src/index.ts.") ||
		!strings.Contains(human.Stderr, "src/deals/won.ts belongs to tell-the-warehouse: When a deal is won, tell the warehouse. Checked by custom/server/test/warehouse.test.ts.") ||
		!strings.Contains(human.Stderr, "src/index.ts was changed here, and no record in custom/changes names it.") ||
		!strings.Contains(human.Stderr, "company-region uses what CRM release 3 moved:\n  company-area: companies.region went to companies.area. Read area where you read region.\n"+
			"    custom/web/region.tsx:1  export default { read: (c) => c.region };\n") {
		t.Fatalf("conflict: %+v", human)
	}
	if again := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm"); again.Code == 0 || !strings.Contains(again.Stderr, "A merge is under way in this checkout.") {
		t.Errorf("a merge under way: %+v", again)
	}
	abort := func() {
		t.Helper()
		r.git(r.dir, "merge", "--abort")
		if status := r.git(r.dir, "status", "--porcelain"); status != "" {
			t.Fatalf("after git merge --abort: %q", status)
		}
	}
	abort()

	got := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm", "--json")
	e, _ := got.JSON["error"].(map[string]any)
	if got.Code == 0 || e["code"] != "MERGE_CONFLICT" || e["message"] != "Merging CRM release 3 left 2 file(s) to settle: src/deals/won.ts, src/index.ts." {
		t.Fatalf("conflict, as JSON: %+v", got)
	}
	details, _ := e["details"].(map[string]any)
	warehouse := map[string]any{"name": "tell-the-warehouse", "asked": "When a deal is won, tell the warehouse.",
		"files": []any{"src/deals/won.ts"}, "moved": []any{}, "checks": []any{"custom/server/test/warehouse.test.ts"}}
	want := []any{map[string]any{"file": "src/deals/won.ts", "changes": []any{warehouse}}, map[string]any{"file": "src/index.ts", "changes": []any{}}}
	region := map[string]any{"name": "company-region", "asked": "Show each company's region.", "files": []any{},
		"moved": []any{map[string]any{"id": "company-area", "moved": []any{map[string]any{"from": "companies.region", "to": "companies.area"}}, "fix": "Read area where you read region.",
			"uses": []any{map[string]any{"file": "custom/web/region.tsx", "line": float64(1), "text": "export default { read: (c) => c.region };"}}}},
		"checks": []any{"custom/server/test/warehouse.test.ts"}}
	if !reflect.DeepEqual(details["conflicts"], want) || !reflect.DeepEqual(details["met"], []any{region, warehouse}) || !reflect.DeepEqual(details["unrecorded"], []any{"src/index.ts"}) ||
		details["release"] != "CRM release 3" || details["from"] != "CRM release 2" {
		t.Errorf("details = %v", details)
	}
	abort()
}

// A branch with nothing of its own since its release takes the next one as it is, by fast-forward.
func TestManagedMergeFastForward(t *testing.T) {
	srv := forkAPI(t, &forkState{release: 2})
	defer srv.Close()
	r := newMergeRepos(t, srv.URL)
	r.git(r.dir, "reset", "--quiet", "--hard", "refs/remotes/whisk/upstream")
	must(t, os.WriteFile(filepath.Join(r.dir, ".git", "info", "exclude"), []byte(".whisk/\n"), 0o644))
	r.release(3, map[string]string{"src/deals/won.ts": "// Release 3\n" + wonV2})

	got := runRemote(t, r.dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm", "--json")
	if got.Code != 0 || got.JSON["merged"] != true || got.JSON["release"] != "CRM release 3" || got.JSON["from"] != "CRM release 2" {
		t.Fatalf("merge: %+v", got)
	}
	if head, release := r.git(r.dir, "rev-parse", "HEAD"), r.git(r.remote, "rev-parse", "upstream"); head != release || got.JSON["commit"] != head {
		t.Errorf("HEAD %s, release 3 %s, answered %v", head, release, got.JSON["commit"])
	}
}

// Outside a fork's checkout there is nothing to merge into.
func TestManagedMergeOutsideAFork(t *testing.T) {
	srv := forkAPI(t, &forkState{release: 2, changed: true})
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	if got := runRemote(t, dir, srv.URL, nil, "managed", "merge", "--org", "acme", "--app", "crm"); got.Code == 0 || !strings.Contains(got.Stdout+got.Stderr, "does not follow one of Whisk's products") {
		t.Errorf("outside a fork: %+v", got)
	}
}

func TestMergePlan(t *testing.T) {
	warehouse := custom.Change{Name: "tell-the-warehouse", Asked: "Tell the warehouse.", Touches: []string{"src/deals/won.ts", "custom/server/"}, Checks: []string{"custom/server/test/a.test.ts", "custom/server/test/b.test.ts"}}
	region := custom.Change{Name: "company-region", Asked: "Show the region.", Touches: []string{"src/companies/"}, Checks: []string{"custom/server/test/a.test.ts"}}
	page := custom.Change{Name: "own-page", Asked: "A page of our own.", Touches: []string{"custom/web/page.tsx"}, Checks: []string{"custom/web/page.test.tsx"}}
	quiet := custom.Change{Name: "quiet", Asked: "Nothing the release meets.", Touches: []string{"custom/web/quiet.tsx"}, Checks: []string{"custom/web/quiet.test.tsx"}}
	phones := custom.Moved{ID: "company-phones-as-list", Moves: []custom.Move{{From: "companies.phone_number", To: "companies.phone_jsonb"}}, Fix: "Read phone_jsonb."}
	lines := map[string][]custom.Line{
		"custom/web/page.tsx":  {{Number: 3, Text: "<p>{company.phone_number}</p>"}},
		"custom/web/quiet.tsx": {{Number: 1, Text: "export default {};"}},
		"src/deals/won.ts":     {{Number: 5, Text: "tell();"}},
	}
	j := mergePlan([]custom.Change{warehouse, region, page, quiet}, []string{"src/index.ts", "src/app.ts"},
		[]string{"src/companies/list.tsx", "src/deals/won.ts", "src/index.ts", "migrations/0032.sql", "moved/company-phones-as-list.yaml"}, []custom.Moved{phones}, lines)
	want := []metChange{
		{"tell-the-warehouse", "Tell the warehouse.", []string{"src/deals/won.ts"}, []metNote{}, warehouse.Checks},
		{"company-region", "Show the region.", []string{"src/companies/list.tsx"}, []metNote{}, region.Checks},
		{"own-page", "A page of our own.", []string{}, []metNote{{phones, []custom.Use{{File: "custom/web/page.tsx", Line: 3, Text: "<p>{company.phone_number}</p>"}}}}, page.Checks},
	}
	if !reflect.DeepEqual(j.Met, want) {
		t.Errorf("met = %+v", j.Met)
	}
	if !reflect.DeepEqual(j.Carried, []string{"quiet"}) || !reflect.DeepEqual(j.Checks, []string{"custom/server/test/a.test.ts", "custom/server/test/b.test.ts", "custom/web/page.test.tsx"}) ||
		!reflect.DeepEqual(j.Unrecorded, []string{"src/index.ts"}) {
		t.Errorf("carried %q, checks %q, unrecorded %q", j.Carried, j.Checks, j.Unrecorded)
	}
	if j := mergePlan(nil, nil, []string{"src/a.ts"}, nil, nil); j.Met == nil || j.Carried == nil || j.Checks == nil || j.Unrecorded == nil || len(j.Met)+len(j.Carried)+len(j.Checks)+len(j.Unrecorded) != 0 {
		t.Errorf("no changes: %+v", j)
	}
	conflicts := conflictsOf([]string{"src/deals/won.ts", "custom/web/page.tsx", "migrations/0001_init.sql"}, []custom.Change{warehouse, region, page}, j)
	if len(conflicts) != 3 || len(conflicts[0].Changes) != 1 || conflicts[0].Changes[0].Name != "tell-the-warehouse" || conflicts[0].Changes[0].Moved == nil ||
		!reflect.DeepEqual(conflicts[1].Changes[0].Moved, want[2].Moved) || conflicts[2].Changes == nil || len(conflicts[2].Changes) != 0 {
		t.Errorf("conflicts = %+v", conflicts)
	}
}

// The lines a diff adds, by their numbers in the file as it is now.
func TestDiffAdded(t *testing.T) {
	diff := "diff --git a/src/deals/won.ts b/src/deals/won.ts\nindex 1..2 100644\n--- a/src/deals/won.ts\n+++ b/src/deals/won.ts\n" +
		"@@ -4,0 +5 @@ export const won\n+tell();\n@@ -9,2 +10,3 @@\n-old();\n-older();\n+one();\n+++two;\n+three();\n"
	want := []custom.Line{{Number: 5, Text: "tell();"}, {Number: 10, Text: "one();"}, {Number: 11, Text: "++two;"}, {Number: 12, Text: "three();"}}
	if got := diffAdded(diff); !reflect.DeepEqual(got, want) {
		t.Errorf("diffAdded = %+v", got)
	}
	if hunkStart("@@ -1 +0,0 @@") != 1 || hunkStart("@@ bad") != 0 || diffAdded("") != nil {
		t.Error("hunkStart or an empty diff")
	}
}

func TestPrintMerged(t *testing.T) {
	var b strings.Builder
	note := metNote{custom.Moved{ID: "deal-won-at", Moves: []custom.Move{{From: "deals.won_at", To: "deals.closed_at"}, {From: "record"}}, Values: "Every date is kept.", Fix: "Read closed_at."},
		[]custom.Use{{File: "custom/server/warehouse.ts", Line: 1, Text: "post(deal.won_at);"}}}
	j := mergeJoin{Met: []metChange{{"tell-the-warehouse", "Tell the warehouse.", []string{"src/deals/won.ts"}, []metNote{note}, []string{"custom/server/test/a.test.ts"}}},
		Carried: []string{"company-region"}, Checks: []string{"custom/server/test/a.test.ts"}, Unrecorded: []string{"src/index.ts"}}
	printMerged(output.Printer{}, &b, "CRM release 3", "CRM release 2", j)
	want := "Merged CRM release 3, after CRM release 2. 1 of the business's 2 changes touches files the release changed too:\n\n" +
		"tell-the-warehouse\n  Tell the warehouse.\n  the release changed src/deals/won.ts\n" +
		"  deal-won-at: deals.won_at went to deals.closed_at and record went away. Every date is kept. Read closed_at.\n" +
		"    custom/server/warehouse.ts:1  post(deal.won_at);\n" +
		"  checked by custom/server/test/a.test.ts\n\n" +
		"No record names src/index.ts, which the release and this branch both changed: whisk changes draft <name> writes one.\n\n" +
		"Next: run custom/server/test/a.test.ts and the app's tests, then whisk deploy, which tries the release's migrations on a copy of the live data first.\n"
	if b.String() != want {
		t.Errorf("printed:\n%s\nwant:\n%s", b.String(), want)
	}
	b.Reset()
	printMerged(output.Printer{}, &b, "CRM release 3", "CRM release 2", mergeJoin{Carried: []string{"company-region"}})
	if !strings.HasPrefix(b.String(), "Merged CRM release 3, after CRM release 2. The business's 1 change touches no file the release changed.\n") {
		t.Errorf("none met:\n%s", b.String())
	}
	b.Reset()
	many := note
	for i := 2; i <= 12; i++ {
		many.Uses = append(many.Uses, custom.Use{File: "a.ts", Line: i, Text: "won_at"})
	}
	printNote(output.Printer{}, &b, many)
	if !strings.HasSuffix(b.String(), "    a.ts:10  won_at\n    and 2 more\n") {
		t.Errorf("many uses:\n%s", b.String())
	}
}
