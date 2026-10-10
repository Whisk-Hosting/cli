package connect

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)

func env(secrets map[string]string, req *Request) Env {
	return Env{Secrets: secrets, Request: req, Token: "tok", Now: at, Nonce: "n0", UUID: "u0"}
}

func eval(t *testing.T, src string, ctx Context, e Env) string {
	t.Helper()
	tm, _, err := ParseTemplate(src, ctx)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	v, err := tm.Eval(e)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

func TestTemplates(t *testing.T) {
	req := &Request{Method: "GET", URL: "https://api.example.com/v1/stock?a=1", Host: "api.example.com", Path: "/v1/stock", Query: "b=2&a=1", Body: []byte(`{"x":1}`)}
	e := env(map[string]string{"KEY": "k3y", "USER": "jo", "PASS": "pw", "B64": base64.StdEncoding.EncodeToString([]byte("raw"))}, req)
	all := Context{Request: true, Token: true}
	mac := hmac.New(sha256.New, []byte("k3y"))
	mac.Write([]byte("b=2&a=1"))
	sum := sha256.Sum256([]byte(`{"x":1}`))
	for _, c := range []struct{ src, want string }{
		{"Bearer {secret.KEY}", "Bearer k3y"},
		{"{{literal}} {secret.KEY}", "{literal} k3y"},
		{"{base64(hmac_sha256(secret.KEY, request.query))}", base64.StdEncoding.EncodeToString(mac.Sum(nil))},
		{"{hex(sha256(request.body))}", hex.EncodeToString(sum[:])},
		{"{basic(secret.USER, secret.PASS)}", "Basic " + base64.StdEncoding.EncodeToString([]byte("jo:pw"))},
		{"{time.unix}", "1791507723"},
		{"{time.amz}/{time.date}", "20261009T010203Z/20261009"},
		{"{add(time.unix, 300)}", "1791508023"},
		{"{concat(request.method, ' ', lower('ABC'))}", "GET abc"},
		{"{sort_query(request.query)}", "a=1&b=2"},
		{"{urlencode('a b/c')}", "a%20b%2Fc"},
		{"{base64_decode(secret.B64)}", "raw"},
		{"{json('iss', secret.USER, 'iat', time.unix, 'n', json('a', 1))}", `{"iss":"jo","iat":1791507723,"n":{"a":1}}`},
		{"{token}:{nonce}:{uuid}", "tok:n0:u0"},
		{"{'it\\'s'}", "it's"},
	} {
		if got := eval(t, c.src, all, e); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestTemplateRefusals(t *testing.T) {
	for _, c := range []struct {
		src string
		ctx Context
	}{
		{"{secret.lower}", Context{Request: true}},
		{"{secret}", Context{Request: true}},
		{"{request.path}", Context{}},
		{"{request.cookie}", Context{Request: true}},
		{"{token}", Context{Request: true}},
		{"{env.HOME}", Context{Request: true}},
		{"{exec('ls')}", Context{Request: true}},
		{"{hmac_sha256(secret.KEY)}", Context{Request: true}},
		{"{jwt('none', secret.KEY, json())}", Context{Request: true}},
		{"{jwt(secret.ALG, secret.KEY, json())}", Context{Request: true}},
		{"{json('a')}", Context{Request: true}},
		{"{unclosed", Context{Request: true}},
		{"stray }", Context{Request: true}},
		{"{'open}", Context{Request: true}},
		{"{}", Context{Request: true}},
		{"{base64(secret.KEY) extra}", Context{Request: true}},
	} {
		if _, _, err := ParseTemplate(c.src, c.ctx); err == nil {
			t.Errorf("%q parsed; want a refusal", c.src)
		}
	}
}

// TestJWTRules holds the mechanical limits on a signed JWT (CONTRACT.md §3.1): its claims are
// the recipe's own, never the request's, and it expires within the hour.
func TestJWTRules(t *testing.T) {
	refused := []string{
		"{jwt('HS256', secret.KK, json('aud', request.body, 'exp', add(time.unix, 60)))}",
		"{jwt('HS256', secret.KK, json('scope', request.query, 'exp', add(time.unix, 60)))}",
		"{jwt('HS256', secret.KK, json('exp', add(time.unix, 60)), request.path)}",
		"{jwt('HS256', secret.KK, json('sub', 'x'))}",
		"{jwt('HS256', secret.KK, json('exp', add(time.unix, 3601)))}",
		"{jwt('HS256', secret.KK, json('exp', 4102444800))}",
		"{jwt('HS256', secret.KK, json('exp', time.unix))}",
		"{jwt('HS256', secret.KK, json('exp', 'soon'))}",
	}
	for _, src := range refused {
		if _, _, err := ParseTemplate(src, Context{Request: true}); err == nil {
			t.Errorf("%s was accepted", src)
		}
	}
	ok := "{jwt('HS256', secret.KK, json('iss', secret.ISS, 'aud', 'https://api.example.com', 'iat', time.unix, 'exp', add(time.unix, 3600)))}"
	if _, _, err := ParseTemplate(ok, Context{Request: true}); err != nil {
		t.Errorf("a recipe's own claims expiring in an hour: %v", err)
	}
	if err := jwtExpiry([]byte(`{"exp":100}`), time.Unix(200, 0)); err == nil {
		t.Error("an expired JWT would be signed")
	}
}

func TestMissingSecret(t *testing.T) {
	tm, names, err := ParseTemplate("{secret.AA}{secret.BB}{secret.AA}", Context{})
	if err != nil || strings.Join(names, ",") != "AA,BB" {
		t.Fatalf("names %v err %v", names, err)
	}
	_, err = tm.Eval(env(map[string]string{"AA": "x"}, nil))
	var miss *MissingSecretError
	if !errors.As(err, &miss) || miss.Name != "BB" {
		t.Fatalf("err %v, want B missing", err)
	}
}

func TestJWT(t *testing.T) {
	e := env(map[string]string{"KK": "secret"}, nil)
	got := eval(t, "{jwt('HS256', secret.KK, json('sub', '1', 'exp', add(time.unix, 300)), 'kid1')}", Context{}, e)
	parts := strings.Split(got, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %s", got)
	}
	head, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if string(head) != `{"alg":"HS256","typ":"JWT","kid":"kid1"}` {
		t.Fatalf("header %s", head)
	}
	m := hmac.New(sha256.New, []byte("secret"))
	m.Write([]byte(parts[0] + "." + parts[1]))
	if parts[2] != base64.RawURLEncoding.EncodeToString(m.Sum(nil)) {
		t.Fatal("HS256 signature does not verify")
	}

	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	rp := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rk)})
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	eb, _ := x509.MarshalPKCS8PrivateKey(ek)
	_, dk, _ := ed25519.GenerateKey(rand.Reader)
	db, _ := x509.MarshalPKCS8PrivateKey(dk)
	keys := map[string]string{
		"RSA": strings.ReplaceAll(string(rp), "\n", `\n`), // stored with escaped line breaks
		"EC":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: eb})),
		"ED":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: db})),
	}
	for alg, key := range map[string]string{"RS256": "RSA", "PS256": "RSA", "ES256": "EC", "EdDSA": "ED"} {
		tok := eval(t, "{jwt('"+alg+"', secret."+key+", json('a', 1, 'exp', add(time.unix, 60)))}", Context{}, env(keys, nil))
		if len(strings.Split(tok, ".")) != 3 {
			t.Errorf("%s: %s", alg, tok)
		}
	}
	if _, err := mustTemplate(t, "{jwt('RS256', secret.EC, json('exp', add(time.unix, 60)))}").Eval(env(keys, nil)); err == nil {
		t.Error("RS256 with an EC key signed")
	}
	if _, _, err := ParseTemplate("{jwt('HS256', secret.KK, 'not json')}", Context{}); err == nil {
		t.Error("claims that are not JSON were accepted")
	}
}

