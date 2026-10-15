package whisk

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
)

func TestPickLoginContext(t *testing.T) {
	bound := config.Binding{Org: "acme", App: "crm"}
	for _, tc := range []struct {
		name             string
		orgFlag, appFlag string
		b                config.Binding
		ok               bool
		wantOrg, wantApp string
	}{
		{"nothing", "", "", config.Binding{}, false, "", ""},
		{"the binding", "", "", bound, true, "acme", "crm"},
		{"--org for another business drops the binding's app", "globex", "", bound, true, "globex", ""},
		{"--org and --app", "globex", "site", bound, true, "globex", "site"},
		{"--app in the bound business", "", "site", bound, true, "acme", "site"},
		{"--app with no business", "", "site", config.Binding{}, false, "", ""},
		{"--org alone, unbound", "acme", "", config.Binding{}, false, "acme", ""},
	} {
		org, app := pickLoginContext(tc.orgFlag, tc.appFlag, tc.b, tc.ok)
		if org != tc.wantOrg || app != tc.wantApp {
			t.Errorf("%s: %q/%q, want %q/%q", tc.name, org, app, tc.wantOrg, tc.wantApp)
		}
	}
}

func TestContextRefused(t *testing.T) {
	old := &api.Error{Code: "INVALID_REQUEST", Message: `The request body is not the expected JSON: json: unknown field "org"`}
	for _, tc := range []struct {
		name string
		req  api.DeviceCodeRequest
		err  error
		want bool
	}{
		{"an older platform refuses the field", api.DeviceCodeRequest{Org: "acme"}, old, true},
		{"no context was sent", api.DeviceCodeRequest{}, old, false},
		{"another refusal", api.DeviceCodeRequest{Org: "acme"}, &api.Error{Code: "INVALID_REQUEST", Message: "no key"}, false},
		{"rate limited", api.DeviceCodeRequest{Org: "acme"}, &api.Error{Code: "RATE_LIMITED", Message: "unknown field"}, false},
		{"unreachable", api.DeviceCodeRequest{Org: "acme"}, errors.New("unknown field"), false},
		{"no error", api.DeviceCodeRequest{Org: "acme"}, nil, false},
	} {
		if got := contextRefused(tc.req, tc.err); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

func TestNeedsHumanLogin(t *testing.T) {
	link := "https://whisk.run/me/logins/t1?add=northwind"
	e := needsHumanLogin(&api.Error{Status: 403, Code: "BUSINESS_NOT_IN_LOGIN", Message: "This login does not cover Northwind Bakery.",
		Details: map[string]any{"url": link, "org": "northwind", "org_name": "Northwind Bakery", "login": "t1"}})
	if e == nil || e.Code != "NEEDS_HUMAN" || e.Details["url"] != link || e.Details["reason"] != "BUSINESS_NOT_IN_LOGIN" || !strings.Contains(e.Fix, "Add Northwind Bakery") {
		t.Fatalf("%+v", e)
	}
	if needsHumanLogin(&api.Error{Code: "BUSINESS_NOT_IN_LOGIN"}) != nil || needsHumanLogin(&api.Error{Code: "TOKEN_SCOPE", Details: map[string]any{"url": link}}) != nil {
		t.Fatal("converted an error without a link, or another code")
	}
}

// contextAPI is a platform's device code endpoint, current (it keeps the context) or from before
// login context (its strict decoder refuses fields it does not know).
type contextAPI struct {
	mu       sync.Mutex
	old      bool
	requests []map[string]string
}

func (c *contextAPI) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/device/code", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		c.mu.Lock()
		c.requests = append(c.requests, in)
		c.mu.Unlock()
		for k := range in {
			if c.old && k != "public_key" && k != "device_name" && k != "os" {
				writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": `The request body is not the expected JSON: json: unknown field "` + k + `"`, "fix": "Send the documented fields only."}})
				return
			}
		}
		writeJSON(w, 200, map[string]any{"device_code": "dc-c", "user_code": "CCCC-DDDD", "verification_url": "https://whisk.run/device?code=CCCC-DDDD", "interval": 1, "expires_in": 60})
	})
	mux.HandleFunc("GET /v1/orgs/{org}/apps", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "BUSINESS_NOT_IN_LOGIN", "message": "This login does not cover Northwind Bakery.",
			"fix": "Ask the person to open the link.", "details": map[string]any{"url": "https://whisk.run/me/logins/t1?add=northwind", "org": "northwind", "org_name": "Northwind Bakery"}}})
	})
	return httptest.NewServer(mux)
}

// whisk login sends the directory's business and app with the code request, so the approval page
// starts on them; a platform from before that refuses the fields, and login asks again without.
func TestLoginSendsWhatItWorksOn(t *testing.T) {
	for _, old := range []bool{false, true} {
		c := &contextAPI{old: old}
		srv := c.server()
		dir := t.TempDir()
		must(t, config.SaveBinding(dir, config.Binding{Org: "acme", App: "crm", API: srv.URL}))
		r := runCLI(t, dir, map[string]string{"WHISK_API": srv.URL}, &memStore{m: map[string]config.Credential{}}, "login", "--no-wait", "--json")
		srv.Close()
		if r.Code != 2 || r.JSON["error"].(map[string]any)["code"] != "NEEDS_HUMAN" {
			t.Fatalf("old=%v: %+v", old, r)
		}
		first := c.requests[0]
		if first["org"] != "acme" || first["app"] != "crm" || first["public_key"] == "" {
			t.Fatalf("old=%v: first request %v", old, first)
		}
		if old && (len(c.requests) != 2 || c.requests[1]["org"] != "" || c.requests[1]["public_key"] == "") {
			t.Fatalf("an older platform: requests %v", c.requests)
		}
		if !old && len(c.requests) != 1 {
			t.Fatalf("a current platform was asked twice: %v", c.requests)
		}
	}
}

// A login used in a business it does not cover prints the NEEDS_HUMAN block with the link, plainly,
// and exits 2.
func TestBusinessNotInLoginNeedsHuman(t *testing.T) {
	c := &contextAPI{}
	srv := c.server()
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{"default": {Token: "whsk_agent_x", API: srv.URL}}}
	env := map[string]string{"WHISK_API": srv.URL}
	r := runCLI(t, t.TempDir(), env, store, "apps", "list", "--org", "northwind")
	if r.Code != 2 || !strings.Contains(r.Stderr, "NEEDS_HUMAN") || !strings.Contains(r.Stderr, "https://whisk.run/me/logins/t1?add=northwind") || !strings.Contains(r.Stderr, "Add Northwind Bakery") {
		t.Fatalf("%+v", r)
	}
	r = runCLI(t, t.TempDir(), env, store, "apps", "list", "--org", "northwind", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || e["details"].(map[string]any)["reason"] != "BUSINESS_NOT_IN_LOGIN" {
		t.Fatalf("json: %+v", r)
	}
}
