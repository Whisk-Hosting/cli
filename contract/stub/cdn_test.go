package stub

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk dev answers a CDN purge as the platform does for an app no CDN serves, and refuses the
// paths the platform refuses.
func TestCDNPurge(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	a := newAPI(s)
	path := "/v1/orgs/" + s.orgID + "/apps/" + s.appID + "/cdn/purge"
	call := func(body string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer "+s.serviceToken)
		}
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(`{}`, false); rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	for _, body := range []string{`{}`, `{"paths":["/entries/42","/entries/*"]}`} {
		if rec := call(body, true); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"on":false`) || !strings.Contains(rec.Body.String(), `"urls":[]`) {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	for _, body := range []string{`{"paths":["entries"]}`, `{"paths":["/a*/b"]}`, `not json`} {
		if rec := call(body, true); rec.Code != 400 || !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
}
