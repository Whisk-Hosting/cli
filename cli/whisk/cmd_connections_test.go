package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/connect"
	werrors "github.com/whisk-run/contract/errors"
)

func TestGrantState(t *testing.T) {
	prod, prev := apitypes.ConnectionProduction, apitypes.ConnectionPreviews
	grant := func(env apitypes.ConnectionEnvironment, state apitypes.ConnectionState, current bool) api.ConnectionGrant {
		return api.ConnectionGrant{Environment: env, State: state, Current: current}
	}
	cases := []struct {
		name string
		c    api.Connection
		env  apitypes.ConnectionEnvironment
		want string
	}{
		{"never granted", api.Connection{}, prod, "not granted"},
		{"never granted, a deploy waits", api.Connection{Waiting: true}, prod, "waits for you"},
		{"previews never wait", api.Connection{Waiting: true}, prev, "not granted"},
		{"granted", api.Connection{Grants: []api.ConnectionGrant{grant(prod, apitypes.ConnectionActive, true)}}, prod, "granted"},
		{"granted in production only", api.Connection{Grants: []api.ConnectionGrant{grant(prod, apitypes.ConnectionActive, true)}}, prev, "not granted"},
		{"paused", api.Connection{Grants: []api.ConnectionGrant{grant(prod, apitypes.ConnectionPaused, true)}}, prod, "paused"},
		{"revoked", api.Connection{Waiting: true, Grants: []api.ConnectionGrant{grant(prod, apitypes.ConnectionRevoked, false)}}, prod, "revoked"},
		{"older version, a deploy waits", api.Connection{Waiting: true, Grants: []api.ConnectionGrant{grant(prod, apitypes.ConnectionActive, false)}}, prod, "waits for you"},
		{"older version of previews", api.Connection{Grants: []api.ConnectionGrant{grant(prev, apitypes.ConnectionActive, false)}}, prev, "granted, older version"},
	}
	for _, c := range cases {
		if got := grantState(c.c, c.env); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestConnectionRows(t *testing.T) {
	all := []api.Connection{
		{Name: "erp", Summary: connect.Summary{Host: "api.example-erp.com"}, Waiting: true, Unset: []string{"ERP_KEY"},
			Grants: []api.ConnectionGrant{{Environment: "previews", State: "active", Current: true, CallsToday: 4}}},
		{Name: "crm", Summary: connect.Summary{Host: "api.example-crm.com"},
			Grants: []api.ConnectionGrant{{Environment: "production", State: "paused", Current: true, CallsToday: 7}, {Environment: "previews", State: "active", Current: true, CallsToday: 2}}},
		{Name: "open"},
	}
	want := [][]string{
		{"erp", "api.example-erp.com", "waits for you", "granted", "a grant; keys ERP_KEY", "4"},
		{"crm", "api.example-crm.com", "paused", "granted", "-", "9"},
		{"open", "-", "not granted", "not granted", "-", "0"},
	}
	if got := connectionRows(all); !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n got %q\nwant %q", got, want)
	}
}

func TestAddressLine(t *testing.T) {
	cases := []struct {
		sum  connect.Summary
		want string
	}{
		{connect.Summary{Host: "api.example-erp.com", Port: "443", BasePath: "/v2"}, "erp sends requests to api.example-erp.com:443, below /v2."},
		{connect.Summary{Host: "api.example-erp.com", Port: "8443", BasePath: "/"}, "erp sends requests to api.example-erp.com:8443."},
		{connect.Summary{}, "erp sends requests to -:-."},
	}
	for _, c := range cases {
		if got := addressLine("erp", c.sum); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.sum, got, c.want)
		}
	}
}

