package stub

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
	"unicode"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/routes"
)

// Identity is the person the stub signs in with --as.
type identity struct {
	UserID   string
	Email    string
	Name     string
	Groups   []string
	Roles    []string
	Audience string // team | customer
}

const sessionCookie = "whisk_stub_session"

// edge is the stand-in for the platform edge on an app hostname: it strips inbound identity,
// passes the app only its own cookies, classifies the route, injects identity from the stub
// session cookie, enforces CSRF and challenge, adds the security headers and proxies to the app.
type edge struct {
	stub  *stub
	proxy *httputil.ReverseProxy
}

func newEdge(s *stub) *edge {
	p := httputil.NewSingleHostReverseProxy(s.upstream)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		s.logf("upstream error: %v", err)
		writeError(w, r, http.StatusBadGateway, werrors.New("PLATFORM_UNAVAILABLE", "The app did not answer: "+err.Error(), "Check that the app is listening on PORT (8080) and that its log shows no crash.", nil))
	}
	p.ModifyResponse = func(resp *http.Response) error {
		bindCookies(resp.Header)
		keepSiteDataLocal(resp.Header)
		return nil
	}
	return &edge{stub: s, proxy: p}
}

func (e *edge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s := e.stub
	reqID := newULID()
	stripWhiskHeaders(r.Header)
	for _, c := range quarantineCookies(r.Header) {
		w.Header().Add("Set-Cookie", c)
	}
	r.Header.Set("X-Whisk-Request-Id", reqID)
	w.Header().Set("X-Whisk-Request-Id", reqID)
	securityHeaders(w.Header(), false)
	r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimit)

	path := r.URL.Path
	// As the platform's edge: a path the app might resolve to another route is refused before
	// it is classified (CADDY.md §5.1 step 1).
	if routes.Ambiguous(r.URL.EscapedPath()) {
		writeError(w, r, http.StatusBadRequest, werrors.New("PATH_AMBIGUOUS", "The request path has a . or .. segment, so the app could serve a different route than the one it names.", "Send the path already resolved, as a browser does: /a/b/../c is /a/c.", nil))
		return
	}
	switch {
	case path == "/.whisk/login":
		e.login(w, r)
		return
	case path == "/.whisk/logout":
		e.logout(w, r)
		return
	case path == "/.whisk/ready":
		w.WriteHeader(http.StatusOK)
		return
	case mediaPath(path):
		// As the platform's edge: these addresses are the platform's, never the app's.
		e.media(w, r)
		return
	}

	if s.manifest.IsService(path) {
		writeError(w, r, http.StatusUnauthorized, werrors.New("AUTH_REQUIRED", "This route only accepts platform deliveries.", "Function runs and webhook deliveries reach this route through the platform; nothing else needs to call it.", map[string]any{"route": path}))
		return
	}

	// A background request from another site's page, another app's included, reaches the app
	// without cookies, as on the platform (CADDY.md §5.1 step 7).
	if foreignRead(r) && !s.manifest.CSRFExempt(path) {
		r.Header.Del("Cookie")
	}

	signedIn := e.signedIn(r)
	public := s.manifest.IsPublic(path)
	audience := "anonymous"
	switch {
	case signedIn:
		e.inject(r)
		audience = s.identity.Audience
	case public:
		r.Header.Set("X-Whisk-Audience", "anonymous")
	default:
		if isNavigation(r) {
			// nosemgrep: go.lang.security.injection.open-redirect.open-redirect -- the local stub, never deployed
			http.Redirect(w, r, "/.whisk/login?return="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Whisk realm=%q", r.Host))
		writeError(w, r, http.StatusUnauthorized, werrors.New("AUTH_REQUIRED", "This route requires a signed-in user.", fmt.Sprintf("Navigate to /.whisk/login?return=%s to sign in, or add the route to routes.public in whisk.yaml if it is meant for everyone.", path), map[string]any{"route": path, "audience": "anonymous"}))
		return
	}

	if (signedIn || appCookies(r)) && mutating(r.Method) && !s.manifest.CSRFExempt(path) && r.Header.Get("Authorization") == "" {
		if !sameOrigin(r) {
			writeError(w, r, http.StatusForbidden, werrors.New("CSRF_REJECTED", fmt.Sprintf("A %s to %s with cookies came from another site or another app.", r.Method, path), fmt.Sprintf("Send the request from the app's own pages, or list %s under routes.csrf_off in whisk.yaml if a cross-site request is intended.", path), map[string]any{"route": path, "sec_fetch_site": r.Header.Get("Sec-Fetch-Site")}))
			return
		}
	}

	if r.Method == http.MethodPost && s.manifest.NeedsChallenge(path) {
		if !e.challengePassed(r) {
			c := s.altcha.newChallenge(time.Now())
			writeError(w, r, http.StatusForbidden, werrors.New("CHALLENGE_REQUIRED", fmt.Sprintf("POST %s needs a solved challenge.", path), "Solve the challenge in details with the Altcha widget and resend with the payload in the altcha form field or the X-Altcha header.", map[string]any{"route": path, "challenge": c}))
			return
		}
	}

	if !public {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	rec := &statusRecorder{ResponseWriter: w, status: 200}
	e.proxy.ServeHTTP(rec, r)
	s.logf("%s %s -> %d (%s) %s", r.Method, path, rec.status, audience, time.Since(start).Round(time.Millisecond))
}

func (e *edge) signedIn(r *http.Request) bool {
	if e.stub.identity == nil {
		return false
	}
	c, err := r.Cookie(sessionCookie)
	return err == nil && c.Value == e.stub.sessionValue
}

func (e *edge) inject(r *http.Request) {
	id := e.stub.identity
	h := r.Header
	h.Set("X-Whisk-Audience", id.Audience)
	h.Set("X-Whisk-User-Id", id.UserID)
	h.Set("X-Whisk-Email", id.Email)
	h.Set("X-Whisk-Name", id.Name)
	h.Set("X-Whisk-Org", e.stub.orgID)
	h.Set("X-Whisk-Session-Id", e.stub.sessionID)
	if id.Audience == "team" {
		if len(id.Groups) > 0 {
			h.Set("X-Whisk-Groups", strings.Join(id.Groups, ","))
		}
		h.Set("X-Whisk-Roles", strings.Join(id.Roles, ","))
	}
}

func (e *edge) login(w http.ResponseWriter, r *http.Request) {
	ret := safeReturn(r.URL.Query().Get("return"))
	if e.stub.identity == nil {
		writeError(w, r, http.StatusBadRequest, werrors.New("AUTH_REQUIRED", "The stub was started without --as, so nobody can sign in.", "Restart whisk-stub with --as you@example.com to sign in as that person.", nil))
		return
	}
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- the local stub serves plain HTTP on the laptop
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: e.stub.sessionValue, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 12 * 3600})
	// nosemgrep: go.lang.security.injection.open-redirect.open-redirect -- the local stub, never deployed
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

func (e *edge) logout(w http.ResponseWriter, r *http.Request) {
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- the local stub serves plain HTTP on the laptop
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	// nosemgrep: go.lang.security.injection.open-redirect.open-redirect -- the local stub, never deployed
	http.Redirect(w, r, safeReturn(r.URL.Query().Get("return")), http.StatusSeeOther)
}

func (e *edge) challengePassed(r *http.Request) bool {
	payload := r.Header.Get("X-Altcha")
	if payload == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err == nil {
			payload = r.PostForm.Get("altcha")
		}
	}
	return payload != "" && e.stub.altcha.verify(payload, time.Now())
}

