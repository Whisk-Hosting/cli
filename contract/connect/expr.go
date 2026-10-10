package connect

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A template is a string with {expression} placeholders (CONTRACT.md §3.1). Text outside the
// braces is literal; {{ and }} stand for a literal brace. An expression is a reference
// (secret.NAME, request.path, time.unix, nonce, token), a 'quoted string', a whole number, or a
// function call f(expression, ...). Templates are parsed once when the manifest is validated
// and evaluated by the broker for each call; evaluation never touches the network.

// Template is a parsed template.
type Template struct {
	src   string
	parts []node
}

// String is the template as written.
func (t Template) String() string { return t.src }

// Context says which references a template may use: the request is absent while a token is
// fetched, and token exists only when the connection has a token step.
type Context struct {
	Request bool
	Token   bool
}

// Env is everything an evaluation may read. Now, Nonce and UUID are fixed for one call so a
// timestamp header and the signature over it agree.
type Env struct {
	Secrets map[string]string
	// Vendor is the vendor app's client_id and client_secret, when the recipe names one.
	Vendor  map[string]string
	Request *Request
	Token   string
	Now     time.Time
	Nonce   string
	UUID    string
}

// Request is the app's call as it will be sent upstream, before the recipe adds anything:
// Method upper case, URL the full upstream URL, Path its escaped path, Query its raw query.
type Request struct {
	Method string
	URL    string
	Host   string
	Path   string
	Query  string
	Body   []byte
}

// NewEnv fills Now, Nonce and UUID for one call.
func NewEnv(secrets map[string]string, req *Request, token string, now time.Time) Env {
	var b [32]byte
	_, _ = rand.Read(b[:])
	u := b[16:]
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	id := hex.EncodeToString(u)
	return Env{
		Secrets: secrets, Request: req, Token: token, Now: now.UTC(),
		Nonce: hex.EncodeToString(b[:16]),
		UUID:  id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:],
	}
}

type value struct {
	b      []byte
	num    int64
	isNum  bool
	isJSON bool
}

func str(s string) value     { return value{b: []byte(s)} }
func (v value) text() string { return string(v.bytes()) }
func (v value) bytes() []byte {
	if v.isNum {
		return []byte(strconv.FormatInt(v.num, 10))
	}
	return v.b
}

type node interface {
	eval(Env) (value, error)
}

type lit struct{ v value }

func (l lit) eval(Env) (value, error) { return l.v, nil }

type ref struct{ path []string }

type call struct {
	fn   string
	args []node
}

// ParseTemplate parses src, checking every reference and function against ctx. It returns the
// secret names the template reads.
func ParseTemplate(src string, ctx Context) (Template, []string, error) {
	var parts []node
	var text strings.Builder
	var secrets []string
	flush := func() {
		if text.Len() > 0 {
			parts = append(parts, lit{str(text.String())})
			text.Reset()
		}
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '{' && i+1 < len(src) && src[i+1] == '{':
			text.WriteByte('{')
			i++
		case c == '}' && i+1 < len(src) && src[i+1] == '}':
			text.WriteByte('}')
			i++
		case c == '}':
			return Template{}, nil, fmt.Errorf("unmatched } at %d; write }} for a literal brace", i)
		case c == '{':
			end := closing(src, i+1)
			if end < 0 {
				return Template{}, nil, fmt.Errorf("unclosed { at %d", i)
			}
			p := &parser{src: src[i+1 : end], ctx: ctx}
			n, err := p.expr()
			if err == nil {
				p.space()
				if p.pos < len(p.src) {
					err = fmt.Errorf("unexpected %q", p.src[p.pos:])
				}
			}
			if err != nil {
				return Template{}, nil, fmt.Errorf("in {%s}: %w", src[i+1:end], err)
			}
			flush()
			parts = append(parts, n)
			secrets = append(secrets, p.secrets...)
			i = end
		default:
			text.WriteByte(c)
		}
	}
	flush()
	if err := checkJWTs(parts, secrets); err != nil {
		return Template{}, nil, err
	}
	return Template{src: src, parts: parts}, uniq(secrets), nil
}