func TestOperationRows(t *testing.T) {
	ops := []connect.SummaryOperation{
		{Method: "GET", Path: "/stock/**", Verb: "Read", Label: "Read stock levels"},
		{Method: "POST", Path: "/orders", Verb: "Create"},
		{Method: "TRACE", Path: "/x", Label: "odd"},
	}
	want := [][]string{
		{"  Read", "GET", "/stock/**", "Read stock levels"},
		{"  Create", "POST", "/orders", "-"},
		{"  TRACE", "TRACE", "/x", "odd"},
	}
	if got := operationRows(ops); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSecretUseAndWarnings(t *testing.T) {
	uses := []connect.Use{
		{Name: "ERP_ID", Sent: true, To: []string{"api.example-erp.com"}},
		{Name: "ERP_KEY", Sent: false, To: []string{}},
		{Name: "CLIENT_SECRET", Sent: true, To: []string{"api.example-erp.com", "login.example.com"}},
		{Name: "ODD", Sent: true},
	}
	wantRows := [][]string{
		{"  ERP_ID", "sent to api.example-erp.com"},
		{"  ERP_KEY", "only signs; it never leaves Whisk"},
		{"  CLIENT_SECRET", "sent to api.example-erp.com, login.example.com"},
		{"  ODD", "sent to the outside system"},
	}
	if got := secretRows(uses); !reflect.DeepEqual(got, wantRows) {
		t.Errorf("rows: got %q, want %q", got, wantRows)
	}
	wantWarn := []string{
		"ERP_ID is sent to api.example-erp.com with every request. That system could record it.",
		"CLIENT_SECRET is sent to api.example-erp.com, login.example.com with every request. That system could record it.",
		"ODD is sent to the outside system with every request. That system could record it.",
	}
	if got := secretWarnings(uses); !reflect.DeepEqual(got, wantWarn) {
		t.Errorf("warnings: got %q, want %q", got, wantWarn)
	}
	if got := secretWarnings(uses[1:2]); got != nil {
		t.Errorf("a key that only signs warns: %q", got)
	}
}

func TestPausePlan(t *testing.T) {
	cases := []struct {
		args     []string
		all      bool
		env      string
		wantName string
		wantEnv  apitypes.ConnectionEnvironment
		wantErr  bool
	}{
		{[]string{"erp"}, false, "", "erp", "", false},
		{[]string{"erp"}, false, "production", "erp", "production", false},
		{[]string{"erp"}, false, "previews", "erp", "previews", false},
		{[]string{"erp"}, false, "preview", "", "", true},
		{[]string{"erp"}, false, "staging", "", "", true},
		{nil, false, "", "", "", true},
		{nil, true, "", "", "", false},
		{[]string{"erp"}, true, "", "", "", true},
		{nil, true, "production", "", "", true},
	}
	for _, c := range cases {
		name, env, err := pausePlan(c.args, c.all, c.env)
		if (err != nil) != c.wantErr || name != c.wantName || env != c.wantEnv {
			t.Errorf("%v all=%v env=%q: got %q %q %v", c.args, c.all, c.env, name, env, err)
		}
		if err != nil && output.ExitCode(err) != output.ExitError {
			t.Errorf("%v: exit %d", c.args, output.ExitCode(err))
		}
	}
}

func TestPauseSentence(t *testing.T) {
	cases := []struct {
		name    string
		env     apitypes.ConnectionEnvironment
		changed int
		want    string
	}{
		{"erp", "", 2, "Paused erp in every environment (2 grants). Calls stop at once; a call already on its way finishes."},
		{"erp", "production", 1, "Paused erp in production (1 grant). Calls stop at once; a call already on its way finishes."},
		{"erp", "previews", 0, "erp had no active grant in previews, so nothing changed."},
		{"", "", 5, "Paused every connection in the business (5 grants). Calls stop at once; a call already on its way finishes."},
		{"", "", 0, "No connection in the business was active, so nothing changed."},
	}
	for _, c := range cases {
		if got := pauseSentence(c.name, c.env, c.changed); got != c.want {
			t.Errorf("%q %q %d: got %q", c.name, c.env, c.changed, got)
		}
	}
}

func TestGrantChange(t *testing.T) {
	ops := []connect.Operation{{Name: "Read stock levels", Method: "GET", Path: "/stock/**"}, {Method: "POST", Path: "/orders"}}
	cases := []struct {
		g    api.GrantNeeded
		want string
	}{
		{api.GrantNeeded{Name: "erp", Environment: "production", RecipeChanged: true}, "erp (production) with a new address or recipe"},
		{api.GrantNeeded{Name: "erp", Environment: "production", NewOperations: ops}, "erp (production) with new operations GET /stock/** (Read stock levels), POST /orders"},
		{api.GrantNeeded{Name: "erp", RecipeChanged: true, NewOperations: ops[:1]}, "erp with a new address or recipe and new operations GET /stock/** (Read stock levels)"},
		{api.GrantNeeded{Name: "erp", Environment: "production"}, "erp (production)"},
	}
	for _, c := range cases {
		if got := grantChange(c.g); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestBlockedDetail(t *testing.T) {
	grants := []any{map[string]any{"kind": "connection", "name": "erp", "environment": "production", "recipe_changed": true,
		"new_operations": []any{map[string]any{"name": "Read stock levels", "method": "GET", "path": "/stock/**"}}}}
	cases := []struct {
		name       string
		d          *werrors.Detail
		wantGrants []api.GrantNeeded
		wantUnset  []string
	}{
		{"no error", nil, nil, nil},
		{"waiting on secrets", &werrors.Detail{Code: "SECRET_UNSET", Details: map[string]any{"names": []any{"A"}}}, nil, nil},
		{"waiting on a grant", &werrors.Detail{Code: "GRANT_NEEDED", Details: map[string]any{"grants": grants, "unset_secrets": []any{"ERP_KEY"}}},
			[]api.GrantNeeded{{Kind: "connection", Name: "erp", Environment: "production", RecipeChanged: true, NewOperations: []connect.Operation{{Name: "Read stock levels", Method: "GET", Path: "/stock/**"}}}},
			[]string{"ERP_KEY"}},
		{"a grant error with nothing in it", &werrors.Detail{Code: "GRANT_NEEDED"}, nil, nil},
		{"malformed details", &werrors.Detail{Code: "GRANT_NEEDED", Details: map[string]any{"grants": "erp", "unset_secrets": 3}}, nil, nil},
	}
	for _, c := range cases {
		g, u := blockedDetail(c.d)
		if !reflect.DeepEqual(g, c.wantGrants) || !reflect.DeepEqual(u, c.wantUnset) {
			t.Errorf("%s: got %+v %v", c.name, g, u)
		}
	}
}

func TestGrantsNeededBlock(t *testing.T) {
	page, secrets := "https://whisk.run/o/acme/apps/crm/connections", "https://whisk.run/o/acme/apps/crm/secrets"
	in := []api.GrantNeeded{
		{Kind: "connection", Name: "erp", Environment: "production", RecipeChanged: true},
		{Kind: "connection", Name: "crm", Environment: "production", NewOperations: []connect.Operation{{Name: "Read contacts", Method: "GET", Path: "/contacts"}}},
	}
	cases := []struct {
		name      string
		unset     []string
		wantInFix []string
		notInFix  []string
	}{
		{"grants only", nil, []string{"grant erp, crm at " + page, "Do not try to grant it yourself"}, []string{"also needs"}},
		{"grants and unset secrets", []string{"ERP_KEY"}, []string{"grant erp, crm at " + page, "It also needs values for ERP_KEY, set at " + secrets + "?set=ERP_KEY."}, nil},
	}
	for _, c := range cases {
		e := grantsNeededBlock("d-1", in, c.unset, page, secrets)
		if e.Code != "GRANT_NEEDED" || output.ExitCode(e) != output.ExitNeedsHuman {
			t.Errorf("%s: code %s exit %d", c.name, e.Code, output.ExitCode(e))
		}
		wantMsg := "Deploy d-1 waits for a person to grant erp (production) with a new address or recipe; crm (production) with new operations GET /contacts (Read contacts)."
		if e.Message != wantMsg {
			t.Errorf("%s: message %q", c.name, e.Message)
		}
		for _, s := range c.wantInFix {
			if !strings.Contains(e.Fix, s) {
				t.Errorf("%s: fix %q lacks %q", c.name, e.Fix, s)
			}
		}
		for _, s := range c.notInFix {
			if strings.Contains(e.Fix, s) {
				t.Errorf("%s: fix %q has %q", c.name, e.Fix, s)
			}
		}
		if strings.Contains(e.Message+e.Fix, "—") {
			t.Errorf("%s: an em dash in user-facing copy", c.name)
		}
		d := e.Details
		listed := d["grants"].([]api.GrantNeeded)
		if d["url"] != page || d["deploy_id"] != "d-1" || len(listed) != 2 || listed[0].NewOperations == nil || d["unset_secrets"] == nil {
			t.Errorf("%s: details %+v", c.name, d)
		}
		if in[0].NewOperations != nil {
			t.Errorf("%s: the block changed its input", c.name)
		}
	}
}

func TestGrantBlock(t *testing.T) {
	e := grantBlock("erp", "acme", "crm", "https://whisk.run/o/acme/apps/crm/connections")
	if e.Code != "NEEDS_HUMAN" || output.ExitCode(e) != output.ExitNeedsHuman || e.Details["url"] != "https://whisk.run/o/acme/apps/crm/connections" || e.Details["name"] != "erp" {
		t.Errorf("block: %+v", e)
	}
	if !strings.Contains(e.Fix, "whisk connections show erp") {
		t.Errorf("fix: %q", e.Fix)
	}
}

func TestGrantCodesExit(t *testing.T) {
	for _, code := range []string{"GRANT_NEEDED", "CONFIRMATION_NEEDED"} {
		if got := output.ExitCode(&output.Error{Code: code}); got != output.ExitNeedsHuman {
			t.Errorf("%s: exit %d, want %d", code, got, output.ExitNeedsHuman)
		}
	}
}

// connectionsAPI is a control plane with two connections and the deploys a grant blocks.
func connectionsAPI(t *testing.T) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var calls []string
	record := func(r *http.Request, body string) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path+" "+body)
		mu.Unlock()
	}
	granted := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	erp := api.Connection{Name: "erp", Hash: "h1", Waiting: true, Unset: []string{},
		Summary: connect.Describe(connect.Connection{URL: "https://api.example-erp.com/v2", Auth: connect.Auth{Headers: map[string]string{
			"api-auth-id": "{secret.ERP_ID}", "api-auth-signature": "{base64(hmac_sha256(secret.ERP_KEY, request.query))}",
		}}, Operations: []connect.Operation{{Name: "Read stock levels", Method: "GET", Path: "/stock/**"}, {Name: "Create sales orders", Method: "POST", Path: "/orders"}}}),
		Grants: []api.ConnectionGrant{{ID: "g1", Environment: "production", Hash: "h0", State: "active", LimitPerMinute: 60, LimitPerDay: 10000, GrantedBy: "Ana", GrantedAt: granted, CallsToday: 12,
			Missing: []connect.Operation{{Name: "Create sales orders", Method: "POST", Path: "/orders"}}}}}
	open := api.Connection{Name: "open", Hash: "h2", Unset: []string{}, Grants: []api.ConnectionGrant{},
		Summary: connect.Describe(connect.Connection{URL: "https://api.example-open.com", Operations: []connect.Operation{{Name: "Read", Method: "GET", Path: "/**"}}})}
	grantErr := map[string]any{"code": "GRANT_NEEDED", "message": "The connection erp needs a person to grant it.", "fix": "Ask an owner.",
		"details": map[string]any{"grants": []map[string]any{{"kind": "connection", "name": "erp", "environment": "production", "recipe_changed": false,
			"new_operations": []map[string]any{{"name": "Create sales orders", "method": "POST", "path": "/orders"}}}}, "unset_secrets": []string{"ERP_KEY"}, "url": "x"}}
	deploy := func(id string, extra map[string]any) map[string]any {
		d := map[string]any{"id": id, "app_id": "a1", "environment": "production", "commit_sha": "abc1234abc", "status": "blocked",
			"phases": []map[string]any{{"phase": "blocked", "at": "2026-10-08T10:00:00Z"}}, "created_at": "2026-10-08T10:00:00Z", "grants_needed": []any{}}
		for k, v := range extra {
			d[k] = v
		}
		return d
	}
	deploys := map[string]map[string]any{
		// The deploy's error carries what it waits on.
		"d-err": deploy("d-err", map[string]any{"error": grantErr}),
		// Only the deploy's record does.
		"d-rec": deploy("d-rec", map[string]any{"grants_needed": []map[string]any{{"kind": "connection", "name": "erp", "environment": "production", "recipe_changed": true, "new_operations": []any{}}}}),
		// Waiting on secrets alone.
		"d-sec": deploy("d-sec", nil),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/connections", func(w http.ResponseWriter, r *http.Request) {
		record(r, "")
		writeJSON(w, 200, map[string]any{"items": []api.Connection{erp, open}, "next_cursor": ""})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/connections/{name}/pause", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		record(r, string(b))
		changed := 0
		if r.PathValue("name") == "erp" {
			changed = 1
		}
		writeJSON(w, 200, api.ConnectionsChanged{Changed: changed})
	})
	mux.HandleFunc("POST /v1/orgs/acme/connections/pause", func(w http.ResponseWriter, r *http.Request) {
		record(r, "")
		writeJSON(w, 200, api.ConnectionsChanged{Changed: 3})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/connections/{name}/grant", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the CLI must never grant")
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/deploys", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, 202, deploys[body["deploy_id"].(string)])
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/deploys/{id}", func(w http.ResponseWriter, r *http.Request) {
		record(r, "")
		writeJSON(w, 200, deploys[r.PathValue("id")])
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "a1", "slug": "crm", "unset_secrets": []string{"XERO_KEY"}, "created_at": "2026-09-07T00:00:00Z"})
	})
	srv := httptest.NewServer(mux)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

