package whisk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

func TestParseSettings(t *testing.T) {
	cases := []struct {
		name      string
		words     []string
		removable bool
		want      map[string]string
		code      string
	}{
		{"none", nil, false, map[string]string{}, ""},
		{"one", []string{"LIMIT_PER_SECOND=5"}, false, map[string]string{"LIMIT_PER_SECOND": "5"}, ""},
		{"value with = and spaces", []string{"NOTE=a=b c"}, false, map[string]string{"NOTE": "a=b c"}, ""},
		{"several", []string{"A=1", "B_2=two"}, false, map[string]string{"A": "1", "B_2": "two"}, ""},
		{"empty removes", []string{"A=", "B=2"}, true, map[string]string{"A": "", "B": "2"}, ""},
		{"empty refused on add", []string{"A="}, false, nil, "INVALID_REQUEST"},
		{"no equals", []string{"LIMIT"}, true, nil, "INVALID_REQUEST"},
		{"no name", []string{"=5"}, true, nil, "INVALID_REQUEST"},
		{"not an env name", []string{"LIMIT-PER=5"}, true, nil, "INVALID_REQUEST"},
		{"variant setting", []string{"UNLEASHED_WAREHOUSE=main"}, false, map[string]string{"UNLEASHED_WAREHOUSE": "main"}, ""},
		{"leading digit", []string{"1A=5"}, true, nil, "INVALID_REQUEST"},
		{"twice", []string{"A=1", "A=2"}, true, nil, "INVALID_REQUEST"},
	}
	for _, c := range cases {
		got, err := parseSettings(c.words, c.removable)
		if c.code != "" {
			e, ok := err.(*output.Error)
			if !ok || e.Code != c.code || e.Fix == "" {
				t.Errorf("%s: err %v, want %s with a fix", c.name, err, c.code)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestParseLinks(t *testing.T) {
	cases := []struct {
		name  string
		words []string
		want  map[string]string
		ok    bool
	}{
		{"none", nil, map[string]string{}, true},
		{"one", []string{"shop=shop"}, map[string]string{"shop": "shop"}, true},
		{"by id", []string{"shop=app_123"}, map[string]string{"shop": "app_123"}, true},
		{"two roles", []string{"shop=shop", "erp=books"}, map[string]string{"shop": "shop", "erp": "books"}, true},
		{"spaces trimmed", []string{" shop = shop "}, map[string]string{"shop": "shop"}, true},
		{"no equals", []string{"shop"}, nil, false},
		{"no role", []string{"=shop"}, nil, false},
		{"no app", []string{"shop="}, nil, false},
		{"role twice", []string{"shop=a", "shop=b"}, nil, false},
	}
	for _, c := range cases {
		got, err := parseLinks(c.words)
		if !c.ok {
			if e, isOut := err.(*output.Error); !isOut || e.Code != "INVALID_REQUEST" {
				t.Errorf("%s: err %v, want INVALID_REQUEST", c.name, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestResolveLinks(t *testing.T) {
	apps := []api.App{{ID: "app_shop", Slug: "shop"}, {ID: "app_crm", Slug: "crm"}}
	cases := []struct {
		name  string
		links map[string]string
		want  map[string]string
		ok    bool
	}{
		{"none", map[string]string{}, map[string]string{}, true},
		{"by slug", map[string]string{"shop": "shop"}, map[string]string{"shop": "app_shop"}, true},
		{"by id", map[string]string{"shop": "app_crm"}, map[string]string{"shop": "app_crm"}, true},
		{"unknown", map[string]string{"shop": "nope"}, nil, false},
	}
	for _, c := range cases {
		got, err := resolveLinks("acme", c.links, apps)
		if !c.ok {
			e, isOut := err.(*output.Error)
			if !isOut || e.Code != "INVALID_REQUEST" || e.Details["app"] != "nope" {
				t.Errorf("%s: err %v", c.name, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
	if got := linksText(map[string]string{"shop": "app_shop", "erp": "app_gone"}, apps); got != "erp=app_gone, shop=shop" {
		t.Errorf("linksText = %q", got)
	}
}

func TestManagedWords(t *testing.T) {
	zero, price := int64(0), int64(4900)
	months := map[*int64]string{nil: "-", &zero: "included", &price: "USD 49.00 a month"}
	for c, want := range months {
		if got := monthText(c); got != want {
			t.Errorf("monthText = %q, want %q", got, want)
		}
	}
	reasons := []struct {
		p    api.ManagedProduct
		want string
	}{
		{api.ManagedProduct{Available: true}, "yes"},
		{api.ManagedProduct{Unavailable: "plan"}, "no: not in this plan"},
		{api.ManagedProduct{Unavailable: "trial"}, "no: not during the free trial"},
		{api.ManagedProduct{Unavailable: "client"}, "no: not for a business its agency pays for"},
		{api.ManagedProduct{Unavailable: "no_release"}, "no: no release ready yet"},
		{api.ManagedProduct{Unavailable: "frozen"}, "no: frozen"},
		{api.ManagedProduct{Available: false, Unavailable: ""}, "no"},
		{api.ManagedProduct{Available: true, Unavailable: "plan"}, "yes"},
		{api.ManagedProduct{Unavailable: "retired", Slug: "older"}, "no: no longer offered"},
	}
	for _, c := range reasons {
		if got := unavailableText(c.p); got != c.want {
			t.Errorf("unavailableText(%+v) = %q, want %q", c.p, got, c.want)
		}
	}
	paused := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	statuses := []struct {
		a    api.App
		want string
	}{
		{api.App{Status: "running"}, "running"},
		{api.App{Status: "running", Managed: &api.AppManaged{}}, "running, managed"},
		{api.App{Status: "sleeping", Managed: &api.AppManaged{Held: true}, PausedAt: &paused}, "sleeping, managed, held, paused"},
		{api.App{Status: "sleeping", PausedAt: &paused}, "sleeping, paused"},
	}
	for _, c := range statuses {
		if got := appStatusText(c.a); got != c.want {
			t.Errorf("appStatusText = %q, want %q", got, c.want)
		}
	}
	if got := releaseText(&api.AppManaged{Release: &api.ManagedRelease{Commit: "abcdef0123456"}}); got != "abcdef0" {
		t.Errorf("releaseText = %q", got)
	}
	if got := releaseText(nil); got != "-" {
		t.Errorf("releaseText(nil) = %q", got)
	}
	lines := strings.Join(managedLines(api.App{Slug: "erp-link", PausedAt: &paused, Managed: &api.AppManaged{Product: "erp-link", Name: "ERP link", Variant: "unleashed",
		Settings: map[string]string{"B": "2", "A": "1"}, Links: map[string]string{"shop": "app_shop"}}}, []api.App{{ID: "app_shop", Slug: "shop"}}), "\n")
	for _, want := range []string{"managed by Whisk: ERP link (erp-link), variant unleashed", "links shop=shop", "settings A=1, B=2", "paused since", "whisk resume erp-link"} {
		if !strings.Contains(lines, want) {
			t.Errorf("managedLines lacks %q:\n%s", want, lines)
		}
	}
	if got := managedLines(api.App{Slug: "crm"}, nil); len(got) != 0 {
		t.Errorf("managedLines for a plain app = %v", got)
	}
}

// managedState is what the fake managed-app routes remember between calls.
type managedState struct {
	adds    []map[string]any
	patches []map[string]any
	links   []string // "role body.app" of each PUT .../links/:role
	paused  bool
	calls   []string
}

func (st *managedState) copyApp() map[string]any {
	a := map[string]any{"id": "app_erp", "org_id": "org_acme", "slug": "erp-link", "name": "ERP link", "hostname": "erp-link.acme.whisk.page",
		"status": "running", "state": "running", "region": "nz",
		"managed": map[string]any{"product": "erp-link", "name": "ERP link", "variant": "unleashed",
			"release":  map[string]any{"id": "rel_1", "commit": "0123456789abcdef", "live_at": "2026-10-09T12:00:00Z"},
			"settings": map[string]any{"LIMIT_PER_SECOND": "5"}, "links": map[string]any{"shop": "app_shop"}, "held": false}}
	if st.paused {
		a["paused_at"] = "2026-10-10T01:00:00Z"
		a["status"] = "sleeping"
	}
	return a
}

// managedAPI answers the managed-app routes of MANAGED-APPS.md §10 for the acme business.
func managedAPI(t *testing.T, st *managedState) *httptest.Server {
	mux := http.NewServeMux()
	shop := map[string]any{"id": "app_shop", "org_id": "org_acme", "slug": "shop", "name": "shop", "hostname": "shop.acme.whisk.page", "status": "running", "region": "nz"}
	note := func(r *http.Request) { st.calls = append(st.calls, r.Method+" "+r.URL.Path) }
	mux.HandleFunc("GET /v1/orgs/acme/apps", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		writeJSON(w, 200, map[string]any{"next_cursor": "", "items": []any{shop, st.copyApp()}})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/erp-link", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		writeJSON(w, 200, st.copyApp())
	})
	mux.HandleFunc("GET /v1/orgs/acme/managed", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		writeJSON(w, 200, map[string]any{
			"products": []any{
				map[string]any{"slug": "erp-link", "name": "ERP link", "variants": []any{map[string]any{"name": "unleashed", "title": "Unleashed", "settings": []any{"UNLEASHED_WAREHOUSE"}}, map[string]any{"name": "xero", "title": "Xero", "settings": []any{}}},
					"settings": []any{"LIMIT_PER_SECOND", "ALLOWANCE_DAY"}, "links": []any{"shop"}, "month_cents": 4900, "available": true},
				map[string]any{"slug": "payroll", "name": "Payroll", "variants": []any{}, "settings": []any{}, "links": []any{}, "available": false, "unavailable": "plan"},
			},
			"copies": []any{st.copyApp()},
		})
	})
	mux.HandleFunc("POST /v1/orgs/acme/managed", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		if r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("POST managed without Idempotency-Key")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.adds = append(st.adds, body)
		if body["product"] == "payroll" {
			writeJSON(w, 402, map[string]any{"error": map[string]any{"code": "MANAGED_UNAVAILABLE", "message": "Payroll is not included in the Free plan.",
				"fix": "An owner or billing contact can choose a plan that includes it at https://whisk.run/o/acme/billing, then add it.", "details": map[string]any{"product": "payroll", "reason": "plan"}}})
			return
		}
		writeJSON(w, 201, st.copyApp())
	})
	mux.HandleFunc("PATCH /v1/orgs/acme/apps/erp-link/managed", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.patches = append(st.patches, body)
		writeJSON(w, 200, st.copyApp())
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/erp-link/pause", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		st.paused = true
		writeJSON(w, 200, st.copyApp())
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/erp-link/resume", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		st.paused = false
		writeJSON(w, 200, st.copyApp())
	})
	mux.HandleFunc("PUT /v1/orgs/acme/apps/erp-link/links/{role}", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		if r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("PUT link without Idempotency-Key")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.links = append(st.links, r.PathValue("role")+" "+fmt.Sprint(body["app"]))
		if r.PathValue("role") != "shop" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "ERP link has no link called " + r.PathValue("role") + ".",
				"fix": "Use one of: [shop].", "details": map[string]any{"role": r.PathValue("role")}}})
			return
		}
		writeJSON(w, 200, st.copyApp())
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/shop/pause", func(w http.ResponseWriter, r *http.Request) {
		note(r)
		writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "Only a managed app can be paused.",
			"fix": "Stop the app's work in its own code instead.", "details": map[string]any{"app": "shop"}}})
	})
	return httptest.NewServer(mux)
}

func TestManagedList(t *testing.T) {
	st := &managedState{paused: true}
	srv := managedAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	for _, args := range [][]string{{"managed"}, {"managed", "list"}} {
		r := runRemote(t, dir, srv.URL, nil, args...)
		for _, want := range []string{"erp-link", "ERP link", "USD 49.00 a month", "unleashed (UNLEASHED_WAREHOUSE), xero", "LIMIT_PER_SECOND, ALLOWANCE_DAY", "yes",
			"payroll", "no: not in this plan", "0123456", "sleeping, managed, paused", "shop=shop", "whisk managed add"} {
			if r.Code != 0 || !strings.Contains(r.Stdout, want) {
				t.Errorf("%v lacks %q (exit %d):\n%s%s", args, want, r.Code, r.Stdout, r.Stderr)
			}
		}
	}

	r := runRemote(t, dir, srv.URL, nil, "managed", "list", "--json")
	products, _ := r.JSON["products"].([]any)
	copies, _ := r.JSON["copies"].([]any)
	if r.Code != 0 || r.JSON["org"] != "acme" || len(products) != 2 || len(copies) != 1 {
		t.Fatalf("managed list json: %+v", r)
	}
	first := products[0].(map[string]any)
	if first["month_cents"] != float64(4900) || first["available"] != true || products[1].(map[string]any)["unavailable"] != "plan" {
		t.Errorf("products: %+v", products)
	}
	if m := copies[0].(map[string]any)["managed"].(map[string]any); m["product"] != "erp-link" || copies[0].(map[string]any)["paused_at"] == nil {
		t.Errorf("copies: %+v", copies)
	}
}

func TestManagedAdd(t *testing.T) {
	st := &managedState{}
	srv := managedAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "managed", "add", "erp-link", "--link", "shop=shop", "--variant", "unleashed", "--set", "LIMIT_PER_SECOND=5", "--set", "ALLOWANCE_DAY=1000")
	if r.Code != 0 || len(st.adds) != 1 {
		t.Fatalf("managed add: %+v adds %v", r, st.adds)
	}
	body := st.adds[0]
	links, _ := body["links"].(map[string]any)
	settings, _ := body["settings"].(map[string]any)
	if body["product"] != "erp-link" || body["variant"] != "unleashed" || links["shop"] != "app_shop" || len(links) != 1 ||
		settings["LIMIT_PER_SECOND"] != "5" || settings["ALLOWANCE_DAY"] != "1000" {
		t.Errorf("add body: %v", body)
	}
	for _, want := range []string{"Added ERP link to acme as erp-link (unleashed), linked shop=shop.", "whisk status --app erp-link", "https://erp-link.acme.whisk.page"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("add output lacks %q:\n%s", want, r.Stdout)
		}
	}

	// A link to an app the business does not have is refused before anything is added.
	r = runRemote(t, dir, srv.URL, nil, "managed", "add", "erp-link", "--link", "shop=nope", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != output.ExitError || e["code"] != "INVALID_REQUEST" || e["fix"] == "" || len(st.adds) != 1 {
		t.Errorf("unknown link: %+v", r)
	}

	// Malformed words are refused before any call.
	before := len(st.calls)
	for _, args := range [][]string{
		{"managed", "add", "erp-link", "--link", "shop", "--json"},
		{"managed", "add", "erp-link", "--link", "shop=shop", "--set", "LIMIT_PER_SECOND", "--json"},
		{"managed", "add", "erp-link", "--link", "shop=shop", "--set", "LIMIT_PER_SECOND=", "--json"},
	} {
		r := runRemote(t, dir, srv.URL, nil, args...)
		if e, _ := r.JSON["error"].(map[string]any); r.Code != output.ExitError || e["code"] != "INVALID_REQUEST" {
			t.Errorf("%v: %+v", args, r)
		}
	}
	if len(st.calls) != before {
		t.Errorf("malformed words reached the API: %v", st.calls[before:])
	}

	// The platform's refusal passes through with its code and fix.
	r = runRemote(t, dir, srv.URL, nil, "managed", "add", "payroll", "--json")
	e, _ = r.JSON["error"].(map[string]any)
	if r.Code != output.ExitError || e["code"] != "MANAGED_UNAVAILABLE" || !strings.Contains(e["fix"].(string), "billing") {
		t.Errorf("unavailable: %+v", r)
	}
	if last := st.adds[len(st.adds)-1]; last["product"] != "payroll" || len(last["links"].(map[string]any)) != 0 || last["settings"] != nil {
		t.Errorf("add without links or settings sent %v", last)
	}

	r = runRemote(t, dir, srv.URL, nil, "managed", "add", "erp-link", "--link", "shop=app_shop", "--variant", "xero", "--json")
	app, _ := r.JSON["app"].(map[string]any)
	if r.Code != 0 || r.JSON["org"] != "acme" || app["slug"] != "erp-link" || app["managed"] == nil {
		t.Errorf("add json: %+v", r)
	}
}

