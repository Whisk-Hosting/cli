package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

// dataAPI is the control plane for the data, customer, token and run commands.
type dataState struct {
	bodies  map[string][]map[string]any // "METHOD path" → bodies received
	deleted []string
}

func (st *dataState) record(r *http.Request) map[string]any {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	key := r.Method + " " + r.URL.Path
	st.bodies[key] = append(st.bodies[key], body)
	return body
}

func dataAPI(t *testing.T, st *dataState) *httptest.Server {
	st.bodies = map[string][]map[string]any{}
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer whsk_agent_test" {
				writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "AUTH_REQUIRED", "message": "no token", "fix": "login"}})
				return
			}
			if missingKey(r) {
				t.Errorf("%s %s without Idempotency-Key", r.Method, r.URL.Path)
			}
			h(w, r)
		}
	}
	app := "/v1/orgs/acme/apps/crm"
	mux.HandleFunc("POST "+app+"/db/url", auth(func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		env, _ := body["environment"].(string)
		if env == "" {
			env = "production"
		}
		reachable := "anywhere"
		if env == "preview:inside" {
			reachable = "platform"
		}
		writeJSON(w, 201, map[string]any{"url": "postgres://whisk_u1:tok@db.whisk.run:6432/app_a1?sslmode=require", "environment": env, "expires_at": "2026-09-16T10:15:00Z", "reachable": reachable})
	}))
	mux.HandleFunc("POST "+app+"/db/query", auth(func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		deleted := map[string]any{"statements": []any{map[string]any{"command": "DELETE 2", "rows": 2}}, "tables": []any{map[string]any{"table": "public.notes", "inserted": 0, "updated": 0, "deleted": 2}}}
		switch {
		case body["confirm"] == "wq_abc":
			writeJSON(w, 200, map[string]any{"environment": "production", "database": "app_a1", "columns": []any{}, "rows": []any{}, "row_count": 0, "command": "DELETE 2", "effect": deleted,
				"restore_point": map[string]any{"id": "rp1", "environment": "production", "created_at": "2026-09-16T10:00:00Z"}})
		case body["confirm"] != nil:
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "CONFIRM_INVALID", "message": "That code is unknown, used, expired or someone else's.", "fix": "Run the SQL again."}})
		case strings.HasPrefix(body["sql"].(string), "delete"):
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "CONFIRM_REQUIRED", "message": "Nothing was changed yet. This would delete 2 rows in public.notes in the production database.",
				"fix": "Run whisk db query --confirm wq_abc.", "details": map[string]any{"code": "wq_abc", "effect": deleted}}})
		default:
			writeJSON(w, 200, map[string]any{"environment": "production", "database": "app_a1", "columns": []any{"id", "body"}, "rows": []any{[]any{"1", "hello"}, []any{"2", nil}},
				"row_count": 2, "command": "SELECT 2", "effect": map[string]any{"statements": []any{map[string]any{"command": "SELECT 2", "rows": 2}}, "tables": []any{}}})
		}
	}))
	mux.HandleFunc("POST "+app+"/db/schema", auth(func(w http.ResponseWriter, r *http.Request) {
		st.record(r)
		writeJSON(w, 200, map[string]any{"environment": "production", "database": "app_a1", "tables": []any{map[string]any{"schema": "public", "name": "notes", "kind": "table", "rows_estimate": 2,
			"columns":     []any{map[string]any{"name": "id", "type": "integer", "nullable": false, "default": "nextval('notes_id_seq'::regclass)"}, map[string]any{"name": "body", "type": "text", "nullable": true, "default": nil}},
			"constraints": []any{map[string]any{"name": "notes_pkey", "kind": "primary_key", "definition": "PRIMARY KEY (id)"}}}}})
	}))
	mux.HandleFunc("POST "+app+"/db/snapshot", auth(func(w http.ResponseWriter, r *http.Request) {
		st.record(r)
		writeJSON(w, 201, map[string]any{"id": "snap1", "environment": "production", "lsn": "0/16B6C50", "created_at": "2026-09-16T10:00:00Z"})
	}))
	mux.HandleFunc("POST "+app+"/restore", auth(func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		if body["at"] != "2026-09-07T14:13:00Z" {
			t.Errorf("restore at %v", body["at"])
		}
		if body["at"] == "2026-09-07T14:13:00Z" && body["swap"] == true {
			writeJSON(w, 402, map[string]any{"error": map[string]any{"code": "RESTORE_NOT_AVAILABLE", "message": "Self-service restore is not included in the Free plan.", "fix": "Upgrade.", "details": map[string]any{"plan": "free"}}})
			return
		}
		writeJSON(w, 202, map[string]any{"id": "r1", "environment": "production", "at": "2026-09-07T14:13:00Z", "swap": false, "status": "queued", "target_database": "app_a1_restore_20260907141300", "created_at": "2026-09-16T10:00:00Z"})
	}))

	customers := []map[string]any{
		{"id": "c1", "email": "pat@customer.example", "name": "Pat", "status": "active", "created_at": "2026-09-10T00:00:00Z"},
		{"id": "c2", "email": "sam@customer.example", "status": "blocked", "created_at": "2026-09-11T00:00:00Z"},
	}
	mux.HandleFunc("GET "+app+"/customers", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": customers, "signup": false})
	}))
	mux.HandleFunc("POST "+app+"/customers", auth(func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		writeJSON(w, 201, map[string]any{"id": "c3", "email": body["email"], "name": body["name"], "status": "invited", "invite_url": "https://crm--acme.whisk.page/.whisk/invite/abc", "created_at": "2026-09-16T00:00:00Z"})
	}))
	mux.HandleFunc("PATCH "+app+"/customers/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "email": "pat@customer.example", "status": body["status"], "created_at": "2026-09-10T00:00:00Z"})
	}))
	mux.HandleFunc("DELETE "+app+"/customers/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		st.deleted = append(st.deleted, r.URL.Path)
		w.WriteHeader(204)
	}))

	mux.HandleFunc("GET /v1/orgs/acme/tokens", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "t1", "kind": "agent", "label": "claude on laptop", "scopes": []string{"org:01J", "deploy"}, "last_used_at": "2026-09-16T09:00:00Z", "expires_at": "2026-10-16T09:00:00Z", "created_at": "2026-09-01T00:00:00Z"}}})
	}))
	mux.HandleFunc("POST /v1/orgs/acme/tokens", auth(func(w http.ResponseWriter, r *http.Request) {
		body := st.record(r)
		writeJSON(w, 201, map[string]any{"token": map[string]any{"id": "t2", "kind": "agent", "label": body["label"], "scopes": body["scopes"], "created_at": "2026-09-16T00:00:00Z"}, "value": "whsk_agent_ONCE"})
	}))
	mux.HandleFunc("DELETE /v1/orgs/acme/tokens/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		st.deleted = append(st.deleted, r.URL.Path)
		w.WriteHeader(204)
	}))

	run := map[string]any{"id": "run1", "function": "nightly", "status": "parked", "steps": []string{"fetch"}, "started_at": "2026-09-16T06:00:00Z"}
	mux.HandleFunc("GET "+app+"/runs", auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("function") != "nightly" || q.Get("status") != "parked" || q.Get("since") == "" {
			t.Errorf("runs query %s", r.URL.RawQuery)
		}
		writeJSON(w, 200, map[string]any{"items": []map[string]any{run}, "next_cursor": ""})
	}))
	mux.HandleFunc("POST "+app+"/runs/run1/replay", auth(func(w http.ResponseWriter, r *http.Request) {
		st.record(r)
		writeJSON(w, 202, map[string]any{"id": "run2", "function": "nightly", "status": "running", "steps": []string{}})
	}))
	mux.HandleFunc("POST "+app+"/cron/{function}/run", auth(func(w http.ResponseWriter, r *http.Request) {
		st.record(r)
		switch r.PathValue("function") {
		case "nightly":
			writeJSON(w, 202, map[string]any{"id": "run3", "function": "nightly", "status": "queued", "trigger": "cron", "manual": true, "steps": []string{}})
		case "slow":
			writeJSON(w, 202, map[string]any{"function": "slow", "status": "queued", "trigger": "cron", "manual": true, "steps": []string{}, "event_id": "evt9"})
		case "canary-event":
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "canary-event runs on the event canary.event, not on a schedule.", "fix": "Start it by sending its event: whisk events send canary.event --data '{...}'."}})
		case "fresh":
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "FUNCTION_NOT_LIVE", "message": "fresh cannot run yet: no deploy of crm has gone live.", "fix": "Push with whisk deploy and wait for it to go live, then run whisk cron run fresh again.", "details": map[string]any{"reason": "not_deployed"}}})
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "crm declares no function named " + r.PathValue("function") + ".", "fix": "Run whisk cron list."}})
		}
	}))
	mux.HandleFunc("GET "+app+"/runs/run3", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "run3", "function": "nightly", "status": "completed", "trigger": "cron", "manual": true, "steps": []string{"count-notes", "record-summary"}})
	}))
	mux.HandleFunc("POST "+app+"/runs/run1/cancel", auth(func(w http.ResponseWriter, r *http.Request) {
		st.record(r)
		writeJSON(w, 200, map[string]any{"id": "run1", "function": "nightly", "status": "cancelled", "steps": []string{"fetch"}})
	}))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": r.URL.Path, "fix": "check the path"}})
	})
	return httptest.NewServer(mux)
}