func TestConnectionsCommands(t *testing.T) {
	srv, calls := connectionsAPI(t)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	page := srv.URL + "/o/acme/apps/crm/connections"

	r := runRemote(t, dir, srv.URL, nil, "connections", "list", "--json")
	if r.Code != 0 || len(r.JSON["connections"].([]any)) != 2 || r.JSON["url"] != page {
		t.Fatalf("list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "list")
	for _, want := range []string{"erp", "api.example-erp.com", "waits for you", "a grant", "12", page} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Fatalf("list human lacks %q: %+v", want, r)
		}
	}

	r = runRemote(t, dir, srv.URL, nil, "connections", "show", "erp", "--json")
	if r.Code != 0 || r.JSON["connection"].(map[string]any)["name"] != "erp" || r.JSON["url"] != page {
		t.Fatalf("show: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "show", "erp")
	for _, want := range []string{
		"erp sends requests to api.example-erp.com:443, below /v2.",
		"Create", "POST", "/orders", "Create sales orders",
		"ERP_ID", "sent to api.example-erp.com",
		"ERP_KEY", "only signs; it never leaves Whisk",
		"Warning: ERP_ID is sent to api.example-erp.com with every request.",
		"The app may send anything these addresses accept.",
		"production: waits for you; granted by Ana on ", "not yet granted: POST /orders (Create sales orders)",
		"previews: not granted", page,
	} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Fatalf("show human lacks %q:\n%s", want, r.Stdout)
		}
	}
	if strings.Contains(r.Stdout, "Warning: ERP_KEY") {
		t.Errorf("a key that only signs was warned about:\n%s", r.Stdout)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "show", "open")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Keys: none.") {
		t.Fatalf("show with no keys: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "show", "billing", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("show unknown: %+v", r)
	}

	before := len(calls())
	r = runRemote(t, dir, srv.URL, nil, "connections", "grant", "erp", "--json")
	e := r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || e["details"].(map[string]any)["url"] != page {
		t.Fatalf("grant: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "grant", "erp")
	if r.Code != 2 || !strings.Contains(r.Stderr, "NEEDS_HUMAN") || !strings.Contains(r.Stderr, page) {
		t.Fatalf("grant human: %+v", r)
	}
	if len(calls()) != before {
		t.Errorf("grant called the platform: %v", calls()[before:])
	}

	r = runRemote(t, dir, srv.URL, nil, "connections", "pause", "erp", "--env", "production", "--json")
	if r.Code != 0 || r.JSON["changed"] != float64(1) || r.JSON["environment"] != "production" {
		t.Fatalf("pause: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "pause", "erp")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Paused erp in every environment") || !strings.Contains(r.Stdout, page) {
		t.Fatalf("pause human: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "connections", "pause", "--all", "--json")
	if r.Code != 0 || r.JSON["changed"] != float64(3) || r.JSON["all"] != true {
		t.Fatalf("pause --all: %+v", r)
	}
	for _, args := range [][]string{{"pause"}, {"pause", "erp", "--env", "staging"}, {"pause", "erp", "--all"}} {
		r = runRemote(t, dir, srv.URL, nil, append(append([]string{"connections"}, args...), "--json")...)
		if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "INVALID_REQUEST" {
			t.Errorf("%v: %+v", args, r)
		}
	}
	got := strings.Join(calls(), "\n")
	for _, want := range []string{
		`POST /v1/orgs/acme/apps/crm/connections/erp/pause {"environment":"production"}`,
		`POST /v1/orgs/acme/apps/crm/connections/erp/pause {}`,
		`POST /v1/orgs/acme/connections/pause`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("calls lack %q:\n%s", want, got)
		}
	}
}

// A deploy blocked on a grant answers GRANT_NEEDED naming the connection, what changed and the
// connections page, exit 2, whether its error or only its record says so (CLI.md §5.4). One
// blocked on secrets alone keeps the secrets block.
func TestDeployBlockedOnAGrant(t *testing.T) {
	srv, _ := connectionsAPI(t)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	page := srv.URL + "/o/acme/apps/crm/connections"

	r := runRemote(t, dir, srv.URL, nil, "rollback", "d-err", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "GRANT_NEEDED" {
		t.Fatalf("blocked by error: %+v", r)
	}
	d := e["details"].(map[string]any)
	if d["url"] != page || d["deploy_id"] != "d-err" || len(d["grants"].([]any)) != 1 || d["unset_secrets"].([]any)[0] != "ERP_KEY" ||
		!strings.Contains(e["message"].(string), "erp (production) with new operations POST /orders (Create sales orders)") ||
		!strings.Contains(e["fix"].(string), srv.URL+"/o/acme/apps/crm/secrets?set=ERP_KEY") {
		t.Fatalf("blocked by error: %v", e)
	}

	r = runRemote(t, dir, srv.URL, nil, "rollback", "d-rec")
	if r.Code != 2 || !strings.Contains(r.Stderr, "GRANT_NEEDED") || !strings.Contains(r.Stderr, "erp (production) with a new address or recipe") || !strings.Contains(r.Stderr, page) {
		t.Fatalf("blocked by record: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "rollback", "d-sec", "--json")
	e, _ = r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || !strings.Contains(e["message"].(string), "XERO_KEY") {
		t.Fatalf("blocked on secrets: %+v", r)
	}
}
