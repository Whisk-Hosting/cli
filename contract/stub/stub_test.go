package stub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/webhook"
)

const testManifest = `
whisk: 1
name: demo
routes:
  public: ["/", "/health", "/whoami", "/contact"]
  challenge: ["/contact"]
  csrf_off: ["/embed/*"]
secrets: [STRIPE_WEBHOOK_SECRET]
webhooks:
  - { name: stripe, preset: stripe, secret: STRIPE_WEBHOOK_SECRET, handler: /hooks/stripe }
  - { name: legacy, preset: token, handler: /hooks/legacy, handshake: { query: validationToken } }
`

// echoApp stands in for a template: it echoes the X-Whisk-* headers it received as JSON.
func echoApp(t *testing.T, seen chan<- http.Header) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			seen <- r.Header.Clone()
		}
		out := map[string]string{}
		for k, v := range r.Header {
			if strings.HasPrefix(k, "X-Whisk-") {
				out[k] = v[0]
			}
		}
		if r.URL.Path == "/hooks/stripe" {
			body, _ := io.ReadAll(r.Body)
			out["body"] = string(body)
		}
		w.Header().Set("Set-Cookie", "app=1; Domain=evil.example; Path=/")
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func newTestStub(t *testing.T, app *httptest.Server) *stub {
	m, err := manifest.Parse([]byte(testManifest))
	if err != nil {
		t.Fatal(err)
	}
	up, _ := url.Parse(app.URL)
	s := &stub{
		manifest: m, orgID: newULID(), appID: newULID(), upstream: up,
		serviceToken: "whsk_service_test", sessionValue: "sess", sessionID: newULID(),
		deliveryKey: randomBytes(32), deliveryMarker: randomToken(16), mediaKey: randomBytes(32),
		secrets: map[string]string{"STRIPE_WEBHOOK_SECRET": "whsec_test"}, urlTokens: map[string]string{"legacy": "tok123"},
		bodyLimit: 8 << 20, altcha: altcha{key: []byte("k"), maxNumber: 1000}, store: newStore(),
		identity: &identity{UserID: "01USER", Email: "ana@acme.example", Name: "Ana", Groups: []string{"finance"}, Roles: []string{"owner"}, Audience: "team"},
	}
	s.hooks = newHooks(s)
	s.hooks.retryAfter = []time.Duration{10 * time.Millisecond}
	return s
}

func get(t *testing.T, h http.Handler, method, path string, hdr map[string]string, body string) (*httptest.ResponseRecorder, map[string]string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]string{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestEdgeIdentity(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	e := newEdge(s)
	cookie := map[string]string{"Cookie": sessionCookie + "=sess"}

	rec, out := get(t, e, "GET", "/whoami", map[string]string{"X-Whisk-User-Id": "forged", "X-Whisk-Roles": "owner"}, "")
	if rec.Code != 200 || out["X-Whisk-Audience"] != "anonymous" || out["X-Whisk-User-Id"] != "" {
		t.Fatalf("public anonymous: %d %v", rec.Code, out)
	}
	if out["X-Whisk-Request-Id"] == "" || rec.Header().Get("X-Whisk-Request-Id") == "" {
		t.Fatal("request id missing")
	}
	rec, out = get(t, e, "GET", "/whoami", cookie, "")
	if out["X-Whisk-Audience"] != "team" || out["X-Whisk-Email"] != "ana@acme.example" || out["X-Whisk-Roles"] != "owner" || out["X-Whisk-Groups"] != "finance" {
		t.Fatalf("public signed in: %v", out)
	}
	rec, _ = get(t, e, "GET", "/notes", nil, "")
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("private fetch without session: %d", rec.Code)
	}
	rec, _ = get(t, e, "GET", "/notes", map[string]string{"Sec-Fetch-Mode": "navigate", "Accept": "text/html"}, "")
	if rec.Code != 302 || !strings.HasPrefix(rec.Header().Get("Location"), "/.whisk/login?return=") {
		t.Fatalf("private navigation without session: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec, out = get(t, e, "GET", "/notes", cookie, "")
	if rec.Code != 200 || out["X-Whisk-User-Id"] != "01USER" || rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("private signed in: %d %v %s", rec.Code, out, rec.Header().Get("Cache-Control"))
	}
	if sc := rec.Header().Get("Set-Cookie"); strings.Contains(strings.ToLower(sc), "domain=") || !strings.Contains(sc, "Secure") || !strings.HasPrefix(sc, "__Host-") {
		t.Fatalf("cookie not bound to the host: %s", sc)
	}
	rec, _ = get(t, e, "GET", "/hooks/stripe", cookie, "")
	if rec.Code != 401 {
		t.Fatalf("service route from the edge must be 401, got %d", rec.Code)
	}
	rec, _ = get(t, e, "GET", "/.whisk/login?return=/notes", nil, "")
	if rec.Code != 303 || rec.Header().Get("Location") != "/notes" || !strings.Contains(rec.Header().Get("Set-Cookie"), sessionCookie+"=sess") {
		t.Fatalf("login: %d %v", rec.Code, rec.Header())
	}
	rec, _ = get(t, e, "GET", "/.whisk/logout?return=//evil.example", cookie, "")
	if rec.Code != 303 || rec.Header().Get("Location") != "/" || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout: %d %v", rec.Code, rec.Header())
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Server") != "whisk" {
		t.Fatal("security headers missing")
	}
}