func mustTemplate(t *testing.T, src string) Template {
	t.Helper()
	tm, _, err := ParseTemplate(src, Context{Request: true, Token: true})
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func conn() Connection {
	return Connection{
		URL: "https://api.example.com/v2",
		Auth: Auth{
			Headers: map[string]string{"Authorization": "Bearer {token}", "X-Sig": "{hex(hmac_sha256(secret.SIGN, request.path))}"},
			Token:   &TokenStep{URL: "https://login.example.com/token", Form: map[string]string{"client_id": "{secret.ID}", "client_secret": "{secret.SECRET}"}},
		},
		Operations: []Operation{{Name: "Read stock", Method: "get", Path: "/stock/**"}, {Name: "Create orders", Method: "POST", Path: "/orders"}},
	}.WithDefaults()
}

func TestCheck(t *testing.T) {
	ps, secrets := Check(conn())
	if len(ps) != 0 {
		t.Fatalf("problems %v", ps)
	}
	if strings.Join(secrets, ",") != "ID,SECRET,SIGN" {
		t.Fatalf("secrets %v", secrets)
	}
	bad := func(edit func(*Connection), path string) {
		t.Helper()
		c := conn()
		edit(&c)
		ps, _ := Check(c.WithDefaults())
		for _, p := range ps {
			if p.Path == path {
				return
			}
		}
		t.Errorf("no problem at %s: %v", path, ps)
	}
	bad(func(c *Connection) { c.URL = "ftp://x" }, "/url")
	bad(func(c *Connection) { c.URL = "https://u:p@x.com" }, "/url")
	bad(func(c *Connection) { c.URL = "https://x.com/a?b=1" }, "/url")
	bad(func(c *Connection) { c.URL = "https://x.com/a/../b" }, "/url")
	bad(func(c *Connection) { c.Auth.Headers["Host"] = "x" }, "/auth/headers/Host")
	bad(func(c *Connection) { c.Auth.Headers["X-Whisk-Org"] = "x" }, "/auth/headers/X-Whisk-Org")
	bad(func(c *Connection) { c.Auth.Headers["Bad Header"] = "x" }, "/auth/headers/Bad Header")
	bad(func(c *Connection) { c.Auth.Token.Form["x"] = "{request.path}" }, "/auth/token/form/x")
	bad(func(c *Connection) { c.Auth.Token.JSON = map[string]string{"a": "b"} }, "/auth/token")
	bad(func(c *Connection) { c.Auth.Token.Method = "PUT" }, "/auth/token/method")
	bad(func(c *Connection) { c.Auth.Token.Lifetime = MaxLifetime + 1 }, "/auth/token/lifetime")
	bad(func(c *Connection) { c.Operations = nil }, "/operations")
	bad(func(c *Connection) { c.Operations[1].Name = "read STOCK" }, "/operations/1/name")
	bad(func(c *Connection) { c.Operations[0].Name = "" }, "/operations/0/name")
	bad(func(c *Connection) { c.Operations[0].Method = "TRACE" }, "/operations/0/method")
	bad(func(c *Connection) { c.Operations[0].Path = "/a/../b" }, "/operations/0/path")
	bad(func(c *Connection) { c.Operations[0].Path = "stock" }, "/operations/0/path")
	bad(func(c *Connection) { c.Auth.Headers = nil }, "/auth")

	keyless := Connection{URL: "https://api.weather.example", Operations: []Operation{{Name: "Forecast", Method: "GET", Path: "/**"}}}.WithDefaults()
	if ps, _ := Check(keyless); len(ps) != 0 {
		t.Fatalf("a keyless connection is refused: %v", ps)
	}
}

func TestGapsAndEffective(t *testing.T) {
	g := conn()
	if !Covers(&g, conn()) {
		t.Fatal("the same connection is not covered")
	}
	fewer := conn()
	fewer.Operations = fewer.Operations[:1]
	if !Covers(&g, fewer) {
		t.Fatal("fewer operations need a grant")
	}
	more := conn()
	more.Operations = append(more.Operations, Operation{Name: "Delete orders", Method: "DELETE", Path: "/orders/*"})
	if changed, ops := Gaps(&g, more); changed || len(ops) != 1 || ops[0].Method != "DELETE" {
		t.Fatalf("gaps %v %v", changed, ops)
	}
	relabelled := conn()
	relabelled.Operations[0].Name = "Look at things"
	if Covers(&g, relabelled) {
		t.Fatal("a renamed operation is covered; the name is part of the version a person saw")
	}
	moved := conn()
	moved.URL = "https://evil.example.com/v2"
	if changed, _ := Gaps(&g, moved); !changed {
		t.Fatal("a new address is not a change")
	}
	resigned := conn()
	resigned.Auth.Headers["X-Leak"] = "{secret.SIGN}"
	if changed, _ := Gaps(&g, resigned); !changed {
		t.Fatal("a new recipe is not a change")
	}
	if changed, ops := Gaps(nil, conn()); !changed || len(ops) != 2 {
		t.Fatal("no grant is not every gap")
	}
	if e := Effective(&g, more); e == nil || len(e.Operations) != 2 {
		t.Fatalf("effective %v", e)
	}
	if Effective(&g, moved) != nil || Effective(&g, resigned) != nil || Effective(nil, conn()) != nil {
		t.Fatal("an ungranted address or recipe is effective")
	}
	if Canonical(conn()) != Canonical(func() Connection {
		c := conn()
		c.Operations[0], c.Operations[1] = c.Operations[1], c.Operations[0]
		return c
	}()) {
		t.Fatal("canonical form depends on operation order")
	}
}

func TestMatch(t *testing.T) {
	ops := conn().Operations
	for _, c := range []struct {
		method, path string
		ok           bool
	}{
		{"GET", "/stock/42", true},
		{"HEAD", "/stock/42", true},
		{"GET", "/stock/a/b", true},
		{"POST", "/orders", true},
		{"POST", "/orders/", true},
		{"PUT", "/orders", false},
		{"GET", "/payroll", false},
		{"GET", "/stock/../payroll", false},
		{"GET", "/stock/%2e%2e/payroll", false},
		{"GET", "/stock/a%2Fb", false},
		{"GET", "/stock/a%5cb", false},
		{"GET", "/stock/%zz", false},
	} {
		if _, ok := Match(ops, c.method, c.path); ok != c.ok {
			t.Errorf("%s %s matched %v, want %v", c.method, c.path, ok, c.ok)
		}
	}
}

func TestUsageAndDescribe(t *testing.T) {
	c := conn()
	c.Auth.Query = map[string]string{"key": "{base64(secret.PLAIN)}", "sig": "{hex(sha256(concat(secret.HASHED, time.unix)))}"}
	got := map[string]Use{}
	for _, u := range Usage(c) {
		got[u.Name] = u
	}
	want := map[string]Use{
		"ID":     {Name: "ID", Sent: true, To: []string{"login.example.com"}},
		"SECRET": {Name: "SECRET", Sent: true, To: []string{"login.example.com"}},
		"SIGN":   {Sent: false},
		"PLAIN":  {Sent: true, To: []string{"api.example.com"}},
		"HASHED": {Sent: false},
	}
	for n, w := range want {
		g, ok := got[n]
		if !ok || g.Sent != w.Sent || strings.Join(g.To, ",") != strings.Join(w.To, ",") {
			t.Errorf("%s: %+v, want sent %v to %v", n, g, w.Sent, w.To)
		}
	}
	jwtc := Connection{URL: "https://x.example", Auth: Auth{Headers: map[string]string{"Authorization": "Bearer {jwt('HS256', secret.KK, json('iss', secret.ISS, 'exp', add(time.unix, 60)))}"}}}
	for _, u := range Usage(jwtc) {
		if u.Name == "KK" && u.Sent || u.Name == "ISS" && !u.Sent {
			t.Errorf("jwt usage %+v", u)
		}
	}
	d := Describe(c)
	if d.Host != "api.example.com" || d.Port != "443" || d.BasePath != "/v2" || d.TokenHost != "login.example.com" || d.Insecure {
		t.Fatalf("describe %+v", d)
	}
	if d.Operations[0].Verb != "Read" || d.Operations[0].Label != "Read stock" || d.Operations[1].Verb != "Create" {
		t.Fatalf("operations %+v", d.Operations)
	}
}

func TestApplyAndToken(t *testing.T) {
	c := conn()
	req := &Request{Method: "GET", Path: "/v2/stock/1"}
	placed, err := Apply(c, env(map[string]string{"SIGN": "s"}, req))
	if err != nil {
		t.Fatal(err)
	}
	m := hmac.New(sha256.New, []byte("s"))
	m.Write([]byte("/v2/stock/1"))
	if placed.Headers.Get("Authorization") != "Bearer tok" || placed.Headers.Get("X-Sig") != hex.EncodeToString(m.Sum(nil)) {
		t.Fatalf("placed %v", placed.Headers)
	}
	if _, err := Apply(Connection{Auth: Auth{Headers: map[string]string{"X": "{secret.NL}"}}}, env(map[string]string{"NL": "a\r\nInjected: 1"}, req)); err == nil {
		t.Fatal("a value with a line break was placed")
	}
	tr, err := RenderToken(*c.Auth.Token, env(map[string]string{"ID": "id", "SECRET": "s&x"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Method != "POST" || string(tr.Body) != "client_id=id&client_secret=s%26x" || tr.Headers.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("token request %+v %s", tr, tr.Body)
	}
	step := TokenStep{Field: "data.token", ExpiresField: "data.ttl", Lifetime: 300}
	tok, life, err := ReadToken(step, []byte(`{"data":{"token":"abc","ttl":"120"}}`))
	if err != nil || tok != "abc" || life != 120 {
		t.Fatalf("token %q %d %v", tok, life, err)
	}
	if _, life, _ := ReadToken(step, []byte(`{"data":{"token":"abc"}}`)); life != 300 {
		t.Fatalf("default lifetime %d", life)
	}
	if _, _, err := ReadToken(step, []byte(`{"data":{}}`)); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err %v", err)
	}
	if _, _, err := ReadToken(step, []byte(`<html>`)); err == nil {
		t.Fatal("HTML read as a token")
	}
}

func TestUpstream(t *testing.T) {
	u, err := Upstream("https://api.example.com/v2/", "/stock/a%20b", "x=1")
	if err != nil || u.String() != "https://api.example.com/v2/stock/a%20b?x=1" || HostPort(u) != "api.example.com:443" {
		t.Fatalf("%v %v", u, err)
	}
}

func TestCanonicalRoundTrip(t *testing.T) {
	var back Connection
	if err := json.Unmarshal([]byte(Canonical(conn())), &back); err != nil || Canonical(back) != Canonical(conn()) {
		t.Fatalf("canonical does not round-trip: %v", err)
	}
}

func TestHashIgnoresOperationOrder(t *testing.T) {
	a := Connection{URL: "https://api.example.com", Operations: []Operation{{Name: "a", Method: "GET", Path: "/a"}, {Name: "b", Method: "GET", Path: "/b"}}}
	b := Connection{URL: "https://api.example.com", Operations: []Operation{{Name: "b", Method: "get", Path: "/b"}, {Name: "a", Method: "GET", Path: "/a"}}}
	if Hash(a) != Hash(b) || len(Hash(a)) != 64 {
		t.Fatalf("hash: %s %s", Hash(a), Hash(b))
	}
	b.URL = "https://api.example.org"
	if Hash(a) == Hash(b) {
		t.Fatal("a different address must hash differently")
	}
}

func TestRevokeStep(t *testing.T) {
	c := Connection{URL: "https://api.example.com", Operations: []Operation{{Name: "Read", Method: "GET", Path: "/**"}},
		Auth: Auth{Headers: map[string]string{"Authorization": "Bearer {token}"}, Token: &TokenStep{URL: "https://login.example.com/token",
			Form:   map[string]string{"client_id": "{secret.CID}", "client_secret": "{secret.CSEC}"},
			Revoke: &RevokeStep{URL: "https://login.example.com/revoke", Headers: map[string]string{"Authorization": "{basic(secret.CID, secret.CSEC)}"}}}}}.WithDefaults()
	if ps, _ := Check(c); len(ps) != 0 {
		t.Fatalf("problems %v", ps)
	}
	if !Describe(c).Revokes {
		t.Error("the summary does not say revoking cancels tokens")
	}
	req, err := RenderRevoke(*c.Auth.Token.Revoke, env(map[string]string{"CID": "id", "CSEC": "sec"}, nil), "tok-1")
	if err != nil || req.Method != "POST" || string(req.Body) != "token=tok-1&token_type_hint=access_token" || req.Headers.Get("Authorization") != "Basic aWQ6c2Vj" {
		t.Fatalf("revoke %+v %s %v", req, req.Body, err)
	}
	elsewhere := c
	tok := *c.Auth.Token
	tok.Revoke = &RevokeStep{URL: "https://evil.example.net/revoke"}
	elsewhere.Auth.Token = &tok
	if ps, _ := Check(elsewhere.WithDefaults()); len(ps) == 0 || ps[0].Path != "/auth/token/revoke/url" {
		t.Errorf("a revocation on another host: %v", ps)
	}
}
