package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// A fork in words (FORKS.md §4): where its code stands against its product's releases, and what
// brings it the newest.
func TestForkWords(t *testing.T) {
	fork := func(release, newest int, changed bool) *api.AppFork {
		return &api.AppFork{Product: "crm", Name: "CRM", Branch: "upstream", Release: release, Newest: newest, Behind: max(0, newest-release), Changed: changed}
	}
	for _, c := range []struct {
		name   string
		f      *api.AppFork
		text   string
		next   string
		row    []string
		status string
	}{
		{"the newest as Whisk made it", fork(3, 3, false), "release 3, the newest, as Whisk made it", "",
			[]string{"crm-2", "crm", "3", "3", "as released", "running, fork"}, "running, fork"},
		{"an older release as Whisk made it", fork(2, 3, false), "release 2 as Whisk made it, 1 behind release 3", "whisk managed update crm-2 moves it to release 3",
			[]string{"crm-2", "crm", "2", "3", "as released", "running, fork, 1 behind"}, "running, fork, 1 behind"},
		{"the newest with the business's changes", fork(3, 3, true), "release 3, the newest, with changes of its own", "",
			[]string{"crm-2", "crm", "3", "3", "changed", "running, fork"}, "running, fork"},
		{"changed and behind", fork(1, 3, true), "release 1 with changes of its own, 2 behind release 3",
			"whisk managed merge in a checkout of its code brings release 3 in, naming each change of the business's it meets, and whisk deploy puts it live",
			[]string{"crm-2", "crm", "1", "3", "changed", "running, fork, 2 behind"}, "running, fork, 2 behind"},
		{"a history of its own", fork(0, 3, true), "code of its own that holds none of its releases",
			"git fetch whisk, then git merge whisk/upstream, brings release 3 into its code, and whisk deploy puts it live",
			[]string{"crm-2", "crm", "-", "3", "changed", "running, fork, 3 behind"}, "running, fork, 3 behind"},
		{"no code yet", fork(0, 3, false), "no code of its own yet", "whisk managed update crm-2 moves it to release 3",
			[]string{"crm-2", "crm", "-", "3", "-", "running, fork, 3 behind"}, "running, fork, 3 behind"},
	} {
		a := api.App{Slug: "crm-2", Status: "running", Fork: c.f}
		if got := forkText(c.f); got != c.text {
			t.Errorf("%s: forkText = %q, want %q", c.name, got, c.text)
		}
		if got := forkNext(a.Slug, c.f); got != c.next {
			t.Errorf("%s: forkNext = %q, want %q", c.name, got, c.next)
		}
		if got := forkRow(a); !slices.Equal(got, c.row) {
			t.Errorf("%s: forkRow = %v, want %v", c.name, got, c.row)
		}
		if got := appStatusText(a); got != c.status {
			t.Errorf("%s: appStatusText = %q, want %q", c.name, got, c.status)
		}
		lines := forkLines(a)
		want := []string{"follows CRM (crm): " + c.text, "its releases arrive on the branch upstream, which only Whisk moves"}
		if c.next != "" {
			want = append(want, c.next)
		}
		if !slices.Equal(lines, want) {
			t.Errorf("%s: forkLines = %v, want %v", c.name, lines, want)
		}
	}
	if got := forkLines(api.App{Slug: "shop"}); got != nil {
		t.Errorf("forkLines for an app that follows nothing = %v", got)
	}
	// A fork the platform could not describe still has its row.
	if got := forkRow(api.App{Slug: "crm", Status: "running"}); !slices.Equal(got, []string{"crm", "-", "-", "-", "-", "running"}) {
		t.Errorf("forkRow without the fork = %v", got)
	}
}

