package stub

import (
	"encoding/json"
	"strings"
	"testing"
)

// The stub answers an app's own sending domains as the platform does: the service token is
// required, a domain registers once with its records, verifies when checked, lists, reads and is
// removed by its id; Whisk's own domains and names that are not domains are refused.
func TestStubEmailDomains(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	a := newAPI(s)
	base := "/v1/orgs/" + s.orgID + "/apps/" + s.appID + "/email/domains"
	auth := map[string]string{"Authorization": "Bearer whsk_service_test"}
	read := func(body string) emailDomain {
		t.Helper()
		var d emailDomain
		if err := json.Unmarshal([]byte(body), &d); err != nil {
			t.Fatalf("%v: %s", err, body)
		}
		return d
	}

	if rec, _ := get(t, a, "GET", base, nil, ""); rec.Code != 401 {
		t.Fatalf("without the token: %d", rec.Code)
	}
	rec, _ := get(t, a, "POST", base, auth, `{"domain":" @Mail.Customer.Example "}`)
	if rec.Code != 201 {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	d := read(rec.Body.String())
	if d.Domain != "mail.customer.example" || d.Status != "pending" || len(d.DNSRecords) != 3 || !strings.Contains(rec.Body.String(), "v=spf1") {
		t.Fatalf("registered %+v", d)
	}
	if rec, _ = get(t, a, "POST", base, auth, `{"domain":"mail.customer.example"}`); rec.Code != 200 || read(rec.Body.String()).ID != d.ID {
		t.Fatalf("again: %d %s", rec.Code, rec.Body.String())
	}
	for _, bad := range []struct{ domain, code string }{{"not a domain", "INVALID_REQUEST"}, {"mail.whisk.page", "EMAIL_DOMAIN_TAKEN"}} {
		if rec, _ = get(t, a, "POST", base, auth, `{"domain":"`+bad.domain+`"}`); !strings.Contains(rec.Body.String(), bad.code) {
			t.Fatalf("%s: %d %s", bad.domain, rec.Code, rec.Body.String())
		}
	}
	if rec, _ = get(t, a, "POST", base+"/"+d.ID+"/verify", auth, ""); rec.Code != 200 || read(rec.Body.String()).Status != "verified" {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	if rec, _ = get(t, a, "GET", base+"/"+d.ID, auth, ""); rec.Code != 200 || read(rec.Body.String()).VerifiedAt == nil {
		t.Fatalf("read: %d %s", rec.Code, rec.Body.String())
	}
	if rec, _ = get(t, a, "GET", base, auth, ""); !strings.Contains(rec.Body.String(), d.ID) {
		t.Fatalf("list: %s", rec.Body.String())
	}
	if rec, _ = get(t, a, "DELETE", base+"/"+d.ID, auth, ""); rec.Code != 204 {
		t.Fatalf("remove: %d", rec.Code)
	}
	if rec, _ = get(t, a, "GET", base+"/"+d.ID, auth, ""); rec.Code != 404 {
		t.Fatalf("after removal: %d", rec.Code)
	}
	if rec, _ = get(t, a, "GET", base, auth, ""); strings.TrimSpace(rec.Body.String()) != `{"items":[]}` {
		t.Fatalf("empty list: %s", rec.Body.String())
	}
}