// closing finds the } that ends an expression starting at i, skipping quoted strings.
func closing(s string, i int) int {
	for ; i < len(s); i++ {
		switch s[i] {
		case '\'':
			for i++; i < len(s) && s[i] != '\''; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '}':
			return i
		}
	}
	return -1
}

// readsVendor reports whether the template reads the vendor app anywhere.
func (t Template) readsVendor() bool {
	var reads func(n node) bool
	reads = func(n node) bool {
		switch n := n.(type) {
		case ref:
			return n.path[0] == "vendor"
		case call:
			for _, a := range n.args {
				if reads(a) {
					return true
				}
			}
		}
		return false
	}
	for _, p := range t.parts {
		if reads(p) {
			return true
		}
	}
	return false
}

// Eval renders the template in env.
func (t Template) Eval(env Env) (string, error) {
	var b strings.Builder
	for _, p := range t.parts {
		v, err := p.eval(env)
		if err != nil {
			return "", err
		}
		b.Write(v.bytes())
	}
	return b.String(), nil
}

// MarshalJSON keeps a template's source, so a parsed connection round-trips.
func (t Template) MarshalJSON() ([]byte, error) { return json.Marshal(t.src) }

type parser struct {
	src     string
	pos     int
	ctx     Context
	secrets []string
}

func (p *parser) space() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) expr() (node, error) {
	p.space()
	if p.pos >= len(p.src) {
		return nil, errors.New("empty expression")
	}
	c := p.src[p.pos]
	switch {
	case c == '\'':
		return p.quoted()
	case c >= '0' && c <= '9' || c == '-':
		start := p.pos
		p.pos++
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
		n, err := strconv.ParseInt(p.src[start:p.pos], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", p.src[start:p.pos])
		}
		return lit{value{num: n, isNum: true}}, nil
	case isIdent(c):
		name := p.ident()
		p.space()
		if p.pos < len(p.src) && p.src[p.pos] == '(' {
			return p.call(name)
		}
		return p.reference(name)
	}
	return nil, fmt.Errorf("unexpected %q", string(c))
}

func isIdent(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' }

func (p *parser) ident() string {
	start := p.pos
	for p.pos < len(p.src) && (isIdent(p.src[p.pos]) || p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *parser) quoted() (node, error) {
	var b strings.Builder
	for p.pos++; p.pos < len(p.src); p.pos++ {
		c := p.src[p.pos]
		switch {
		case c == '\\' && p.pos+1 < len(p.src):
			p.pos++
			switch p.src[p.pos] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(p.src[p.pos])
			}
		case c == '\'':
			p.pos++
			return lit{str(b.String())}, nil
		default:
			b.WriteByte(c)
		}
	}
	return nil, errors.New("unclosed quote")
}

// References a template may use, and whether each needs the request or a token.
var timeFormats = map[string]func(time.Time) string{
	"unix":    func(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) },
	"unix_ms": func(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) },
	"rfc3339": func(t time.Time) string { return t.Format(time.RFC3339) },
	"http":    func(t time.Time) string { return t.Format(http1123) },
	"amz":     func(t time.Time) string { return t.Format("20060102T150405Z") },
	"date":    func(t time.Time) string { return t.Format("20060102") },
}

const http1123 = "Mon, 02 Jan 2006 15:04:05 GMT"

var requestFields = map[string]func(*Request) []byte{
	"method": func(r *Request) []byte { return []byte(r.Method) },
	"url":    func(r *Request) []byte { return []byte(r.URL) },
	"host":   func(r *Request) []byte { return []byte(r.Host) },
	"path":   func(r *Request) []byte { return []byte(r.Path) },
	"query":  func(r *Request) []byte { return []byte(r.Query) },
	"body":   func(r *Request) []byte { return r.Body },
}

// UsesBody reports whether a template reads request.body, so the broker buffers the body only
// when a recipe signs it.
func (t Template) UsesBody() bool { return strings.Contains(t.src, "request.body") }