// safeReturn keeps return targets on this hostname, as the platform does. A browser reads "/\"
// as "//" and drops tabs and newlines, so a backslash after the first slash and any control
// character are refused too.
func safeReturn(ret string) string {
	if ret == "" || !strings.HasPrefix(ret, "/") || strings.HasPrefix(ret, "//") || strings.HasPrefix(ret, `/\`) || strings.ContainsFunc(ret, unicode.IsControl) {
		return "/"
	}
	return ret
}

func stripWhiskHeaders(h http.Header) {
	for name := range h {
		if strings.HasPrefix(strings.ToLower(name), "x-whisk-") {
			h.Del(name)
		}
	}
}

// hostPrefix binds a cookie to the host that set it. As on the platform edge, every app cookie
// carries it in the browser and the app sees the name without it (CADDY.md §5.3).
const hostPrefix = "__Host-"

// bindCookies rewrites each Set-Cookie the app returns into a __Host- cookie: the name gains
// the prefix, Domain is dropped, the Path becomes / and Secure is added. A cookie without a name
// and one that would be the platform session are dropped.
// keepSiteDataLocal drops the "cookies" directive from Clear-Site-Data, and turns "*" into the
// directives that stay with the app's origin, as the platform's edge does: clearing cookies would
// clear every app's on the apps domain (CADDY.md §5.3).
func keepSiteDataLocal(h http.Header) {
	values := h.Values("Clear-Site-Data")
	if len(values) == 0 {
		return
	}
	h.Del("Clear-Site-Data")
	var kept []string
	seen := map[string]bool{}
	for _, v := range values {
		for _, d := range strings.Split(v, ",") {
			d = strings.TrimSpace(d)
			adds := []string{d}
			switch strings.ToLower(strings.Trim(d, `"`)) {
			case "", "cookies":
				adds = nil
			case "*":
				adds = []string{`"cache"`, `"storage"`, `"executionContexts"`}
			}
			for _, a := range adds {
				if !seen[a] {
					seen[a] = true
					kept = append(kept, a)
				}
			}
		}
	}
	if len(kept) > 0 {
		h.Set("Clear-Site-Data", strings.Join(kept, ", "))
	}
}