func errCode(r result) string {
	e, _ := r.JSON["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestDBAndRestore(t *testing.T) {
	st := &dataState{}
	srv := dataAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "db", "url", "--json")
	if r.Code != 0 || !strings.HasPrefix(r.JSON["url"].(string), "postgres://") || r.JSON["environment"] != "production" {
		t.Fatalf("db url: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "url", "--env", "preview:feature-x")
	if r.Code != 0 || !strings.HasPrefix(strings.TrimSpace(r.Stdout), "postgres://") || !strings.Contains(r.Stderr, "audited") {
		t.Fatalf("db url human: %+v", r)
	}
	if got := st.bodies["POST /v1/orgs/acme/apps/crm/db/url"][1]["environment"]; got != "preview:feature-x" {
		t.Fatalf("environment sent: %v", got)
	}

	r = runRemote(t, dir, srv.URL, nil, "db", "snapshot", "--json")
	if r.Code != 0 || r.JSON["snapshot"].(map[string]any)["id"] != "snap1" {
		t.Fatalf("db snapshot: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "snapshot")
	if r.Code != 0 || !strings.Contains(r.Stdout, "whisk restore --at 2026-09-16T10:00:00Z") {
		t.Fatalf("db snapshot human: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "db", "url", "--env", "preview:inside", "--json")
	if r.Code != 1 || errCode(r) != "DB_URL_PRIVATE" {
		t.Fatalf("db url inside the platform: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "url", "--env", "preview:inside", "--private", "--json")
	if r.Code != 0 || r.JSON["reachable"] != "platform" {
		t.Fatalf("db url --private: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "db", "query", "select id, body from notes", "--json")
	if r.Code != 0 || r.JSON["row_count"] != float64(2) || r.JSON["rows"].([]any)[1].([]any)[1] != nil {
		t.Fatalf("db query: %+v", r)
	}
	if got := st.bodies["POST /v1/orgs/acme/apps/crm/db/query"][0]; got["sql"] != "select id, body from notes" || got["limit"] != float64(1000) {
		t.Fatalf("db query sent: %v", got)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "query", "select id, body from notes")
	if r.Code != 0 || !strings.Contains(r.Stdout, "hello") || !strings.Contains(r.Stdout, "null") || !strings.Contains(r.Stdout, "(2 rows)") {
		t.Fatalf("db query human: %+v", r)
	}
	if err := os.WriteFile(filepath.Join(dir, "look.sql"), []byte("select 1;\nselect id, body from notes;"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "query", "--file", "look.sql", "--json")
	if got := st.bodies["POST /v1/orgs/acme/apps/crm/db/query"][2]["sql"]; r.Code != 0 || got != "select 1;\nselect id, body from notes;" {
		t.Fatalf("db query --file: %+v sent %v", r, got)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "query", "--json")
	if r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
		t.Fatalf("db query without sql: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "query", "delete from notes", "--json")
	if r.Code != 1 || errCode(r) != "CONFIRM_REQUIRED" || r.JSON["error"].(map[string]any)["details"].(map[string]any)["code"] != "wq_abc" {
		t.Fatalf("db query destructive: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "query", "--confirm", "wq_abc")
	if r.Code != 0 || !strings.Contains(r.Stdout, "DELETE 2") || !strings.Contains(r.Stdout, "whisk restore --at 2026-09-16T10:00:00Z") {
		t.Fatalf("db query --confirm: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "query", "--confirm", "wq_abc", "select 1", "--json")
	if r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
		t.Fatalf("db query --confirm with sql: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "db", "schema", "--json")
	if r.Code != 0 || r.JSON["tables"].([]any)[0].(map[string]any)["name"] != "notes" {
		t.Fatalf("db schema: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "db", "schema")
	if r.Code != 0 || !strings.Contains(r.Stdout, "public.notes (about 2 rows)") || !strings.Contains(r.Stdout, "id integer not null") || !strings.Contains(r.Stdout, "PRIMARY KEY (id)") {
		t.Fatalf("db schema human: %+v", r)
	}

	r = runRemoteIn(t, "select id,\n body from notes;\ndelete from notes;\nno\ndelete from notes;\nyes\n", dir, srv.URL, "db", "shell")
	if r.Code != 0 || strings.Count(r.Stdout, "hello") != 1 || !strings.Contains(r.Stderr, "Not run; nothing was changed.") || !strings.Contains(r.Stdout, "DELETE 2") {
		t.Fatalf("db shell: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "restore", "--json")
	if r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
		t.Fatalf("restore without --at: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "restore", "--at", "yesterday-ish", "--json")
	if r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
		t.Fatalf("restore with a bad time: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "restore", "--at", "2026-09-07T14:13:00Z", "--json")
	if r.Code != 0 || r.JSON["restore"].(map[string]any)["target_database"] != "app_a1_restore_20260907141300" {
		t.Fatalf("restore: %+v", r)
	}
	if body := st.bodies["POST /v1/orgs/acme/apps/crm/restore"][0]; body["swap"] != false {
		t.Fatalf("restore body %v", body)
	}
	r = runRemote(t, dir, srv.URL, nil, "restore", "--at", "2026-09-07T14:13:00Z", "--swap", "--json")
	if r.Code != 1 || errCode(r) != "RESTORE_NOT_AVAILABLE" {
		t.Fatalf("restore on a plan without it: %+v", r)
	}
}

func TestCustomers(t *testing.T) {
	st := &dataState{}
	srv := dataAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "customers", "list", "--json")
	if r.Code != 0 || len(r.JSON["customers"].([]any)) != 2 || r.JSON["signup"] != false {
		t.Fatalf("customers list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "customers", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "pat@customer.example") || !strings.Contains(r.Stdout, "Invitation only") {
		t.Fatalf("customers list human: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "customers", "invite", "new@customer.example", "--name", "New", "--json")
	if r.Code != 0 || r.JSON["invite_url"] != "https://crm--acme.whisk.page/.whisk/invite/abc" {
		t.Fatalf("customers invite: %+v", r)
	}
	if body := st.bodies["POST /v1/orgs/acme/apps/crm/customers"][0]; body["email"] != "new@customer.example" || body["name"] != "New" {
		t.Fatalf("invite body %v", body)
	}
	r = runRemote(t, dir, srv.URL, nil, "customers", "block", "PAT@customer.example", "--json")
	if r.Code != 0 || r.JSON["customer"].(map[string]any)["status"] != "blocked" {
		t.Fatalf("customers block: %+v", r)
	}
	if body := st.bodies["PATCH /v1/orgs/acme/apps/crm/customers/c1"][0]; body["status"] != "blocked" {
		t.Fatalf("block body %v", body)
	}
	r = runRemote(t, dir, srv.URL, nil, "customers", "unblock", "sam@customer.example", "--json")
	if r.Code != 0 || st.bodies["PATCH /v1/orgs/acme/apps/crm/customers/c2"][0]["status"] != "active" {
		t.Fatalf("customers unblock: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "customers", "remove", "sam@customer.example", "--json")
	if r.Code != 0 || r.JSON["removed"] != true || st.deleted[len(st.deleted)-1] != "/v1/orgs/acme/apps/crm/customers/c2" {
		t.Fatalf("customers remove: %+v %v", r, st.deleted)
	}
	r = runRemote(t, dir, srv.URL, nil, "customers", "remove", "nobody@customer.example", "--json")
	if r.Code != 1 || errCode(r) != "NOT_FOUND" {
		t.Fatalf("customers remove unknown: %+v", r)
	}
}

func TestTokensRuns(t *testing.T) {
	st := &dataState{}
	srv := dataAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "tokens", "list", "--json")
	if r.Code != 0 || len(r.JSON["tokens"].([]any)) != 1 {
		t.Fatalf("tokens list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "tokens", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "claude on laptop") || strings.Contains(r.Stdout, "whsk_") {
		t.Fatalf("tokens list human: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "tokens", "revoke", "t1", "--json")
	if r.Code != 0 || r.JSON["revoked"] != true || st.deleted[0] != "/v1/orgs/acme/tokens/t1" {
		t.Fatalf("tokens revoke: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "agent-token", "create", "--json")
	if r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
		t.Fatalf("agent-token without a label: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "agent-token", "create", "--label", "github actions", "--scopes", "deploy,secrets:declare", "--json")
	if r.Code != 0 || r.JSON["value"] != "whsk_agent_ONCE" || r.JSON["token"].(map[string]any)["id"] != "t2" {
		t.Fatalf("agent-token create: %+v", r)
	}
	if body := st.bodies["POST /v1/orgs/acme/tokens"][0]; body["label"] != "github actions" || len(body["scopes"].([]any)) != 2 {
		t.Fatalf("agent-token body %v", body)
	}

	r = runRemote(t, dir, srv.URL, nil, "runs", "list", "--function", "nightly", "--status", "parked", "--since", "6h", "--json")
	if r.Code != 0 || len(r.JSON["runs"].([]any)) != 1 {
		t.Fatalf("runs list: %+v", r)
	}
	// The function may be named as the one argument, the shape "the last runs of X" takes.
	r = runRemote(t, dir, srv.URL, nil, "runs", "list", "nightly", "--status", "parked", "--since", "6h", "--json")
	if r.Code != 0 || len(r.JSON["runs"].([]any)) != 1 {
		t.Fatalf("runs list <function>: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "runs", "list", "nightly", "--function", "other", "--json")
	if r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
		t.Fatalf("runs list with two functions: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "runs", "replay", "run1", "--json")
	if r.Code != 0 || r.JSON["run"].(map[string]any)["id"] != "run2" || r.JSON["replayed"] != "run1" {
		t.Fatalf("runs replay: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "runs", "cancel", "run1")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Cancelled run1 (cancelled)") {
		t.Fatalf("runs cancel: %+v", r)
	}

}

// whisk cron run starts one run of a cron function now and says how to follow it; the API's
// refusals reach the agent as they are (CLI.md §5.8).
func TestCronRun(t *testing.T) {
	st := &dataState{}
	srv := dataAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "cron", "run", "nightly", "--json")
	run, _ := r.JSON["run"].(map[string]any)
	if r.Code != 0 || run["id"] != "run3" || run["trigger"] != "cron" || run["manual"] != true || r.JSON["function"] != "nightly" || r.JSON["app"] != "crm" {
		t.Fatalf("cron run: %+v", r)
	}
	if len(st.bodies["POST /v1/orgs/acme/apps/crm/cron/nightly/run"]) != 1 {
		t.Fatalf("cron run calls: %v", st.bodies)
	}
	r = runRemote(t, dir, srv.URL, nil, "cron", "run", "nightly")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Started nightly as run3 (queued)") || !strings.Contains(r.Stdout, "whisk runs show run3") {
		t.Fatalf("cron run human: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "cron", "run", "slow")
	if r.Code != 0 || !strings.Contains(r.Stdout, "by its event evt9") || !strings.Contains(r.Stdout, "whisk runs list --event evt9") {
		t.Fatalf("cron run the engine has not listed yet: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "runs", "show", "run3")
	if r.Code != 0 || !strings.Contains(r.Stdout, "started: by hand (whisk cron run)") {
		t.Fatalf("runs show of a manual run: %+v", r)
	}
	for _, tc := range []struct{ function, code string }{
		{"canary-event", "INVALID_REQUEST"},
		{"fresh", "FUNCTION_NOT_LIVE"},
		{"nope", "NOT_FOUND"},
	} {
		r = runRemote(t, dir, srv.URL, nil, "cron", "run", tc.function, "--json")
		if r.Code != 1 || errCode(r) != tc.code {
			t.Errorf("cron run %s: %+v", tc.function, r)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "cron", "run", "--json")
	if r.Code == 0 {
		t.Fatalf("cron run with no function: %+v", r)
	}
}

// A run says what started it, and a listing marks one started by hand.
func TestRunTriggerAndWhy(t *testing.T) {
	for _, tc := range []struct {
		run     api.Run
		trigger string
		why     string
	}{
		{api.Run{Trigger: "cron", Manual: true}, "by hand (whisk cron run), as its schedule would", "started by hand"},
		{api.Run{Trigger: "cron"}, "on its schedule", ""},
		{api.Run{Trigger: "po.created"}, "on the event po.created", ""},
		{api.Run{Trigger: "replay"}, "by a replay", ""},
		{api.Run{Trigger: "cron", Manual: true, RerunOf: "run1"}, "by hand (whisk cron run), as its schedule would", "re-run of run1"},
		{api.Run{}, "", ""},
	} {
		if got := runTrigger(tc.run); got != tc.trigger {
			t.Errorf("runTrigger(%+v) = %q, want %q", tc.run, got, tc.trigger)
		}
		if got := runWhy(tc.run); got != tc.why {
			t.Errorf("runWhy(%+v) = %q, want %q", tc.run, got, tc.why)
		}
	}
}
