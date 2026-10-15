package stub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/contract/apitypes"
)

// The stub answers the custom domain routes with the app's service token: an added domain
// carries its records; a name under .test or .example verifies at once and any other answers
// DOMAIN_UNVERIFIED; a name already added is DOMAIN_TAKEN; removal is by id.
func TestDomains(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	a := newAPI(s)
	base := "/v1/orgs/" + s.orgID + "/apps/" + s.appID + "/domains"
	call := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer "+s.serviceToken)
		}
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, req)
		return rec
	}
	code := func(rec *httptest.ResponseRecorder) string {
		var out struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Error.Code
	}
	domain := func(rec *httptest.ResponseRecorder) apitypes.Domain {
		var d apitypes.Domain
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}

	if rec := call("GET", base, "", false); rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := call("GET", base, "", true); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{`{"hostname":"10.0.0.1"}`, `{"hostname":"x.whisk.page"}`, `not json`} {
		if rec := call("POST", base, bad, true); rec.Code != 400 || code(rec) != "INVALID_REQUEST" {
			t.Errorf("POST %s: %d %s", bad, rec.Code, code(rec))
		}
	}

	rec := call("POST", base, `{"hostname":"Results.Lab.test."}`, true)
	if rec.Code != 201 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	local := domain(rec)
	if local.Hostname != "results.lab.test" || local.Status != apitypes.DomainPendingDNS || local.AddedBy != apitypes.DomainAddedByApp || len(local.Records) != 2 || local.Records[0].Type != "TXT" || local.Records[1].Type != "CNAME" {
		t.Fatalf("added %+v", local)
	}
	if rec := call("POST", base, `{"hostname":"results.lab.test"}`, true); rec.Code != 409 || code(rec) != "DOMAIN_TAKEN" {
		t.Errorf("again: %d %s", rec.Code, code(rec))
	}
	rec = call("POST", base+"/"+local.ID+"/verify", "", true)
	if v := domain(rec); rec.Code != 200 || !v.Verified || v.Status != apitypes.DomainActive || len(v.Records) != 0 {
		t.Errorf("verify a .test name: %d %+v", rec.Code, v)
	}

	rec = call("POST", base, `{"hostname":"results.lab.com"}`, true)
	real := domain(rec)
	if rec := call("POST", base+"/"+real.ID+"/verify", "", true); rec.Code != 409 || code(rec) != "DOMAIN_UNVERIFIED" {
		t.Errorf("verify a real name: %d %s", rec.Code, code(rec))
	}
	if rec := call("DELETE", base+"/"+real.ID, "", true); rec.Code != http.StatusNoContent {
		t.Errorf("remove: %d", rec.Code)
	}
	if rec := call("DELETE", base+"/"+real.ID, "", true); rec.Code != 404 {
		t.Errorf("remove again: %d", rec.Code)
	}
	var list apitypes.List[apitypes.Domain]
	_ = json.Unmarshal(call("GET", base, "", true).Body.Bytes(), &list)
	if len(list.Items) != 1 || list.Items[0].ID != local.ID {
		t.Errorf("list %+v", list.Items)
	}
}