func bindCookies(h http.Header) {
	cookies := h.Values("Set-Cookie")
	if len(cookies) == 0 {
		return
	}
	h.Del("Set-Cookie")
	for _, c := range cookies {
		parts := strings.Split(c, ";")
		name, value, ok := strings.Cut(parts[0], "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.EqualFold(hostPrefix+name, "__Host-whisk_session") || strings.EqualFold(name, "__Host-whisk_session") {
			continue
		}
		kept := []string{hostPrefix + name + "=" + value}
		secure, priority := false, false
		for _, p := range parts[1:] {
			an, _, _ := strings.Cut(p, "=")
			switch strings.ToLower(strings.TrimSpace(an)) {
			case "domain", "path":
				continue
			case "secure":
				secure = true
			case "priority":
				priority = true
			}
			kept = append(kept, p)
		}
		out := strings.Join(kept, ";") + "; Path=/"
		if !secure {
			out += "; Secure"
		}
		if !priority {
			out += "; Priority=High"
		}
		h.Add("Set-Cookie", out)
	}
}

// quarantineCookies passes the app only its own cookies, without the prefix, and the stub's
// session, and returns the Set-Cookie values that delete every other cookie the browser sent.
func quarantineCookies(h http.Header) []string {
	values := h.Values("Cookie")
	if len(values) == 0 {
		return nil
	}
	var kept, clear []string
	seen := map[string]bool{}
	for _, v := range values {
		for _, pair := range strings.Split(v, ";") {
			pair = strings.TrimSpace(pair)
			name, value, _ := strings.Cut(pair, "=")
			name = strings.TrimSpace(name)
			switch {
			case pair == "":
			case name == sessionCookie:
				kept = append(kept, pair)
			case strings.HasPrefix(name, hostPrefix):
				if len(name) > len(hostPrefix) {
					kept = append(kept, name[len(hostPrefix):]+"="+value)
				}
			case cookieToken(name) && !seen[name]:
				seen[name] = true
				clear = append(clear, name+"=; Max-Age=0; Path=/; Secure")
			}
		}
	}
	h.Del("Cookie")
	if len(kept) > 0 {
		h.Set("Cookie", strings.Join(kept, "; "))
	}
	return clear
}

// foreignRead is a request that changes nothing, is not a navigation, and came from a page on
// another site.
func foreignRead(r *http.Request) bool {
	if mutating(r.Method) {
		return false
	}
	site, mode := r.Header.Get("Sec-Fetch-Site"), r.Header.Get("Sec-Fetch-Mode")
	return (site == "same-site" || site == "cross-site") && mode != "" && mode != "navigate"
}

// appCookies reports whether the request carries any of the app's own cookies.
func appCookies(r *http.Request) bool {
	for _, c := range r.Cookies() {
		if c.Name != sessionCookie {
			return true
		}
	}
	return false
}

// cookieToken is a cookie name a Set-Cookie can carry back.
func cookieToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c <= ' ' || c >= 0x7f || strings.IndexByte("()<>@,;:\\\"/[]?={}", c) >= 0 {
			return false
		}
	}
	return true
}

func securityHeaders(h http.Header, preview bool) {
	h.Set("Strict-Transport-Security", "max-age=31536000")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Origin-Agent-Cluster", "?1")
	h.Set("Server", "whisk")
	if preview {
		h.Set("X-Robots-Tag", "noindex, nofollow")
	}
}

func mutating(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	return false
}

func isNavigation(r *http.Request) bool {
	return r.Header.Get("Sec-Fetch-Mode") == "navigate" || (r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html"))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// writeError renders the standard error body: JSON for API clients, a small HTML page for
// browser navigations, as the platform does.
func writeError(w http.ResponseWriter, r *http.Request, status int, body werrors.Body) {
	if r != nil && isNavigation(r) && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprintf(w, "<!doctype html><title>%s</title><main style=\"font-family:system-ui;max-width:40rem;margin:4rem auto\"><h1>%s</h1><p>%s</p><p>%s</p><p><small>%s</small></p></main>",
			html.EscapeString(body.Error.Code), html.EscapeString(body.Error.Code), html.EscapeString(body.Error.Message), html.EscapeString(body.Error.Fix), html.EscapeString(body.Error.Docs))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