func TestManagedSet(t *testing.T) {
	st := &managedState{}
	srv := managedAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "managed", "set", "erp-link", "LIMIT_PER_SECOND=10", "ALLOWANCE_DAY=")
	if r.Code != 0 || len(st.patches) != 1 {
		t.Fatalf("managed set: %+v", r)
	}
	settings, _ := st.patches[0]["settings"].(map[string]any)
	if len(settings) != 2 || settings["LIMIT_PER_SECOND"] != "10" || settings["ALLOWANCE_DAY"] != "" {
		t.Errorf("patch body: %v", st.patches[0])
	}
	if !strings.Contains(r.Stdout, "Changed acme/erp-link: set LIMIT_PER_SECOND; removed ALLOWANCE_DAY.") {
		t.Errorf("set output:\n%s", r.Stdout)
	}

	r = runRemote(t, dir, srv.URL, nil, "managed", "set", "erp-link", "LIMIT_PER_SECOND=3", "--json")
	set, _ := r.JSON["set"].([]any)
	removed, _ := r.JSON["removed"].([]any)
	if r.Code != 0 || len(set) != 1 || set[0] != "LIMIT_PER_SECOND" || removed == nil || len(removed) != 0 || r.JSON["app"] == nil {
		t.Errorf("set json: %+v", r)
	}

	for _, args := range [][]string{{"managed", "set", "erp-link", "--json"}, {"managed", "set", "erp-link", "BAD", "--json"}} {
		r := runRemote(t, dir, srv.URL, nil, args...)
		if r.Code == 0 || r.JSON["error"] == nil {
			t.Errorf("%v accepted: %+v", args, r)
		}
	}
	if len(st.patches) != 2 {
		t.Errorf("refused words reached the API: %v", st.patches)
	}
}