func (p *parser) reference(name string) (node, error) {
	parts := strings.Split(name, ".")
	bad := func(why string) (node, error) { return nil, fmt.Errorf("%s: %s", name, why) }
	switch parts[0] {
	case "secret":
		if len(parts) != 2 || !SecretName.MatchString(parts[1]) {
			return bad("write secret.NAME with NAME in upper snake case")
		}
		p.secrets = append(p.secrets, parts[1])
	case "request":
		if !p.ctx.Request {
			return bad("the request is not available while a token is fetched")
		}
		if len(parts) != 2 || requestFields[parts[1]] == nil {
			return bad("one of request.method, request.url, request.host, request.path, request.query, request.body")
		}
	case "time":
		if len(parts) != 2 || timeFormats[parts[1]] == nil {
			return bad("one of time.unix, time.unix_ms, time.rfc3339, time.http, time.amz, time.date")
		}
	case "nonce", "uuid":
		if len(parts) != 1 {
			return bad("takes no field")
		}
	case "vendor":
		if len(parts) != 2 || parts[1] != "client_id" && parts[1] != "client_secret" {
			return bad("one of vendor.client_id, vendor.client_secret")
		}
	case "token":
		if !p.ctx.Token {
			return bad("token needs a token step under auth.token, and is not available inside it")
		}
		if len(parts) != 1 {
			return bad("takes no field")
		}
	default:
		return bad("unknown reference; use secret, request, time, nonce, uuid or token")
	}
	return ref{parts}, nil
}

func (r ref) eval(env Env) (value, error) {
	switch r.path[0] {
	case "secret":
		v, ok := env.Secrets[r.path[1]]
		if !ok {
			return value{}, &MissingSecretError{Name: r.path[1]}
		}
		return str(v), nil
	case "request":
		if env.Request == nil {
			return value{}, errors.New("no request")
		}
		return value{b: requestFields[r.path[1]](env.Request)}, nil
	case "time":
		if r.path[1] == "unix" {
			return value{num: env.Now.Unix(), isNum: true}, nil
		}
		if r.path[1] == "unix_ms" {
			return value{num: env.Now.UnixMilli(), isNum: true}, nil
		}
		return str(timeFormats[r.path[1]](env.Now)), nil
	case "nonce":
		return str(env.Nonce), nil
	case "uuid":
		return str(env.UUID), nil
	case "token":
		return str(env.Token), nil
	case "vendor":
		v, ok := env.Vendor[r.path[1]]
		if !ok || v == "" {
			return value{}, &MissingSecretError{Name: "vendor." + r.path[1]}
		}
		return str(v), nil
	}
	return value{}, errors.New("unknown reference")
}

// MissingSecretError is an evaluation that needed a secret with no value.
type MissingSecretError struct{ Name string }

func (e *MissingSecretError) Error() string { return e.Name + " has no value" }