// The products table names what a copy is only when a product whose copies are the business's
// own code is listed, so a business offered none sees the table it always has.
func TestProductsTable(t *testing.T) {
	managed := api.ManagedProduct{Slug: "erp-link", Name: "ERP link", Links: []string{"shop"}, Available: true}
	forks := api.ManagedProduct{Slug: "crm", Name: "CRM", Kind: api.ProductForks, Available: true}

	header, rows := productsTable([]api.ManagedProduct{managed})
	if slices.Contains(header, "KIND") || len(rows[0]) != len(header) {
		t.Errorf("managed products alone: %v %v", header, rows)
	}
	if got := addHints([]api.ManagedProduct{managed}); len(got) != 1 || !strings.Contains(got[0], "--link <role>=<app>") {
		t.Errorf("hints for managed products alone = %v", got)
	}

	header, rows = productsTable([]api.ManagedProduct{managed, forks})
	if header[2] != "KIND" || rows[0][2] != "run by Whisk" || rows[1][2] != "your own code" || len(rows[1]) != len(header) {
		t.Errorf("with a forks product: %v %v", header, rows)
	}
	if got := addHints([]api.ManagedProduct{managed, forks}); len(got) != 2 || got[1] != "Add one that is your own code with: whisk managed add <product>" {
		t.Errorf("hints with a forks product = %v", got)
	}
	if got := addHints([]api.ManagedProduct{forks}); len(got) != 1 || !strings.HasSuffix(got[0], "whisk managed add <product>") {
		t.Errorf("hints for forks products alone = %v", got)
	}
	if got := addHints(nil); len(got) != 0 {
		t.Errorf("hints with no products = %v", got)
	}
}