func TestPauseResume(t *testing.T) {
	st := &managedState{}
	srv := managedAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "pause", "erp-link")
	if r.Code != 0 || !st.paused || !strings.Contains(r.Stdout, "Paused acme/erp-link") || !strings.Contains(r.Stdout, "whisk resume erp-link") {
		t.Fatalf("pause: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "list")
	shopLine := strings.Split(r.Stdout, "\n")[1] // the header, then shop, which is no copy
	if !strings.Contains(r.Stdout, "sleeping, managed, paused") || !strings.HasPrefix(shopLine, "shop ") || strings.Contains(shopLine, "managed") {
		t.Errorf("apps list marks:\n%s", r.Stdout)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "info", "erp-link")
	for _, want := range []string{"managed by Whisk: ERP link (erp-link), variant unleashed", "release 0123456", "links shop=shop", "settings LIMIT_PER_SECOND=5", "paused since"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("apps info lacks %q:\n%s", want, r.Stdout)
		}
	}

	r = runRemote(t, dir, srv.URL, nil, "resume", "erp-link", "--json")
	if r.Code != 0 || st.paused || r.JSON["paused"] != false || r.JSON["org"] != "acme" {
		t.Fatalf("resume: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "pause", "erp-link", "--json")
	if r.Code != 0 || r.JSON["paused"] != true {
		t.Fatalf("pause json: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "pause", "shop", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != output.ExitError || e["code"] != "INVALID_REQUEST" || e["fix"] == "" {
		t.Errorf("pause a plain app: %+v", r)
	}
	if r := runRemote(t, dir, srv.URL, nil, "pause", "--json"); r.Code == 0 {
		t.Errorf("pause without an app: %+v", r)
	}
}

func TestManagedLink(t *testing.T) {
	st := &managedState{}
	srv := managedAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "managed", "link", "erp-link", "shop=shop")
	if r.Code != 0 || len(st.links) != 1 || st.links[0] != "shop app_shop" || !strings.Contains(r.Stdout, "Linked acme/erp-link: shop=shop.") {
		t.Fatalf("managed link: %+v links %v", r, st.links)
	}

	r = runRemote(t, dir, srv.URL, nil, "managed", "link", "erp-link", "shop=app_shop", "--json")
	if r.Code != 0 || r.JSON["role"] != "shop" || r.JSON["linked"] != "app_shop" || r.JSON["org"] != "acme" || r.JSON["app"] == nil || st.links[1] != "shop app_shop" {
		t.Errorf("link json: %+v", r)
	}

	// A role the product does not have: the platform's refusal passes through.
	r = runRemote(t, dir, srv.URL, nil, "managed", "link", "erp-link", "erp=shop", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code != output.ExitError || e["code"] != "INVALID_REQUEST" || e["fix"] == "" {
		t.Errorf("unknown role: %+v", r)
	}

	// An app the business does not have, or a malformed word, never reaches the link route.
	for _, args := range [][]string{
		{"managed", "link", "erp-link", "shop=nope", "--json"},
		{"managed", "link", "erp-link", "shop", "--json"},
		{"managed", "link", "erp-link", "--json"},
		{"managed", "link", "erp-link", "shop=shop", "erp=shop", "--json"},
	} {
		if r := runRemote(t, dir, srv.URL, nil, args...); r.Code == 0 || r.JSON["error"] == nil {
			t.Errorf("%v accepted: %+v", args, r)
		}
	}
	if len(st.links) != 3 {
		t.Errorf("refused links reached the API: %v", st.links)
	}
}

func TestManagedRefusal(t *testing.T) {
	if err := managedRefusal(api.App{Slug: "shop"}); err != nil {
		t.Errorf("an ordinary app refused: %v", err)
	}
	err := managedRefusal(api.App{Slug: "erp-link", Managed: &api.AppManaged{Product: "erp-link", Name: "ERP link"}})
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "APP_MANAGED" || oe.Details["product"] != "erp-link" {
		t.Errorf("a copy: %v", err)
	}
}