func (p *parser) call(name string) (node, error) {
	f, ok := functions[name]
	if !ok {
		return nil, fmt.Errorf("unknown function %s; see CONTRACT.md §3.1 for the list", name)
	}
	p.pos++ // (
	var args []node
	p.space()
	if p.pos < len(p.src) && p.src[p.pos] == ')' {
		p.pos++
	} else {
		for {
			a, err := p.expr()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			p.space()
			if p.pos >= len(p.src) {
				return nil, fmt.Errorf("%s( is not closed", name)
			}
			if p.src[p.pos] == ',' {
				p.pos++
				continue
			}
			if p.src[p.pos] == ')' {
				p.pos++
				break
			}
			return nil, fmt.Errorf("expected , or ) in %s(", name)
		}
	}
	if len(args) < f.min || f.max >= 0 && len(args) > f.max {
		return nil, fmt.Errorf("%s takes %s", name, f.arity())
	}
	if f.check != nil {
		if err := f.check(args); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	if name == "jwt" {
		for _, a := range args[2:] {
			if reads(a, "request") {
				return nil, errors.New("jwt: its claims and key id are the recipe's own; they may read secrets, the time, nonce and uuid, never the request the app sends")
			}
		}
	}
	return call{name, args}, nil
}

// MaxJWTLifetime is the furthest a JWT the broker signs may expire after it is signed.
const MaxJWTLifetime = 3600

// reads reports whether n reads a reference of that kind anywhere inside it.
func reads(n node, kind string) bool {
	switch v := n.(type) {
	case ref:
		return v.path[0] == kind
	case call:
		for _, a := range v.args {
			if reads(a, kind) {
				return true
			}
		}
	}
	return false
}

// jwtExpiry holds a JWT's claims to a numeric exp after now and no more than MaxJWTLifetime
// seconds ahead, so no recipe signs a long-lived credential. Pure.
func jwtExpiry(claims []byte, now time.Time) error {
	var c struct {
		Exp *json.Number `json:"exp"`
	}
	d := json.NewDecoder(strings.NewReader(string(claims)))
	d.UseNumber()
	if err := d.Decode(&c); err != nil || c.Exp == nil {
		return fmt.Errorf("jwt: its claims need exp, at most %d seconds after time.unix", MaxJWTLifetime)
	}
	exp, err := c.Exp.Int64()
	if err != nil || exp <= now.Unix() || exp > now.Unix()+MaxJWTLifetime {
		return fmt.Errorf("jwt: exp must be after time.unix and at most %d seconds after it", MaxJWTLifetime)
	}
	return nil
}

// checkJWTs renders the claims of every jwt call in the parts with stand-in secrets at a fixed
// time and holds them to jwtExpiry, so a recipe that would sign a long-lived token fails its
// check rather than its first call.
func checkJWTs(parts []node, secrets []string) error {
	stand := map[string]string{}
	for _, s := range secrets {
		stand[s] = "x"
	}
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	env := Env{Secrets: stand, Request: &Request{}, Token: "x", Now: now, Nonce: "x", UUID: "x"}
	var walk func(node) error
	walk = func(n node) error {
		c, ok := n.(call)
		if !ok {
			return nil
		}
		for _, a := range c.args {
			if err := walk(a); err != nil {
				return err
			}
		}
		if c.fn != "jwt" {
			return nil
		}
		claims, err := c.args[2].eval(env)
		if err != nil {
			return nil
		}
		return jwtExpiry(claims.bytes(), now)
	}
	for _, p := range parts {
		if err := walk(p); err != nil {
			return err
		}
	}
	return nil
}

func (c call) eval(env Env) (value, error) {
	args := make([]value, len(c.args))
	for i, a := range c.args {
		v, err := a.eval(env)
		if err != nil {
			return value{}, err
		}
		args[i] = v
	}
	if c.fn == "jwt" {
		if err := jwtExpiry(args[2].bytes(), env.Now); err != nil {
			return value{}, err
		}
	}
	v, err := functions[c.fn].run(args)
	if err != nil {
		return value{}, fmt.Errorf("%s: %w", c.fn, err)
	}
	return v, nil
}

type function struct {
	min, max int
	check    func([]node) error
	run      func([]value) (value, error)
}

func (f function) arity() string {
	switch {
	case f.max < 0:
		return fmt.Sprintf("%d or more arguments", f.min)
	case f.min == f.max:
		return fmt.Sprintf("%d argument%s", f.min, map[bool]string{true: "", false: "s"}[f.min == 1])
	}
	return fmt.Sprintf("%d to %d arguments", f.min, f.max)
}

// Functions are fixed: a recipe composes them and adds none of its own (CONTRACT.md §3.1).
var functions map[string]function

func init() {
	one := func(fn func([]byte) []byte) function {
		return function{min: 1, max: 1, run: func(a []value) (value, error) { return value{b: fn(a[0].bytes())}, nil }}
	}
	onef := func(fn func([]byte) ([]byte, error)) function {
		return function{min: 1, max: 1, run: func(a []value) (value, error) {
			b, err := fn(a[0].bytes())
			return value{b: b}, err
		}}
	}
	mac := func(h func() hash.Hash) function {
		return function{min: 2, max: 2, run: func(a []value) (value, error) {
			m := hmac.New(h, a[0].bytes())
			m.Write(a[1].bytes())
			return value{b: m.Sum(nil)}, nil
		}}
	}
	sum := func(h func() hash.Hash) function {
		return one(func(b []byte) []byte { x := h(); x.Write(b); return x.Sum(nil) })
	}
	functions = map[string]function{
		"hmac_sha1":   mac(sha1.New),
		"hmac_sha256": mac(sha256.New),
		"hmac_sha512": mac(sha512.New),
		"sha1":        sum(sha1.New),
		"sha256":      sum(sha256.New),
		"sha512":      sum(sha512.New),
		"base64":      one(func(b []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(b)) }),
		"base64url":   one(func(b []byte) []byte { return []byte(base64.RawURLEncoding.EncodeToString(b)) }),
		"hex":         one(func(b []byte) []byte { return []byte(hex.EncodeToString(b)) }),
		"base64_decode": onef(func(b []byte) ([]byte, error) {
			s := strings.TrimRight(strings.TrimSpace(string(b)), "=")
			if d, err := base64.RawStdEncoding.DecodeString(s); err == nil {
				return d, nil
			}
			return base64.RawURLEncoding.DecodeString(s)
		}),
		"hex_decode": onef(func(b []byte) ([]byte, error) { return hex.DecodeString(strings.TrimSpace(string(b))) }),
		"urlencode":  one(func(b []byte) []byte { return []byte(strings.ReplaceAll(url.QueryEscape(string(b)), "+", "%20")) }),
		"upper":      one(func(b []byte) []byte { return []byte(strings.ToUpper(string(b))) }),
		"lower":      one(func(b []byte) []byte { return []byte(strings.ToLower(string(b))) }),
		"trim":       one(func(b []byte) []byte { return []byte(strings.TrimSpace(string(b))) }),
		"sort_query": one(func(b []byte) []byte { return []byte(sortQuery(string(b))) }),
		"concat": {min: 1, max: -1, run: func(a []value) (value, error) {
			var out []byte
			for _, v := range a {
				out = append(out, v.bytes()...)
			}
			return value{b: out}, nil
		}},
		"basic": {min: 2, max: 2, run: func(a []value) (value, error) {
			return str("Basic " + base64.StdEncoding.EncodeToString([]byte(a[0].text()+":"+a[1].text()))), nil
		}},
		"add": {min: 2, max: 2, check: numbers, run: func(a []value) (value, error) {
			x, err := toNum(a[0])
			if err != nil {
				return value{}, err
			}
			y, err := toNum(a[1])
			if err != nil {
				return value{}, err
			}
			return value{num: x + y, isNum: true}, nil
		}},
		"json": {min: 0, max: -1, check: pairs, run: func(a []value) (value, error) {
			var b strings.Builder
			b.WriteByte('{')
			for i := 0; i+1 < len(a); i += 2 {
				if i > 0 {
					b.WriteByte(',')
				}
				k, _ := json.Marshal(a[i].text())
				b.Write(k)
				b.WriteByte(':')
				switch v := a[i+1]; {
				case v.isNum, v.isJSON:
					b.Write(v.bytes())
				default:
					s, _ := json.Marshal(v.text())
					b.Write(s)
				}
			}
			b.WriteByte('}')
			return value{b: []byte(b.String()), isJSON: true}, nil
		}},
		"jwt":  {min: 3, max: 4, check: algorithm(jwtAlgs), run: runJWT},
		"sign": {min: 3, max: 3, check: algorithm(signAlgs), run: runSign},
	}
}

