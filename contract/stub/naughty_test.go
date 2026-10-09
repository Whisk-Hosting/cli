package stub

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/naughty"
)

// serve sends one request with a raw path and headers, bypassing httptest.NewRequest's parser
// so a path can hold anything a client may send.
func serve(h http.Handler, method, path, rawQuery string, header http.Header, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.URL.Path, req.URL.RawPath, req.URL.RawQuery = path, "", rawQuery
	req.RequestURI = req.URL.RequestURI()
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// checkAnswer asserts the stub answered with a status it documents and, for an error, the
// standard body with a catalogued code: JSON for a client, an escaped page for a browser.
func checkAnswer(t *testing.T, where, s string, rec *httptest.ResponseRecorder, statuses ...int) {
	t.Helper()
	ok := false
	for _, st := range statuses {
		ok = ok || rec.Code == st
	}
	if !ok {
		t.Errorf("%s %q: status %d, want one of %v: %s", where, s, rec.Code, statuses, rec.Body.String())
	}
	if rec.Code < 400 {
		return
	}
	switch ct := rec.Header().Get("Content-Type"); {
	case strings.HasPrefix(ct, "text/html"):
		if strings.Contains(strings.ToLower(rec.Body.String()), "<script") {
			t.Errorf("%s %q: error page carries a script tag", where, s)
		}
	case ct == "application/json":
		var body werrors.Body
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %q: error body is not JSON: %v", where, s, err)
			return
		}
		if _, known := werrors.Lookup(body.Error.Code); !known || body.Error.Fix == "" {
			t.Errorf("%s %q: error %+v is not catalogued", where, s, body.Error)
		}
	}
}