func TestEdgeCSRFAndChallenge(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	e := newEdge(s)
	cookie := sessionCookie + "=sess"

	rec, _ := get(t, e, "POST", "/notes", map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "cross-site"}, "{}")
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "CSRF_REJECTED") {
		t.Fatalf("cross-site POST: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, e, "POST", "/notes", map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"}, "{}")
	if rec.Code != 200 {
		t.Fatalf("same-origin POST: %d", rec.Code)
	}
	rec, _ = get(t, e, "POST", "/notes", map[string]string{"Cookie": cookie, "Origin": "https://evil.example"}, "{}")
	if rec.Code != 403 {
		t.Fatalf("foreign Origin POST: %d", rec.Code)
	}
	rec, _ = get(t, e, "POST", "/embed/x", map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "cross-site"}, "{}")
	if rec.Code != 200 {
		t.Fatalf("csrf_off route: %d", rec.Code)
	}
	rec, _ = get(t, e, "POST", "/contact", map[string]string{"Sec-Fetch-Site": "same-origin"}, "name=x")
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "CHALLENGE_REQUIRED") {
		t.Fatalf("challenge missing: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error struct {
			Details struct {
				Challenge altchaChallenge `json:"challenge"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	sol, ok := solve(body.Error.Details.Challenge)
	if !ok {
		t.Fatal("challenge not solvable")
	}
	rec, _ = get(t, e, "POST", "/contact", map[string]string{"Sec-Fetch-Site": "same-origin", "X-Altcha": encodeSolution(sol)}, "name=x")
	if rec.Code != 200 {
		t.Fatalf("solved challenge: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, e, "GET", "/contact", nil, "")
	if rec.Code != 200 {
		t.Fatalf("GET on a challenge route must pass: %d", rec.Code)
	}
}

func TestInternalServiceIdentity(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	in := newInternal(s)
	rec, out := get(t, in, "POST", "/.whisk/inngest", map[string]string{"X-Whisk-User-Id": "forged"}, `{"ctx":{"run_id":"01RUN","attempt":2}}`)
	if rec.Code != 200 || out["X-Whisk-Audience"] != "service" || out["X-Whisk-User-Id"] != "" || out["X-Whisk-Service-App"] != "" {
		t.Fatalf("platform delivery: %d %v", rec.Code, out)
	}
	if out["X-Whisk-Job-Id"] != "01RUN" || out["X-Whisk-Job-Attempt"] != "2" {
		t.Fatalf("job headers: %v", out)
	}
	rec, out = get(t, in, "GET", "/me", map[string]string{"Authorization": "Bearer whsk_service_test"}, "")
	if rec.Code != 200 || out["X-Whisk-Service-App"] != s.appID {
		t.Fatalf("app-to-app: %d %v", rec.Code, out)
	}
	// An app-to-app call cannot carry the platform's delivery headers: they are stripped, and
	// the stub's own private marker never reaches the app.
	forged := map[string]string{"Authorization": "Bearer whsk_service_test", "X-Whisk-Webhook-Id": "01FAKE", "X-Whisk-Delivery-Signature": "v1=00", stubDeliveryMarker: "guess"}
	rec, out = get(t, in, "POST", "/hooks/stripe", forged, "{}")
	if rec.Code != 200 || out["X-Whisk-Webhook-Id"] != "" || out["X-Whisk-Delivery-Signature"] != "" || out["X-Whisk-Service-App"] != s.appID {
		t.Fatalf("forged delivery headers survived an app-to-app call: %d %v", rec.Code, out)
	}
	rec, _ = get(t, in, "GET", "/me", map[string]string{"Authorization": "Bearer nope"}, "")
	if rec.Code != 401 {
		t.Fatalf("bad service token: %d", rec.Code)
	}
}

func TestWebhookIngress(t *testing.T) {
	seen := make(chan http.Header, 4)
	app := echoApp(t, seen)
	defer app.Close()
	s := newTestStub(t, app)
	// The stub delivers through its internal listener; point it at a real one.
	internalSrv := httptest.NewServer(newInternal(s))
	defer internalSrv.Close()
	s.internalURL = internalSrv.URL
	s.apiURL = "http://api.test"
	a := newAPI(s)

	body := `{"id":"evt_1","type":"invoice.paid"}`
	ts := time.Now().Unix()
	p := webhook.Presets()["stripe"]
	hookURL := "/hooks/" + s.orgID + "/" + s.appID + "/stripe"
	sig := webhook.Sign(p, "whsec_test", webhook.Request{Method: "POST", URL: s.apiURL + hookURL, Body: []byte(body)}, itoa(ts), "")
	rec, out := get(t, a, "POST", hookURL, map[string]string{"Stripe-Signature": sig, "Content-Type": "application/json"}, body)
	if rec.Code != 200 || out["id"] == "" {
		t.Fatalf("verified delivery: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case h := <-seen:
		if h.Get("X-Whisk-Webhook-Id") != out["id"] || h.Get("X-Whisk-Audience") != "service" || h.Get("X-Whisk-Webhook-Source") != "stripe" || h.Get("X-Whisk-Webhook-Orig-Stripe-Signature") != sig {
			t.Fatalf("delivery headers: %v", h)
		}
		if why := webhook.VerifyDelivery(s.deliveryKey, h, []byte(body)); why != "" || h.Get(stubDeliveryMarker) != "" || h.Get("X-Whisk-Service-App") != "" {
			t.Fatalf("delivery is not signed as the platform's (%q): %v", why, h)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no delivery")
	}
	rec, _ = get(t, a, "POST", hookURL, map[string]string{"Stripe-Signature": "t=" + itoa(ts) + ",v1=" + strings.Repeat("0", 64)}, body)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "WEBHOOK_UNVERIFIED") {
		t.Fatalf("forged delivery: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, a, "GET", "/v1/orgs/"+s.orgID+"/apps/"+s.appID+"/webhooks/stripe/events", nil, "")
	var list struct {
		Events []webhookEvent `json:"events"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Events) != 2 || !list.Events[0].Verified || list.Events[1].Verified || list.Events[1].DeliveryStatus != "never_delivered" {
		t.Fatalf("events list: %s", rec.Body.String())
	}
	rec, _ = get(t, a, "POST", "/v1/orgs/"+s.orgID+"/apps/"+s.appID+"/webhooks/stripe/events/"+list.Events[0].ID+"/replay", nil, "")
	if rec.Code != 202 {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case h := <-seen:
		if h.Get("X-Whisk-Webhook-Id") != list.Events[0].ID {
			t.Fatal("replay must reuse the webhook id")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no replay delivery")
	}
	rec, _ = get(t, a, "POST", "/hooks/"+s.orgID+"/"+s.appID+"/legacy/tok123", nil, "x")
	if rec.Code != 200 {
		t.Fatalf("token source with token: %d", rec.Code)
	}
	rec, _ = get(t, a, "POST", "/hooks/"+s.orgID+"/"+s.appID+"/legacy", nil, "x")
	if rec.Code != 401 {
		t.Fatalf("token source without token: %d", rec.Code)
	}
	before := len(s.store.eventsFor("legacy"))
	hs := httptest.NewRecorder()
	a.ServeHTTP(hs, httptest.NewRequest("POST", "/hooks/"+s.orgID+"/"+s.appID+"/legacy/tok123?validationToken=abc-1", nil))
	if hs.Code != 200 || hs.Body.String() != "abc-1" || !strings.HasPrefix(hs.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("handshake: %d %q", hs.Code, hs.Body.String())
	}
	hs = httptest.NewRecorder()
	a.ServeHTTP(hs, httptest.NewRequest("POST", "/hooks/"+s.orgID+"/"+s.appID+"/legacy?validationToken=abc-1", nil))
	if hs.Code != 401 || strings.Contains(hs.Body.String(), "abc-1") {
		t.Fatalf("handshake without the token: %d %q", hs.Code, hs.Body.String())
	}
	if len(s.store.eventsFor("legacy")) != before {
		t.Fatal("a handshake was stored as a delivery")
	}
}

func TestQueueAndApprovals(t *testing.T) {
	var received []map[string]any
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var evt map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &evt)
		received = append(received, evt)
		_ = json.NewEncoder(w).Encode(map[string]any{"ids": []string{"01EVT"}, "status": 200})
	}))
	defer engine.Close()
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	s.inngestURL, _ = url.Parse(engine.URL)
	s.eventKey = "k"
	a := newAPI(s)
	events := "/v1/orgs/" + s.orgID + "/apps/" + s.appID + "/events"

	rec, _ := get(t, a, "POST", events, nil, `{"name":"x.y"}`)
	if rec.Code != 401 {
		t.Fatalf("enqueue without token: %d", rec.Code)
	}
	rec, _ = get(t, a, "POST", events, map[string]string{"Authorization": "Bearer whsk_service_test"}, `{"data":{}}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("enqueue without name: %d", rec.Code)
	}
	rec, out := get(t, a, "POST", events, map[string]string{"Authorization": "Bearer whsk_service_test"}, `{"name":"po.created","data":{"po":1},"dedupe_key":"po-1"}`)
	if rec.Code != 200 || out["id"] != "01EVT" || received[0]["id"] != "po-1" || received[0]["name"] != "po.created" {
		t.Fatalf("enqueue: %d %v %v", rec.Code, out, received)
	}

	// An approval request passing through the inngest proxy is recorded.
	reqBody := `{"name":"whisk/approval.requested","data":{"approval_id":"run1:request-approval","to":"owner","title":"PO 1","data":{"po":1},"run_id":"run1","function":"po-approval"}}`
	rec, _ = get(t, a, "POST", "/e/k", nil, reqBody)
	if rec.Code != 200 {
		t.Fatalf("proxy: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, a, "GET", "/v1/orgs/"+s.orgID+"/apps/"+s.appID+"/approvals?status=pending", nil, "")
	if !strings.Contains(rec.Body.String(), "run1:request-approval") {
		t.Fatalf("approval not listed: %s", rec.Body.String())
	}
	rec, _ = get(t, a, "POST", "/approvals/run1:request-approval", nil, `{"decision":"approved","note":"ok"}`)
	if rec.Code != 200 {
		t.Fatalf("decide: %d %s", rec.Code, rec.Body.String())
	}
	last := received[len(received)-1]
	data, _ := last["data"].(map[string]any)
	if last["name"] != approvalDecided || data["decision"] != "approved" || data["approval_id"] != "run1:request-approval" {
		t.Fatalf("decided event: %v", last)
	}
	if actor, _ := data["actor"].(map[string]any); actor["email"] != "ana@acme.example" {
		t.Fatalf("actor: %v", data["actor"])
	}
	rec, _ = get(t, a, "POST", "/approvals/run1:request-approval", nil, `{"decision":"rejected"}`)
	if rec.Code != 409 {
		t.Fatalf("second decision must be refused: %d", rec.Code)
	}
}

func TestAltchaRoundTrip(t *testing.T) {
	a := altcha{key: []byte("secret"), maxNumber: 500}
	c := a.newChallenge(time.Now())
	sol, ok := solve(c)
	if !ok || !a.verify(encodeSolution(sol), time.Now()) {
		t.Fatal("solution does not verify")
	}
	if a.verify(encodeSolution(sol), time.Now().Add(time.Hour)) {
		t.Fatal("expired challenge accepted")
	}
	sol.Number++
	if a.verify(encodeSolution(sol), time.Now()) {
		t.Fatal("wrong number accepted")
	}
}

func TestULID(t *testing.T) {
	a, b := newULID(), newULID()
	if len(a) != 26 || a == b || a[0] != '0' {
		t.Fatalf("ulids: %s %s", a, b)
	}
	for _, c := range a {
		if !strings.ContainsRune(crockford, c) {
			t.Fatalf("bad char %c", c)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// The stub edge passes the app only its own cookies, as the platform edge does: in the browser
// each carries the __Host- prefix, and everything else is kept from the app and deleted.
func TestStubQuarantinesCookies(t *testing.T) {
	h := http.Header{}
	h.Add("Cookie", "planted=1; __Host-sid=mine; "+sessionCookie+"=sess")
	h.Add("Cookie", "planted=2; __Host-=x")
	clear := quarantineCookies(h)
	if got := h.Get("Cookie"); got != "sid=mine; "+sessionCookie+"=sess" {
		t.Fatalf("the app saw %q", got)
	}
	if len(clear) != 1 || clear[0] != "planted=; Max-Age=0; Path=/; Secure" {
		t.Fatalf("deletions: %q", clear)
	}
	only := http.Header{"Cookie": {"planted=1"}}
	quarantineCookies(only)
	if _, ok := only["Cookie"]; ok {
		t.Fatalf("no app cookie, no Cookie header: %q", only)
	}
	set := http.Header{"Set-Cookie": {"sid=1; Path=/app; Domain=localhost", "whisk_session=x", "=nameless"}}
	bindCookies(set)
	if got := set.Values("Set-Cookie"); len(got) != 1 || got[0] != "__Host-sid=1; Path=/; Secure; Priority=High" {
		t.Fatalf("Set-Cookie: %q", got)
	}
}

func TestStubKeepsSiteDataLocal(t *testing.T) {
	for in, want := range map[string]string{
		`"cookies"`:          "",
		`"cache", "cookies"`: `"cache"`,
		`"*"`:                `"cache", "storage", "executionContexts"`,
		`"storage"`:          `"storage"`,
	} {
		h := http.Header{"Clear-Site-Data": {in}}
		keepSiteDataLocal(h)
		if got := h.Get("Clear-Site-Data"); got != want {
			t.Errorf("Clear-Site-Data %s became %q, want %q", in, got, want)
		}
	}
}

func TestStubForeignRead(t *testing.T) {
	for _, c := range []struct {
		method, site, mode string
		want               bool
	}{
		{"GET", "same-site", "cors", true},
		{"GET", "cross-site", "websocket", true},
		{"GET", "same-site", "navigate", false},
		{"GET", "same-origin", "cors", false},
		{"POST", "cross-site", "cors", false},
		{"GET", "cross-site", "", false},
	} {
		r := httptest.NewRequest(c.method, "http://localhost/", nil)
		r.Header.Set("Sec-Fetch-Site", c.site)
		r.Header.Set("Sec-Fetch-Mode", c.mode)
		if got := foreignRead(r); got != c.want {
			t.Errorf("foreignRead(%s %s %s) = %v", c.method, c.site, c.mode, got)
		}
	}
}