func numbers(args []node) error {
	for _, a := range args {
		if l, ok := a.(lit); ok && !l.v.isNum {
			if _, err := strconv.ParseInt(l.v.text(), 10, 64); err != nil {
				return errors.New("takes whole numbers")
			}
		}
	}
	return nil
}

func toNum(v value) (int64, error) {
	if v.isNum {
		return v.num, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v.text()), 10, 64)
	if err != nil {
		return 0, errors.New("takes whole numbers")
	}
	return n, nil
}

func pairs(args []node) error {
	if len(args)%2 != 0 {
		return errors.New("takes name, value pairs")
	}
	return nil
}

var jwtAlgs = map[string]bool{"HS256": true, "HS384": true, "HS512": true, "RS256": true, "RS384": true, "RS512": true, "PS256": true, "ES256": true, "EdDSA": true}
var signAlgs = map[string]bool{"RS256": true, "RS384": true, "RS512": true, "PS256": true, "ES256": true, "EdDSA": true}

func algorithm(known map[string]bool) func([]node) error {
	return func(args []node) error {
		l, ok := args[0].(lit)
		if !ok {
			return errors.New("the algorithm is a quoted name, for example 'RS256'")
		}
		if !known[l.v.text()] {
			names := make([]string, 0, len(known))
			for k := range known {
				names = append(names, k)
			}
			sort.Strings(names)
			return fmt.Errorf("%s is not one of %s", l.v.text(), strings.Join(names, ", "))
		}
		return nil
	}
}