// browserLocation is where a browser goes for a Location header on this host: tabs and
// newlines are removed and a backslash reads as a slash (WHATWG URL), so "/\evil" is "//evil".
func browserLocation(loc string) string {
	loc = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(loc)
	return strings.ReplaceAll(loc, `\`, "/")
}

// staysHere reports whether a redirect target keeps the browser on this hostname.
func staysHere(loc string) bool {
	b := browserLocation(loc)
	return strings.HasPrefix(b, "/") && !strings.HasPrefix(b, "//")
}

func TestNaughtySafeReturn(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, ret := range []string{s, "/" + s, "/\\" + s, "/\t/" + s, "//" + s} {
			if got := safeReturn(ret); !staysHere(got) || strings.ContainsAny(got, "\x00\r\n") {
				t.Errorf("safeReturn(%q) = %q leaves the host", ret, got)
			}
		}
	}
}

func TestNaughtyEdge(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	s.log = io.Discard
	e := newEdge(s)
	cookie := http.Header{"Cookie": {sessionCookie + "=sess"}}
	nav := http.Header{"Sec-Fetch-Mode": {"navigate"}, "Accept": {"text/html"}}
	for _, n := range naughty.Strings() {
		for _, p := range []string{"/" + n, "/embed/" + n, "/hooks/" + n} {
			// A path with a dot segment is refused as ambiguous, like the platform's edge does.
			checkAnswer(t, "edge GET", p, serve(e, "GET", p, "", http.Header{}, ""), 200, 400, 401, 502)
			checkAnswer(t, "edge navigation", p, serve(e, "GET", p, "", nav, ""), 200, 302, 400, 401, 502)
			checkAnswer(t, "edge signed-in POST", p, serve(e, "POST", p, "", http.Header{"Cookie": cookie["Cookie"], "Origin": {n}, "Sec-Fetch-Site": {n}}, n), 200, 400, 401, 403, 502)
		}
		checkAnswer(t, "edge challenge", n, serve(e, "POST", "/contact", "", http.Header{"X-Altcha": {n}, "Content-Type": {"application/x-www-form-urlencoded"}}, "altcha="+url.QueryEscape(n)), 403)
		for _, path := range []string{"/.whisk/login", "/.whisk/logout"} {
			rec := serve(e, "GET", path, "return="+url.QueryEscape(n), http.Header{}, "")
			checkAnswer(t, path, n, rec, 303)
			if loc := rec.Header().Get("Location"); !staysHere(loc) {
				t.Errorf("%s?return=%q redirects to %q, off the host", path, n, loc)
			}
		}
		h := http.Header{"X-Whisk-User-Id": {n}, "X-Whisk-Roles": {n}, "X-Whisk-Audience": {n}}
		rec := serve(e, "GET", "/whoami", "", h, "")
		var seen map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &seen); err != nil || seen["X-Whisk-Audience"] != "anonymous" || seen["X-Whisk-User-Id"] != "" {
			t.Errorf("forged identity %q reached the app: %s", n, rec.Body.String())
		}
	}
}

func TestNaughtyAPI(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	s.log = io.Discard
	s.apiURL = "http://api.test"
	a := newAPI(s)
	prefix := "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	hooks := "/hooks/" + s.orgID + "/" + s.appID
	bearer := http.Header{"Authorization": {"Bearer " + s.serviceToken}}
	for _, n := range naughty.Strings() {
		checkAnswer(t, "decide", n, serve(a, "POST", "/approvals/"+n, "", http.Header{}, `{"decision":"approved"}`), 404)
		checkAnswer(t, "decide body", n, serve(a, "POST", "/approvals/x", "", http.Header{}, n), 400, 404)
		checkAnswer(t, "events", n, serve(a, "GET", prefix+"/webhooks/"+n+"/events", "", http.Header{}, ""), 200, 404)
		checkAnswer(t, "replay", n, serve(a, "POST", prefix+"/webhooks/stripe/events/"+n+"/replay", "", http.Header{}, ""), 404)
		checkAnswer(t, "approvals", n, serve(a, "GET", prefix+"/approvals", "status="+url.QueryEscape(n), http.Header{}, ""), 200)
		checkAnswer(t, "enqueue", n, serve(a, "POST", prefix+"/events", "", bearer, n), 400, 503)
		checkAnswer(t, "enqueue token", n, serve(a, "POST", prefix+"/events", "", http.Header{"Authorization": {"Bearer " + n}}, `{"name":"x"}`), 401)
		checkAnswer(t, "unknown source", n, serve(a, "POST", hooks+"/"+n, "", http.Header{}, n), 401, 404)
		checkAnswer(t, "unknown org", n, serve(a, "POST", "/hooks/"+n, "", http.Header{}, n), 404)
		checkAnswer(t, "token source", n, serve(a, "POST", hooks+"/legacy/"+n, "", http.Header{}, n), 401)
		checkAnswer(t, "stripe", n, serve(a, "POST", hooks+"/stripe", "", http.Header{"Stripe-Signature": {n}, "Content-Type": {n}}, n), 401)
		checkAnswer(t, "workflow api", n, serve(a, "POST", "/e/"+n, "", http.Header{}, n), 404)
	}
	for _, e := range s.store.eventsFor("stripe") {
		if e.Verified {
			t.Errorf("a naughty delivery %q verified", e.BodyText)
		}
	}
	for _, e := range s.store.eventsFor("legacy") {
		if e.Verified {
			t.Errorf("a naughty token delivery %q verified", e.BodyText)
		}
	}
}

// The internal listener takes job headers from the run body; a naughty run id either reaches
// the app as that exact header or the request is refused, and never adds another header.
func TestNaughtyInternal(t *testing.T) {
	seen := make(chan http.Header, 1)
	app := echoApp(t, seen)
	defer app.Close()
	s := newTestStub(t, app)
	s.log = io.Discard
	in := newInternal(s)
	in.proxy.ErrorLog = log.New(io.Discard, "", 0)
	for _, n := range naughty.Strings() {
		id, _ := json.Marshal(n)
		rec := serve(in, "POST", s.manifest.Queue.Endpoint, "", http.Header{"X-Whisk-Audience": {"team"}}, `{"ctx":{"run_id":`+string(id)+`,"attempt":1}}`)
		checkAnswer(t, "internal", n, rec, 200, 502)
		if rec.Code != 200 {
			continue
		}
		h := <-seen
		if h.Get("X-Whisk-Audience") != "service" {
			t.Errorf("run %q: audience %q", n, h.Get("X-Whisk-Audience"))
		}
		var want string
		_ = json.Unmarshal(id, &want)
		if want != "" && h.Get("X-Whisk-Job-Id") != strings.TrimSpace(want) && h.Get("X-Whisk-Job-Id") != want {
			t.Errorf("run %q arrived as job %q", n, h.Get("X-Whisk-Job-Id"))
		}
		checkAnswer(t, "internal token", n, serve(in, "GET", "/"+n, "", http.Header{"Authorization": {"Bearer " + n}}, ""), 401)
	}
}

func TestNaughtyHelpers(t *testing.T) {
	a := altcha{key: []byte("k"), maxNumber: 1000}
	dir := t.TempDir()
	for _, n := range naughty.Strings() {
		if a.verify(n, time.Now()) || a.verify(base64.StdEncoding.EncodeToString([]byte(n)), time.Now()) {
			t.Errorf("altcha accepted %q", n)
		}
		sol, _ := json.Marshal(altchaSolution{Algorithm: "SHA-256", Challenge: n, Salt: n, Signature: n})
		if a.verify(base64.StdEncoding.EncodeToString(sol), time.Now()) {
			t.Errorf("altcha accepted a solution made of %q", n)
		}
		for _, item := range SplitList(n + "," + n) {
			if item == "" || item != strings.TrimSpace(item) {
				t.Errorf("SplitList(%q) gave %q", n, item)
			}
		}
		h := http.Header{"Set-Cookie": {n, "a=1; Domain=" + n + "; Path=/", "__Host-whisk_session=" + n}}
		bindCookies(h)
		for _, c := range h.Values("Set-Cookie") {
			if strings.Contains(strings.ToLower(c), "domain=") && !strings.Contains(strings.ToLower(n), "domain=") {
				t.Errorf("cookie %q kept a domain: %q", n, c)
			}
			if !strings.HasPrefix(c, "__Host-") || !strings.HasSuffix(c, "; Secure") && !strings.Contains(c, "Secure") {
				t.Errorf("cookie %q came out as %q, not bound to the host", n, c)
			}
			if strings.HasPrefix(strings.TrimSpace(c), "__Host-whisk_session=") {
				t.Errorf("the platform session cookie %q reached the browser", c)
			}
		}
		rec := httptest.NewRecorder()
		writeError(rec, httptest.NewRequest("GET", "/", nil), 400, werrors.New("INVALID_REQUEST", n, n, map[string]any{"input": n}))
		if !json.Valid(rec.Body.Bytes()) {
			t.Errorf("writeError(%q) is not JSON", n)
		}
		path := filepath.Join(dir, "secrets.env")
		if err := os.WriteFile(path, []byte("# secrets\nNAME="+n+"\n"+n+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := readSecrets(path); err == nil && got["NAME"] != strings.Trim(strings.TrimSpace(strings.SplitN(n, "\n", 2)[0]), `"`) {
			t.Errorf("readSecrets read NAME=%q as %q", n, got["NAME"])
		}
	}
}
