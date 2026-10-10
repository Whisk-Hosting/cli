// Package connect is a connection: an outside system an app reaches through Whisk's broker
// without ever holding its credential (CONTRACT.md §3.1, CONTROL-PLANE.md §6.8). The manifest
// declares where the system is, how to sign a request to it (a recipe of templates) and the
// operations the app wants; a person grants them. This package parses and checks connections,
// decides whether a grant covers what a manifest asks for, matches a call to an operation and
// renders the credential onto a call. It is pure apart from the clock and randomness Env carries.
package connect

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/whisk-run/contract/routes"
)

var (
	// Name is a connection's name; WHISK_CONNECTION_<NAME>_URL carries its address.
	Name = regexp.MustCompile(`^[a-z][a-z0-9_]{0,29}$`)
	// SecretName is a secret's name, as under secrets in the manifest.
	SecretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
	// OperationName is the plain words a person reads when granting ("read stock").
	OperationName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ,'()/&-]{0,59}$`)
	headerName    = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_|~-]{1,64}$`)
)

// Methods an operation may name; * is any of them.
var Methods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "*"}

// Headers a recipe may never set: the broker owns framing and the destination.
var forbiddenHeaders = map[string]bool{"Host": true, "Content-Length": true, "Transfer-Encoding": true, "Connection": true, "Upgrade": true, "Te": true, "Trailer": true, "Proxy-Authorization": true, "Whisk-Service-Token": true}

// Connection is one entry under connections in whisk.yaml.
type Connection struct {
	URL        string      `json:"url"`
	Auth       Auth        `json:"auth"`
	Operations []Operation `json:"operations"`
}

// Auth is the recipe: where credentials go on each call, and an optional token step.
type Auth struct {
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	Token   *TokenStep        `json:"token,omitempty"`
}

// TokenStep fetches a short-lived token the recipe then places as {token}. The broker keeps it
// until it expires (or Lifetime seconds when the answer says nothing) and fetches it again
// after the outside system refuses a call with 401.
type TokenStep struct {
	URL          string            `json:"url"`
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Form         map[string]string `json:"form,omitempty"`
	JSON         map[string]string `json:"json,omitempty"`
	Field        string            `json:"field,omitempty"`
	ExpiresField string            `json:"expires_field,omitempty"`
	Lifetime     int               `json:"lifetime,omitempty"`
	Revoke       *RevokeStep       `json:"revoke,omitempty"`
}

// RevokeStep is where the broker asks the token's issuer to cancel a token it held (RFC 7009)
// when the grant is revoked: a POST of a form to a URL on the token step's own host. Its
// templates may read secrets and {token}; the form defaults to token={token} and
// token_type_hint=access_token.
type RevokeStep struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Form    map[string]string `json:"form,omitempty"`
}

// Operation is one thing the app may do: a method and a path glob under the connection's URL.
type Operation struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

// Token step defaults.
const (
	DefaultTokenField   = "access_token"
	DefaultExpiresField = "expires_in"
	DefaultLifetime     = 300
	MaxLifetime         = 86400
	MaxOperations       = 100
	MaxTemplate         = 2048
)

// Problem is one finding about a connection, at a JSON pointer below the connection.
type Problem struct {
	Path    string
	Message string
}

// WithDefaults fills the token step's defaults and upper-cases methods.
func (c Connection) WithDefaults() Connection {
	if c.Auth.Token != nil {
		t := *c.Auth.Token
		if t.Method == "" {
			t.Method = http.MethodPost
		}
		t.Method = strings.ToUpper(t.Method)
		if t.Field == "" {
			t.Field = DefaultTokenField
		}
		if t.ExpiresField == "" {
			t.ExpiresField = DefaultExpiresField
		}
		if t.Lifetime == 0 {
			t.Lifetime = DefaultLifetime
		}
		if t.Revoke != nil && len(t.Revoke.Form) == 0 {
			r := *t.Revoke
			r.Form = map[string]string{"token": "{token}", "token_type_hint": "access_token"}
			t.Revoke = &r
		}
		c.Auth.Token = &t
	}
	ops := make([]Operation, len(c.Operations))
	for i, o := range c.Operations {
		o.Method = strings.ToUpper(o.Method)
		ops[i] = o
	}
	c.Operations = ops
	return c
}

// Check returns every problem with a connection (with defaults applied) and the secret names
// its recipe reads. A connection with no problems compiles.
func Check(c Connection) ([]Problem, []string) {
	var ps []Problem
	add := func(path, format string, a ...any) { ps = append(ps, Problem{path, fmt.Sprintf(format, a...)}) }
	if msg := checkURL(c.URL); msg != "" {
		add("/url", "%s", msg)
	}
	var secrets []string
	tmpl := func(path, src string, ctx Context) {
		if len(src) > MaxTemplate {
			add(path, "longer than %d characters", MaxTemplate)
			return
		}
		_, s, err := ParseTemplate(src, ctx)
		if err != nil {
			add(path, "%s", err.Error())
		}
		secrets = append(secrets, s...)
	}
	call := Context{Request: true, Token: c.Auth.Token != nil}
	if c.Auth.Token != nil && len(c.Auth.Headers) == 0 && len(c.Auth.Query) == 0 {
		add("/auth", "the token step's token is never placed; use {token} in a header or query value")
	}
	for _, k := range sortedKeys(c.Auth.Headers) {
		checkHeader("/auth/headers/"+k, k, add)
		tmpl("/auth/headers/"+k, c.Auth.Headers[k], call)
	}
	for _, k := range sortedKeys(c.Auth.Query) {
		if k == "" || strings.ContainsAny(k, "&=#") {
			add("/auth/query/"+k, "a query name may not be empty or contain &, = or #")
		}
		tmpl("/auth/query/"+k, c.Auth.Query[k], call)
	}
	if t := c.Auth.Token; t != nil {
		step := Context{}
		if msg := checkURL(t.URL); msg != "" {
			add("/auth/token/url", "%s", msg)
		}
		if t.Method != http.MethodPost && t.Method != http.MethodGet {
			add("/auth/token/method", "GET or POST")
		}
		if len(t.Form) > 0 && len(t.JSON) > 0 {
			add("/auth/token", "send form or json, not both")
		}
		if t.Method == http.MethodGet && (len(t.Form) > 0 || len(t.JSON) > 0) {
			add("/auth/token", "a GET sends no body; use POST")
		}
		for _, k := range sortedKeys(t.Headers) {
			checkHeader("/auth/token/headers/"+k, k, add)
			tmpl("/auth/token/headers/"+k, t.Headers[k], step)
		}
		for _, k := range sortedKeys(t.Form) {
			tmpl("/auth/token/form/"+k, t.Form[k], step)
		}
		for _, k := range sortedKeys(t.JSON) {
			tmpl("/auth/token/json/"+k, t.JSON[k], step)
		}
		if t.Lifetime < 1 || t.Lifetime > MaxLifetime {
			add("/auth/token/lifetime", "between 1 and %d seconds", MaxLifetime)
		}
		if r := t.Revoke; r != nil {
			revoke := Context{Token: true}
			if msg := checkURL(r.URL); msg != "" {
				add("/auth/token/revoke/url", "%s", msg)
			} else if hostOf(r.URL) != hostOf(t.URL) {
				add("/auth/token/revoke/url", "on the token step's own host, %s", hostOf(t.URL))
			}
			for _, k := range sortedKeys(r.Headers) {
				checkHeader("/auth/token/revoke/headers/"+k, k, add)
				tmpl("/auth/token/revoke/headers/"+k, r.Headers[k], revoke)
			}
			for _, k := range sortedKeys(r.Form) {
				tmpl("/auth/token/revoke/form/"+k, r.Form[k], revoke)
			}
		}
	}
	if len(c.Operations) == 0 {
		add("/operations", "name at least one operation the app needs")
	}
	if len(c.Operations) > MaxOperations {
		add("/operations", "at most %d operations", MaxOperations)
	}
	seen := map[string]int{}
	for i, o := range c.Operations {
		p := fmt.Sprintf("/operations/%d", i)
		if !OperationName.MatchString(o.Name) {
			add(p+"/name", "plain words a person can read, up to 60 characters")
		}
		if j, dup := seen[strings.ToLower(o.Name)]; dup {
			add(p+"/name", "duplicate operation name %s (also /operations/%d)", o.Name, j)
		}
		seen[strings.ToLower(o.Name)] = i
		if !contains(Methods, o.Method) {
			add(p+"/method", "one of %s", strings.Join(Methods, ", "))
		}
		if !strings.HasPrefix(o.Path, "/") || strings.ContainsAny(o.Path, "?# ") || routes.Ambiguous(o.Path) {
			add(p+"/path", "an absolute path glob without query, spaces or dot segments")
		}
	}
	return ps, uniq(secrets)
}

func checkHeader(path, k string, add func(string, string, ...any)) {
	switch {
	case !headerName.MatchString(k):
		add(path, "%q is not a header name", k)
	case forbiddenHeaders[textproto.CanonicalMIMEHeaderKey(k)]:
		add(path, "the broker sets %s itself", k)
	case strings.HasPrefix(strings.ToLower(k), "x-whisk-"):
		add(path, "X-Whisk- headers are the platform's")
	}
}

func checkURL(raw string) string {
	u, err := url.Parse(raw)
	switch {
	case err != nil || u.Host == "" || !u.IsAbs():
		return "an absolute URL such as https://api.example.com/v1"
	case u.Scheme != "https" && u.Scheme != "http":
		return "https (http only reaches test systems)"
	case u.User != nil:
		return "no user or password in the URL; put credentials in the recipe"
	case u.RawQuery != "" || u.Fragment != "":
		return "no query or fragment; put fixed query values under auth.query"
	case routes.Ambiguous(u.EscapedPath()):
		return "no dot segments in the path"
	}
	return ""
}

// Secrets is the sorted list of secret names a connection's recipe reads.
func Secrets(c Connection) []string {
	_, s := Check(c)
	return s
}

// Canonical is the connection as stable JSON: what a person granted is compared byte for byte.
func Canonical(c Connection) string {
	c = c.WithDefaults()
	ops := append([]Operation(nil), c.Operations...)
	sort.Slice(ops, func(i, j int) bool { return opKey(ops[i]) < opKey(ops[j]) })
	c.Operations = ops
	b, _ := json.Marshal(c)
	return string(b)
}

// Hash is the SHA-256 of the canonical connection, in hex: the version a person grants.
func Hash(c Connection) string {
	sum := sha256.Sum256([]byte(Canonical(c)))
	return hex.EncodeToString(sum[:])
}

// Gaps says what a manifest's connection asks for beyond a grant: whether its address or
// recipe differs, and which operations are new. A grant covers the connection when both are
// empty. Fewer operations than granted never needs a person.
func Gaps(granted *Connection, wanted Connection) (recipeChanged bool, newOps []Operation) {
	wanted = wanted.WithDefaults()
	if granted == nil {
		return true, wanted.Operations
	}
	g := granted.WithDefaults()
	recipeChanged = g.URL != wanted.URL || !sameAuth(g.Auth, wanted.Auth)
	have := map[string]bool{}
	for _, o := range g.Operations {
		have[opKey(o)] = true
	}
	for _, o := range wanted.Operations {
		if !have[opKey(o)] {
			newOps = append(newOps, o)
		}
	}
	return recipeChanged, newOps
}

// Covers reports whether a grant covers a manifest's connection.
func Covers(granted *Connection, wanted Connection) bool {
	changed, ops := Gaps(granted, wanted)
	return !changed && len(ops) == 0
}

// Effective is what a running deploy may do: the operations both its manifest and the grant
// name, provided the address and recipe are the ones granted. It answers nil when nothing is.
func Effective(granted *Connection, running Connection) *Connection {
	if granted == nil {
		return nil
	}
	g, r := granted.WithDefaults(), running.WithDefaults()
	if g.URL != r.URL || !sameAuth(g.Auth, r.Auth) {
		return nil
	}
	have := map[string]bool{}
	for _, o := range g.Operations {
		have[opKey(o)] = true
	}
	var ops []Operation
	for _, o := range r.Operations {
		if have[opKey(o)] {
			ops = append(ops, o)
		}
	}
	if len(ops) == 0 {
		return nil
	}
	r.Operations = ops
	return &r
}

func sameAuth(a, b Auth) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func opKey(o Operation) string { return o.Method + " " + o.Path + " " + o.Name }

// Match finds the first operation a call is allowed under. path is the call's escaped path
// below the connection's own path, starting with "/". A path that could mean something else
// once decoded (dot segments, encoded slashes) matches nothing.
func Match(ops []Operation, method, path string) (Operation, bool) {
	if routes.Ambiguous(path) || strings.Contains(strings.ToLower(path), "%2f") || strings.Contains(strings.ToLower(path), "%5c") {
		return Operation{}, false
	}
	for _, o := range ops {
		if (o.Method == "*" || o.Method == method || o.Method == "GET" && method == "HEAD") && routes.Match(o.Path, path) {
			return o, true
		}
	}
	return Operation{}, false
}

// Placed is what a recipe adds to one call.
type Placed struct {
	Headers http.Header
	Query   url.Values
}

// Apply renders a connection's recipe for one call. token is the current token when the
// connection has a token step.
func Apply(c Connection, env Env) (Placed, error) {
	ctx := Context{Request: true, Token: c.Auth.Token != nil}
	out := Placed{Headers: http.Header{}, Query: url.Values{}}
	for _, k := range sortedKeys(c.Auth.Headers) {
		v, err := render(c.Auth.Headers[k], ctx, env)
		if err != nil {
			return Placed{}, err
		}
		out.Headers.Set(k, v)
	}
	for _, k := range sortedKeys(c.Auth.Query) {
		v, err := render(c.Auth.Query[k], ctx, env)
		if err != nil {
			return Placed{}, err
		}
		out.Query.Set(k, v)
	}
	return out, nil
}

// TokenRequest is the token step's request, rendered.
type TokenRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// RenderToken renders a connection's token step.
func RenderToken(t TokenStep, env Env) (TokenRequest, error) {
	env.Request, env.Token = nil, ""
	ctx := Context{}
	out := TokenRequest{Method: t.Method, URL: t.URL, Headers: http.Header{}}
	for _, k := range sortedKeys(t.Headers) {
		v, err := render(t.Headers[k], ctx, env)
		if err != nil {
			return TokenRequest{}, err
		}
		out.Headers.Set(k, v)
	}
	switch {
	case len(t.Form) > 0:
		f := url.Values{}
		for _, k := range sortedKeys(t.Form) {
			v, err := render(t.Form[k], ctx, env)
			if err != nil {
				return TokenRequest{}, err
			}
			f.Set(k, v)
		}
		out.Body = []byte(f.Encode())
		out.Headers.Set("Content-Type", "application/x-www-form-urlencoded")
	case len(t.JSON) > 0:
		m := map[string]string{}
		for _, k := range sortedKeys(t.JSON) {
			v, err := render(t.JSON[k], ctx, env)
			if err != nil {
				return TokenRequest{}, err
			}
			m[k] = v
		}
		out.Body, _ = json.Marshal(m)
		out.Headers.Set("Content-Type", "application/json")
	}
	if out.Headers.Get("Accept") == "" {
		out.Headers.Set("Accept", "application/json")
	}
	return out, nil
}

// RenderRevoke renders a token step's revocation of one token it issued.
func RenderRevoke(r RevokeStep, env Env, token string) (TokenRequest, error) {
	env.Request, env.Token = nil, token
	ctx := Context{Token: true}
	out := TokenRequest{Method: http.MethodPost, URL: r.URL, Headers: http.Header{}}
	for _, k := range sortedKeys(r.Headers) {
		v, err := render(r.Headers[k], ctx, env)
		if err != nil {
			return TokenRequest{}, err
		}
		out.Headers.Set(k, v)
	}
	f := url.Values{}
	for _, k := range sortedKeys(r.Form) {
		v, err := render(r.Form[k], ctx, env)
		if err != nil {
			return TokenRequest{}, err
		}
		f.Set(k, v)
	}
	out.Body = []byte(f.Encode())
	out.Headers.Set("Content-Type", "application/x-www-form-urlencoded")
	return out, nil
}

// ErrNoToken is a token answer without the named field.
var ErrNoToken = errors.New("the token answer has no token at the named field")

// ReadToken takes the token and its lifetime in seconds from a token step's JSON answer.
// Field and ExpiresField are dotted paths ("data.token").
func ReadToken(t TokenStep, body []byte) (string, int, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", 0, errors.New("the token answer is not JSON")
	}
	tok, ok := dig(doc, t.Field).(string)
	if !ok || tok == "" {
		return "", 0, ErrNoToken
	}
	life := t.Lifetime
	switch e := dig(doc, t.ExpiresField).(type) {
	case float64:
		life = int(e)
	case string:
		var n int
		if _, err := fmt.Sscanf(e, "%d", &n); err == nil {
			life = n
		}
	}
	if life < 1 {
		life = t.Lifetime
	}
	if life > MaxLifetime {
		life = MaxLifetime
	}
	return tok, life, nil
}

func dig(doc any, path string) any {
	for _, k := range strings.Split(path, ".") {
		m, ok := doc.(map[string]any)
		if !ok {
			return nil
		}
		doc = m[k]
	}
	return doc
}

func render(src string, ctx Context, env Env) (string, error) {
	t, _, err := ParseTemplate(src, ctx)
	if err != nil {
		return "", err
	}
	v, err := t.Eval(env)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(v, "\r\n") {
		return "", errors.New("a rendered value contains a line break")
	}
	return v, nil
}

// UsesBody reports whether the recipe signs the request body.
func UsesBody(c Connection) bool {
	for _, v := range c.Auth.Headers {
		if strings.Contains(v, "request.body") {
			return true
		}
	}
	for _, v := range c.Auth.Query {
		if strings.Contains(v, "request.body") {
			return true
		}
	}
	return false
}

// EnvName is the variable carrying a connection's address inside the app.
func EnvName(name string) string { return "WHISK_CONNECTION_" + strings.ToUpper(name) + "_URL" }

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