func runJWT(a []value) (value, error) {
	alg := a[0].text()
	header := `{"alg":"` + alg + `","typ":"JWT"`
	if len(a) == 4 {
		kid, _ := json.Marshal(a[3].text())
		header += `,"kid":` + string(kid)
	}
	header += "}"
	claims := a[2].bytes()
	if !json.Valid(claims) {
		return value{}, errors.New("claims are not JSON; build them with json(...)")
	}
	input := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	var sig []byte
	var err error
	switch alg {
	case "HS256":
		sig = macOf(sha256.New, a[1].bytes(), input)
	case "HS384":
		sig = macOf(sha512.New384, a[1].bytes(), input)
	case "HS512":
		sig = macOf(sha512.New, a[1].bytes(), input)
	default:
		sig, err = signWith(alg, a[1].bytes(), []byte(input))
	}
	if err != nil {
		return value{}, err
	}
	return str(input + "." + base64.RawURLEncoding.EncodeToString(sig)), nil
}

func macOf(h func() hash.Hash, key []byte, msg string) []byte {
	m := hmac.New(h, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func runSign(a []value) (value, error) {
	sig, err := signWith(a[0].text(), a[1].bytes(), a[2].bytes())
	return value{b: sig}, err
}

// signWith signs msg with a PEM private key. ES256 gives r||s, the form JWTs use.
func signWith(alg string, keyPEM, msg []byte) ([]byte, error) {
	key, err := privateKey(keyPEM)
	if err != nil {
		return nil, err
	}
	digest := func(h crypto.Hash) []byte { x := h.New(); x.Write(msg); return x.Sum(nil) }
	switch alg {
	case "RS256", "RS384", "RS512", "PS256":
		k, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New(alg + " needs an RSA private key")
		}
		h := map[string]crypto.Hash{"RS256": crypto.SHA256, "RS384": crypto.SHA384, "RS512": crypto.SHA512, "PS256": crypto.SHA256}[alg]
		if alg == "PS256" {
			return rsa.SignPSS(rand.Reader, k, h, digest(h), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
		}
		return rsa.SignPKCS1v15(rand.Reader, k, h, digest(h))
	case "ES256":
		k, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("ES256 needs an EC P-256 private key")
		}
		r, s, err := ecdsa.Sign(rand.Reader, k, digest(crypto.SHA256))
		if err != nil {
			return nil, err
		}
		return append(pad32(r), pad32(s)...), nil
	case "EdDSA":
		k, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, errors.New("EdDSA needs an Ed25519 private key")
		}
		return ed25519.Sign(k, msg), nil
	}
	return nil, errors.New("unknown algorithm " + alg)
}

func pad32(n *big.Int) []byte {
	b := n.Bytes()
	return append(make([]byte, 32-len(b)), b...)
}

func privateKey(p []byte) (any, error) {
	// A PEM stored as a secret often arrives with literal \n in place of line breaks.
	p = []byte(strings.ReplaceAll(string(p), `\n`, "\n"))
	block, _ := pem.Decode(p)
	if block == nil {
		return nil, errors.New("the key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, errors.New("the key is not a PKCS#8, PKCS#1 or EC private key")
}

// sortQuery orders a raw query string's pairs by name then value, keeping their encoding.
func sortQuery(q string) string {
	if q == "" {
		return ""
	}
	ps := strings.Split(q, "&")
	sort.Strings(ps)
	return strings.Join(ps, "&")
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