func TestUpdateLine(t *testing.T) {
	fork := &api.AppFork{Product: "crm", Name: "CRM", Branch: "upstream", Release: 3, Newest: 3}
	changed := &api.AppFork{Product: "crm", Name: "CRM", Branch: "upstream", Release: 3, Newest: 3, Changed: true}
	deploy := &api.Deploy{ID: "d_1"}
	for _, c := range []struct {
		name string
		u    api.ForkUpdate
		want string
	}{
		{"the code moved", api.ForkUpdate{App: api.App{Slug: "crm", Fork: fork}, Deploy: deploy, Updated: true}, "Moved acme/crm to CRM release 3."},
		{"already the newest", api.ForkUpdate{App: api.App{Slug: "crm", Fork: fork}, Deploy: deploy}, "acme/crm already holds CRM release 3, the newest."},
		{"the newest with changes", api.ForkUpdate{App: api.App{Slug: "crm", Fork: changed}}, "acme/crm holds CRM release 3, the newest, with changes of its own, so there is nothing to take."},
		{"moved, the fork not described", api.ForkUpdate{App: api.App{Slug: "crm"}, Deploy: deploy, Updated: true}, "Moved acme/crm to its product's newest release."},
		{"nothing, the fork not described", api.ForkUpdate{App: api.App{Slug: "crm"}}, "acme/crm has nothing to take."},
	} {
		if got := updateLine("acme", c.u); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// What whisk managed add says: the copy by the name it carries, its variant and links when it
// has them, and for a fork what its business does next.
func TestAddedLines(t *testing.T) {
	deploying := "It is deploying the newest release now; whisk status --app erp shows when it is live."
	for _, c := range []struct {
		name            string
		a               api.App
		variant, linked string
		want            []string
	}{
		{"a managed copy with a variant and a link", api.App{Slug: "erp", Hostname: "erp.acme.whisk.page", Managed: &api.AppManaged{Name: "ERP link"}}, "unleashed", "shop=store",
			[]string{"Added ERP link to acme as erp (unleashed), linked shop=store.", deploying, "https://erp.acme.whisk.page"}},
		{"a fork", api.App{Slug: "erp", Fork: &api.AppFork{Name: "CRM", Branch: "upstream"}}, "", "",
			[]string{"Added CRM to acme as erp.", deploying,
				"Its code is the business's own: whisk clone acme/erp brings it here, and whisk deploy puts a change live.",
				"New releases of CRM arrive on its branch upstream, which only Whisk moves; whisk managed update erp takes one while the code is unchanged, and whisk managed merge brings one into changed code."}},
		{"a copy the answer does not describe", api.App{Slug: "erp"}, "", "", []string{"Added erp-link to acme as erp.", deploying}},
	} {
		if got := addedLines("acme", "erp-link", c.a, c.variant, c.linked); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// forkState is what the stand-in API for a business with a fork remembers and how its update
// answers.
type forkState struct {
	release int    // the release the fork's code holds
	changed bool   // whether the business changed its code
	update  string // how POST .../fork/update answers: forward, current, none or changed
	// blocked is an update whose rehearsal changed what people entered: its deploy waits for an
	// owner's or admin's yes, with the copy kept to look at.
	blocked bool
	adds    []map[string]any
	updates int
	// suggestions is what GET .../fork/suggestions answers, newest first.
	suggestions []map[string]any
}

func (st *forkState) app() map[string]any {
	return map[string]any{"id": "app_crm", "org_id": "org_acme", "slug": "crm", "name": "CRM", "hostname": "crm.acme.whisk.page", "url": "https://crm.acme.whisk.page",
		"status": "running", "state": "running", "region": "nz", "git_url": "https://git.whisk.run/org_acme/app_crm.git", "unset_secrets": []any{}, "created_at": "2026-11-10T00:00:00Z",
		"fork": map[string]any{"product": "crm", "name": "CRM", "branch": "upstream", "release": st.release, "newest": 3, "behind": 3 - st.release, "changed": st.changed}}
}

// forkAPI answers the routes a fork uses (FORKS.md) for the acme business, which is offered the
// CRM as its own code beside the ERP link Whisk runs.
func forkAPI(t *testing.T, st *forkState) *httptest.Server {
	mux := http.NewServeMux()
	deploy := func(status string) map[string]any {
		return map[string]any{"id": "d_up", "app_id": "app_crm", "environment": "production", "commit_sha": "abc1234abc", "branch": "main", "status": status,
			"phases": []map[string]any{{"phase": status, "at": "2026-11-10T10:00:00Z"}}, "created_at": "2026-11-10T10:00:00Z"}
	}
	mux.HandleFunc("GET /v1/orgs/acme/managed", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"products": []any{
				map[string]any{"slug": "crm", "name": "CRM", "kind": "forks", "variants": []any{}, "settings": []any{}, "links": []any{}, "available": true},
				map[string]any{"slug": "erp-link", "name": "ERP link", "kind": "managed", "variants": []any{}, "settings": []any{}, "links": []any{"shop"}, "month_cents": 4900, "available": true},
			},
			"copies": []any{},
			"forks":  []any{st.app()},
		})
	})
	mux.HandleFunc("POST /v1/orgs/acme/managed", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.adds = append(st.adds, body)
		writeJSON(w, 201, st.app())
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, st.app()) })
	mux.HandleFunc("GET /v1/orgs/acme/apps", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"next_cursor": "", "items": []any{st.app()}})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/fork/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("POST fork/update without Idempotency-Key")
		}
		st.updates++
		switch st.update {
		case "forward":
			st.release = 3
			writeJSON(w, 202, map[string]any{"app": st.app(), "deploy": deploy("queued"), "updated": true})
		case "current":
			writeJSON(w, 200, map[string]any{"app": st.app(), "deploy": deploy("live"), "updated": false})
		case "none":
			writeJSON(w, 200, map[string]any{"app": st.app(), "updated": false})
		default:
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "FORK_CHANGED", "message": "crm has changes of its own since CRM release 2, so Whisk does not move its code to release 3.",
				"fix":     "Run whisk managed merge in a checkout of crm's code, which brings the release in and names each change of the business's it meets (or git fetch whisk, git merge whisk/upstream), settle any conflicts, run the tests, then whisk deploy.",
				"details": map[string]any{"app": "crm", "product": "crm", "release": 2, "newest": 3}}})
		}
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/fork/suggestions", func(w http.ResponseWriter, r *http.Request) {
		list := st.suggestions
		if list == nil {
			list = []map[string]any{}
		}
		writeJSON(w, 200, map[string]any{"suggestions": list})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/shop/fork/update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "shop does not follow one of Whisk's products.",
			"fix": "Deploy it as any app of your own, with whisk deploy.", "details": map[string]any{"app": "shop"}}})
	})
	changes := []any{
		map[string]any{"kind": "data", "name": "rows:notes", "environment": "production", "hash": "sha256:r3", "recipe_changed": false, "new_operations": []any{}, "change": "1 row of notes is removed."},
		map[string]any{"kind": "data", "name": "column:notes.body:changed", "environment": "production", "hash": "sha256:r3", "recipe_changed": false, "new_operations": []any{}, "change": "2 rows of notes have a different value in body."},
	}
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys/d_up", func(w http.ResponseWriter, r *http.Request) {
		d := deploy("blocked")
		d["grants_needed"] = changes
		d["rehearsal"] = map[string]any{"status": "changed", "release": 3, "from_release": 2, "tables": 4, "rows": 120, "bytes": 8192, "truncated": false,
			"changes": []any{"1 row of notes is removed.", "2 rows of notes have a different value in body."}, "report": []any{},
			"copy": "app_crm_restore_1700000000", "kept_until": "2026-11-11T09:00:00Z", "started_at": "2026-11-10T10:00:01Z", "ended_at": "2026-11-10T10:00:03Z"}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys/d_up/events", func(w http.ResponseWriter, r *http.Request) {
		if st.blocked {
			sse(w,
				map[string]any{"deploy_id": "d_up", "status": "rehearsing", "phase": map[string]any{"phase": "rehearsing", "at": "2026-11-10T10:00:01Z"}, "done": false},
				map[string]any{"deploy_id": "d_up", "status": "blocked", "phase": map[string]any{"phase": "blocked", "at": "2026-11-10T10:00:03Z"}, "done": true,
					"error": map[string]any{"code": "GRANT_NEEDED", "message": "This deploy waits for an owner or admin to approve what the update does to the data people entered.",
						"fix": "Ask an owner or admin to approve it.", "details": map[string]any{"grants": changes, "unset_secrets": []any{}}}},
			)
			return
		}
		sse(w,
			map[string]any{"deploy_id": "d_up", "status": "starting", "phase": map[string]any{"phase": "starting", "at": "2026-11-10T10:00:01Z"}, "done": false},
			map[string]any{"deploy_id": "d_up", "status": "live", "phase": map[string]any{"phase": "live", "at": "2026-11-10T10:00:04Z"}, "done": true, "url": "https://crm.acme.whisk.page", "unset_secrets": []string{}},
		)
	})
	return httptest.NewServer(mux)
}

