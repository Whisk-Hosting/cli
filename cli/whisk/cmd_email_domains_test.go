package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/apitypes"
)

func TestEmailDomainName(t *testing.T) {
	cases := map[string]string{
		"mail.acme.com":       "mail.acme.com",
		"  Mail.Acme.COM  ":   "mail.acme.com",
		"@mail.acme.com":      "mail.acme.com",
		"mail.acme.com.":      "mail.acme.com",
		"":                    "",
		" @ ":                 "",
		"orders@mail.acme.io": "orders@mail.acme.io",
	}
	for in, want := range cases {
		if got := emailDomainName(in); got != want {
			t.Errorf("emailDomainName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The name picks the domain exactly, whatever its case or a trailing dot, and never a near match:
// a parent, a subdomain or an empty name finds nothing, so nothing is acted on by a guess.
func TestFindEmailDomain(t *testing.T) {
	all := []api.EmailDomain{{ID: "ed_1", Domain: "mail.acme.com"}, {ID: "ed_2", Domain: "acme.com"}, {ID: "ed_3", Domain: "news.acme.com", App: "crm"}}
	cases := []struct {
		name string
		want string
	}{
		{"mail.acme.com", "ed_1"},
		{"MAIL.acme.com.", "ed_1"},
		{"acme.com", "ed_2"},
		{"news.acme.com", "ed_3"},
		{"x.mail.acme.com", ""},
		{"acme", ""},
		{"", ""},
	}
	for _, c := range cases {
		d, ok := findEmailDomain(all, c.name)
		if ok != (c.want != "") || d.ID != c.want {
			t.Errorf("findEmailDomain(%q) = %q, %v; want %q", c.name, d.ID, ok, c.want)
		}
	}
	if _, ok := findEmailDomain(nil, "mail.acme.com"); ok {
		t.Error("found a domain in an empty list")
	}
}

// The records are the API's own, host included when it sends one, while the domain is pending;
// a verified or paused domain has none to publish.
func TestEmailDomainRecords(t *testing.T) {
	pending := api.EmailDomain{Domain: "mail.acme.com", Status: apitypes.EmailDomainPending, DNSRecords: []apitypes.DNSRecord{
		{Type: "CNAME", Name: "abc._domainkey.mail.acme.com", Value: "abc.dkim.amazonses.com", Host: "abc._domainkey.mail"},
		{Type: "TXT", Name: "mail.acme.com", Value: "v=spf1 include:amazonses.com ~all"},
	}}
	verified, paused := pending, pending
	verified.Status, paused.Status = apitypes.EmailDomainVerified, apitypes.EmailDomainPaused
	cases := []struct {
		name string
		d    api.EmailDomain
		want []map[string]string
	}{
		{"pending", pending, []map[string]string{
			{"type": "CNAME", "name": "abc._domainkey.mail.acme.com", "host": "abc._domainkey.mail", "value": "abc.dkim.amazonses.com"},
			{"type": "TXT", "name": "mail.acme.com", "value": "v=spf1 include:amazonses.com ~all"}}},
		{"pending, nothing to publish", api.EmailDomain{Status: apitypes.EmailDomainPending}, []map[string]string{}},
		{"verified", verified, []map[string]string{}},
		{"paused", paused, []map[string]string{}},
	}
	for _, c := range cases {
		if got := emailDomainRecords(c.d); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: emailDomainRecords = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEmailDomainSays(t *testing.T) {
	cases := []struct {
		d    api.EmailDomain
		want string
	}{
		{api.EmailDomain{Domain: "mail.acme.com", Status: apitypes.EmailDomainVerified}, "mail.acme.com is verified: the business's apps send from any address at it."},
		{api.EmailDomain{Domain: "news.client.com", App: "crm", Status: apitypes.EmailDomainVerified}, "news.client.com is verified: crm alone sends from any address at it."},
		{api.EmailDomain{Domain: "mail.acme.com", Status: apitypes.EmailDomainPending}, "mail.acme.com is pending: its DNS records are not verified yet."},
		{api.EmailDomain{Domain: "mail.acme.com", Status: "retired"}, "mail.acme.com is retired."},
		{api.EmailDomain{Domain: "mail.acme.com", Status: apitypes.EmailDomainPaused, BounceRate: 0.062, ComplaintRate: 0.0011},
			"mail.acme.com is paused: nothing is sent from it (bounce rate 6.2%, complaint rate 0.11%). An owner resumes it in the dashboard under Settings, Email, once the cause is fixed."},
	}
	for _, c := range cases {
		if got := emailDomainSays(c.d); got != c.want {
			t.Errorf("emailDomainSays(%s) = %q, want %q", c.d.Status, got, c.want)
		}
	}
}

// whisk email domains add, list, verify and remove drive the business's sending domains through
// the routes the dashboard uses: add prints the records as whisk domains add does, verify and
// remove resolve the name to its id from the list and act on that id only, an unknown name
// changes nothing, and every refusal passes through with its code.
func TestEmailDomainsCommands(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	records := []any{
		map[string]any{"type": "CNAME", "name": "abc._domainkey.mail.acme.com", "value": "abc.dkim.amazonses.com", "host": "abc._domainkey.mail"},
		map[string]any{"type": "MX", "name": "bounce.mail.acme.com", "value": "10 feedback-smtp.eu-west-1.amazonses.com", "host": "bounce.mail"},
	}
	pending := map[string]any{"id": "ed_1", "domain": "mail.acme.com", "status": "pending", "dns_records": records, "bounce_rate": 0, "complaint_rate": 0, "created_at": "2026-11-09T09:00:00Z"}
	verified := map[string]any{"id": "ed_2", "domain": "news.acme.com", "status": "verified", "dns_records": []any{}, "bounce_rate": 0, "complaint_rate": 0,
		"verified_at": "2026-11-08T09:00:00Z", "created_at": "2026-11-08T08:00:00Z"}
	empty := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/email/domains", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "list")
		mu.Unlock()
		if empty {
			writeJSON(w, 200, map[string]any{"items": []any{}})
			return
		}
		writeJSON(w, 200, map[string]any{"items": []any{pending, verified}})
	})
	mux.HandleFunc("POST /v1/orgs/acme/email/domains", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Domain string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("add body: %v", err)
		}
		mu.Lock()
		calls = append(calls, "add "+in.Domain)
		mu.Unlock()
		if in.Domain == "whisk.page" {
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "EMAIL_DOMAIN_TAKEN", "message": "whisk.page is one of Whisk's own domains, so no business can send from it.", "fix": "Send from a domain your business owns."}})
			return
		}
		writeJSON(w, 201, pending)
	})
	mux.HandleFunc("POST /v1/orgs/acme/email/domains/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "verify "+r.PathValue("id"))
		mu.Unlock()
		if r.PathValue("id") == "ed_1" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "DOMAIN_UNVERIFIED", "message": "mail.acme.com is not verified yet: the DNS records have not appeared.",
				"fix": "Publish the records in details and try again; DNS can take up to an hour.", "details": map[string]any{"domain": "mail.acme.com", "records": records}}})
			return
		}
		writeJSON(w, 200, verified)
	})
	mux.HandleFunc("DELETE /v1/orgs/acme/email/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, "delete "+r.PathValue("id"))
		mu.Unlock()
		if r.PathValue("id") == "ed_2" {
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "AGENT_CATEGORY", "message": "This agent identity does not hold the Human category.",
				"fix": "Ask your person to do it signed in at the dashboard.", "details": map[string]any{"required": "human"}}})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": r.URL.Path, "fix": "check the path"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	took := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := calls
		calls = nil
		return out
	}

	// add: the records as whisk domains add prints them, host and full name, then the next step.
	r := runRemote(t, dir, srv.URL, nil, "email", "domains", "add", " @Mail.Acme.com ")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Create these DNS records for mail.acme.com, then run whisk email domains verify mail.acme.com:") ||
		!strings.Contains(r.Stdout, "TYPE") || !strings.Contains(r.Stdout, "HOST") || !strings.Contains(r.Stdout, "abc._domainkey.mail ") ||
		!strings.Contains(r.Stdout, "bounce.mail.acme.com") || !strings.Contains(r.Stdout, "10 feedback-smtp.eu-west-1.amazonses.com") {
		t.Errorf("add for a person: %+v", r)
	}
	if got := took(); !reflect.DeepEqual(got, []string{"add mail.acme.com"}) {
		t.Errorf("add asked %v", got)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "add", "mail.acme.com", "--json")
	recs, _ := r.JSON["records"].([]any)
	d, _ := r.JSON["domain"].(map[string]any)
	if r.Code != 0 || r.JSON["org"] != "acme" || d["id"] != "ed_1" || d["status"] != "pending" || len(recs) != 2 || recs[0].(map[string]any)["host"] != "abc._domainkey.mail" {
		t.Errorf("add --json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "add", "whisk.page", "--json")
	if r.Code != 1 || errCode(r) != "EMAIL_DOMAIN_TAKEN" {
		t.Errorf("a refused add: %+v", r)
	}
	took()

	// list
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "DOMAIN") || !strings.Contains(r.Stdout, "mail.acme.com") || !strings.Contains(r.Stdout, "news.acme.com") || !strings.Contains(r.Stdout, "verified") {
		t.Errorf("list for a person: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "list", "--json")
	if all, _ := r.JSON["domains"].([]any); r.Code != 0 || r.JSON["org"] != "acme" || len(all) != 2 {
		t.Errorf("list --json: %+v", r)
	}
	empty = true
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "list")
	if r.Code != 0 || !strings.Contains(r.Stdout, "acme has no sending domains; add one with whisk email domains add <domain>.") {
		t.Errorf("an empty list: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "list", "--json")
	if all, ok := r.JSON["domains"].([]any); r.Code != 0 || !ok || len(all) != 0 {
		t.Errorf("an empty list --json is an empty array: %+v", r)
	}
	empty = false
	took()

	// verify: by name to id; records not there yet pass DOMAIN_UNVERIFIED through with them.
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "verify", "news.acme.com")
	if r.Code != 0 || !strings.Contains(r.Stdout, "news.acme.com is verified: the business's apps send from any address at it.") {
		t.Errorf("verify for a person: %+v", r)
	}
	if got := took(); !reflect.DeepEqual(got, []string{"list", "verify ed_2"}) {
		t.Errorf("verify asked %v", got)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "verify", "mail.acme.com", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	details, _ := e["details"].(map[string]any)
	if r.Code != 1 || errCode(r) != "DOMAIN_UNVERIFIED" || len(details["records"].([]any)) != 2 {
		t.Errorf("verify before the records are published: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "verify", "news.acme.com", "--json")
	if d, _ := r.JSON["domain"].(map[string]any); r.Code != 0 || d["status"] != "verified" || len(r.JSON["records"].([]any)) != 0 {
		t.Errorf("verify --json: %+v", r)
	}
	took()
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "verify", "acme.com", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code != 1 || errCode(r) != "NOT_FOUND" || !strings.Contains(e["fix"].(string), "whisk email domains add acme.com") {
		t.Errorf("verify of an unknown name: %+v", r)
	}
	if got := took(); !reflect.DeepEqual(got, []string{"list"}) {
		t.Errorf("an unknown name was acted on: %v", got)
	}

	// remove: the id of exactly the name given; an unknown or near name deletes nothing.
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "remove", "MAIL.acme.com")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Removed mail.acme.com.") {
		t.Errorf("remove for a person: %+v", r)
	}
	if got := took(); !reflect.DeepEqual(got, []string{"list", "delete ed_1"}) {
		t.Errorf("remove asked %v", got)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "remove", "mail.acme.com", "--json")
	if r.Code != 0 || r.JSON["domain"] != "mail.acme.com" || r.JSON["removed"] != true {
		t.Errorf("remove --json: %+v", r)
	}
	took()
	for _, name := range []string{"acme.com", "x.mail.acme.com"} {
		r = runRemote(t, dir, srv.URL, nil, "email", "domains", "remove", name, "--json")
		if r.Code != 1 || errCode(r) != "NOT_FOUND" {
			t.Errorf("remove of %s: %+v", name, r)
		}
		if got := took(); !reflect.DeepEqual(got, []string{"list"}) {
			t.Errorf("remove of %s deleted by a guess: %v", name, got)
		}
	}
	// An agent identity is refused by the platform: the refusal passes through, exit 4.
	r = runRemote(t, dir, srv.URL, nil, "email", "domains", "remove", "news.acme.com", "--json")
	if r.Code != 4 || errCode(r) != "AGENT_CATEGORY" {
		t.Errorf("remove refused for an agent: %+v", r)
	}
	took()

	for _, args := range [][]string{{"email", "domains", "add"}, {"email", "domains", "verify"}, {"email", "domains", "remove"}, {"email", "domains", "list", "x"}} {
		if r := runRemote(t, dir, srv.URL, nil, args...); r.Code == 0 {
			t.Errorf("%v with the wrong arguments: %+v", args, r)
		}
	}
	if got := took(); len(got) != 0 {
		t.Errorf("wrong arguments reached the API: %v", got)
	}
}