// whisk managed list shows a product whose copies are the business's own code, and the
// business's forks with where each stands.
func TestManagedListForks(t *testing.T) {
	st := &forkState{release: 2}
	srv := forkAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "managed", "list")
	for _, want := range []string{"KIND", "your own code", "run by Whisk", "USD 49.00 a month", "NEWEST", "as released", "running, fork, 1 behind",
		"Add one with: whisk managed add <product> --link", "Add one that is your own code with: whisk managed add <product>"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("managed list lacks %q (exit %d):\n%s%s", want, r.Code, r.Stdout, r.Stderr)
		}
	}
	if strings.Contains(r.Stdout, "No copies yet.") {
		t.Errorf("a business with a fork is told it has no copies:\n%s", r.Stdout)
	}

	r = runRemote(t, dir, srv.URL, nil, "managed", "list", "--json")
	forks, _ := r.JSON["forks"].([]any)
	products, _ := r.JSON["products"].([]any)
	if r.Code != 0 || len(forks) != 1 || len(products) != 2 || products[0].(map[string]any)["kind"] != "forks" {
		t.Fatalf("managed list json: %+v", r)
	}
	if f := forks[0].(map[string]any)["fork"].(map[string]any); f["product"] != "crm" || f["behind"] != float64(1) || f["branch"] != "upstream" {
		t.Errorf("forks: %+v", forks)
	}

	// whisk apps info and whisk apps list say where the fork stands.
	r = runRemote(t, dir, srv.URL, nil, "apps", "info", "crm")
	for _, want := range []string{"follows CRM (crm): release 2 as Whisk made it, 1 behind release 3", "the branch upstream, which only Whisk moves", "whisk managed update crm moves it to release 3"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("apps info lacks %q:\n%s", want, r.Stdout)
		}
	}
	if r = runRemote(t, dir, srv.URL, nil, "apps", "list"); !strings.Contains(r.Stdout, "running, fork, 1 behind") {
		t.Errorf("apps list:\n%s", r.Stdout)
	}
}

// Adding a product whose copies are the business's own code takes only its name, and the answer
// says the code is the business's and how releases arrive.
func TestManagedAddFork(t *testing.T) {
	st := &forkState{release: 3}
	srv := forkAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "managed", "add", "crm")
	if r.Code != 0 || len(st.adds) != 1 {
		t.Fatalf("managed add: %+v adds %v", r, st.adds)
	}
	if body := st.adds[0]; body["product"] != "crm" || len(body["links"].(map[string]any)) != 0 || body["variant"] != nil || body["settings"] != nil {
		t.Errorf("add body: %v", body)
	}
	for _, want := range []string{"Added CRM to acme as crm.", "whisk status --app crm", "whisk clone acme/crm", "whisk deploy",
		"New releases of CRM arrive on its branch upstream, which only Whisk moves", "whisk managed update crm", "https://crm.acme.whisk.page"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("add output lacks %q:\n%s", want, r.Stdout)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "managed", "add", "crm", "--json")
	app, _ := r.JSON["app"].(map[string]any)
	if r.Code != 0 || app["fork"] == nil || app["managed"] != nil {
		t.Errorf("add json: %+v", r)
	}
}

// whisk managed update moves a fork's unchanged code and follows its deploy, says so when there
// is nothing to take, and passes on the platform's refusal of code the business changed.
func TestManagedUpdate(t *testing.T) {
	st := &forkState{release: 2, update: "forward"}
	srv := forkAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	// The code moves and the deploy is followed until it is live; the bound app is the default.
	r := runRemote(t, dir, srv.URL, nil, "managed", "update")
	if r.Code != 0 || st.updates != 1 || !strings.Contains(r.Stderr, "Moved acme/crm to CRM release 3.") || !strings.Contains(r.Stdout, "https://crm.acme.whisk.page") {
		t.Fatalf("managed update: %+v", r)
	}

	// For an agent the summary carries the deploy, whether the code moved and where the fork stands.
	st.release = 2
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--json")
	fork, _ := r.JSON["fork"].(map[string]any)
	if r.Code != 0 || r.JSON["deploy_id"] != "d_up" || r.JSON["status"] != "live" || r.JSON["updated"] != true || fork["release"] != float64(3) {
		t.Fatalf("managed update json: %+v", r)
	}

	// --no-wait names the deploy and exits.
	st.release = 2
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--no-wait")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Moved acme/crm to CRM release 3.") || !strings.Contains(r.Stdout, "whisk deploys info d_up") {
		t.Errorf("managed update --no-wait: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--no-wait", "--json")
	if r.Code != 0 || r.JSON["deploy_id"] != "d_up" || r.JSON["updated"] != true || r.JSON["org"] != "acme" || r.JSON["app"] == nil {
		t.Errorf("managed update --no-wait json: %+v", r)
	}

	// Code that is the newest release already: the deploy that carries it is reported.
	st.update, st.release = "current", 3
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm")
	if r.Code != 0 || !strings.Contains(r.Stderr, "acme/crm already holds CRM release 3, the newest.") || !strings.Contains(r.Stdout, "https://crm.acme.whisk.page") {
		t.Errorf("managed update on the newest: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--json")
	if r.Code != 0 || r.JSON["updated"] != false || r.JSON["status"] != "live" {
		t.Errorf("managed update on the newest, json: %+v", r)
	}

	// Code the business changed that holds the newest release: nothing to take, and no deploy.
	st.update, st.changed = "none", true
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm")
	if r.Code != 0 || !strings.Contains(r.Stdout, "with changes of its own, so there is nothing to take.") {
		t.Errorf("managed update with nothing to take: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--json")
	if r.Code != 0 || r.JSON["updated"] != false || r.JSON["deploy_id"] != nil || r.JSON["app"] == nil {
		t.Errorf("managed update with nothing to take, json: %+v", r)
	}

	// Code the business changed that lacks a release: the platform's refusal, with its merge.
	st.update, st.release = "changed", 2
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != output.ExitError || e["code"] != "FORK_CHANGED" || !strings.Contains(e["fix"].(string), "git merge whisk/upstream") {
		t.Errorf("managed update of changed code: %+v", r)
	}

	// An app that follows no product.
	r = runRemote(t, dir, srv.URL, nil, "managed", "update", "shop", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code != output.ExitError || e["code"] != "INVALID_REQUEST" || e["fix"] == "" {
		t.Errorf("managed update of an app that follows nothing: %+v", r)
	}
	if r := runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "shop", "--json"); r.Code == 0 {
		t.Errorf("two apps accepted: %+v", r)
	}
}

// An update whose rehearsal changed what people entered waits for an owner or admin: the command
// says what the update does to the data and how to read the data as the update left it first, and
// whisk deploys info shows the rehearsal (FORKS.md §4).
func TestManagedUpdateRehearsed(t *testing.T) {
	st := &forkState{release: 2, update: "forward", blocked: true}
	srv := forkAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "managed", "update", "crm", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != output.ExitNeedsHuman || e["code"] != "GRANT_NEEDED" {
		t.Fatalf("a rehearsed update that changes the data: %+v", r)
	}
	message, _ := e["message"].(string)
	fix, _ := e["fix"].(string)
	if !strings.Contains(message, "1 row of notes is removed; 2 rows of notes have a different value in body") {
		t.Errorf("message %q", message)
	}
	if !strings.Contains(fix, "/o/acme/apps/crm/deploys?approve=d_up") || !strings.Contains(fix, "Do not try to allow it yourself") ||
		!strings.Contains(fix, "read the data as the update left it with whisk db query --database app_crm_restore_1700000000 until ") {
		t.Errorf("fix %q", fix)
	}
	details, _ := e["details"].(map[string]any)
	if rehearsal, _ := details["rehearsal"].(map[string]any); rehearsal["copy"] != "app_crm_restore_1700000000" || rehearsal["status"] != "changed" {
		t.Errorf("details %v", details)
	}

	r = runRemote(t, dir, srv.URL, nil, "deploys", "info", "d_up")
	for _, want := range []string{"rehearsal changed: release 3 from 2, a copy of 4 tables and 120 rows", "1 row of notes is removed.",
		"read the copy with whisk db query --database app_crm_restore_1700000000 until "} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("deploys info lacks %q (exit %d):\n%s%s", want, r.Code, r.Stdout, r.Stderr)
		}
	}
}

func TestRehearsalLines(t *testing.T) {
	if got := rehearsalLines(nil); got != nil {
		t.Errorf("no rehearsal = %v", got)
	}
	clean := rehearsalLines(&apitypes.Rehearsal{Status: apitypes.RehearsalClean, Release: 2, FromRelease: 1, Tables: 3, Rows: 9, Changes: []string{}})
	if !slices.Equal(clean, []string{"rehearsal clean: release 2 from 1, a copy of 3 tables and 9 rows"}) {
		t.Errorf("a clean rehearsal = %q", clean)
	}
	failed := rehearsalLines(&apitypes.Rehearsal{Status: apitypes.RehearsalFailed, Release: 4})
	if !slices.Equal(failed, []string{"rehearsal failed: release 4"}) {
		t.Errorf("a failed rehearsal = %q", failed)
	}
}
