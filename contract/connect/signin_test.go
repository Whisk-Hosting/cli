package connect

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

// books is a connection that signs in, the shape of Xero's.
func books() Connection {
	return Connection{URL: "https://api.example.com", Operations: []Operation{{Name: "Read pay runs", Method: "GET", Path: "/payruns/**"}},
		Auth: Auth{
			Headers: map[string]string{"Authorization": "Bearer {token}"},
			SignIn: &SignIn{
				Label:        "Books",
				Authorize:    "https://login.example.com/authorize",
				Token:        "https://identity.example.com/token",
				ClientID:     "{secret.BOOKS_ID}",
				ClientSecret: "{secret.BOOKS_SECRET}",
				Scopes:       []string{"offline_access", "payroll.read"},
				Params:       map[string]string{"prompt": "consent"},
				Revoke:       "https://identity.example.com/revoke",
			},
		}}.WithDefaults()
}

func TestCheckSignIn(t *testing.T) {
	ps, secrets := Check(books())
	if len(ps) != 0 {
		t.Fatalf("problems %v", ps)
	}
	if strings.Join(secrets, ",") != "BOOKS_ID,BOOKS_SECRET" {
		t.Errorf("secrets %v", secrets)
	}
	if books().Auth.SignIn.ClientAuth != ClientAuthBasic {
		t.Error("client_auth defaults to basic")
	}
	cases := []struct {
		name   string
		change func(c *Connection)
		at     string
	}{
		{"a token step too", func(c *Connection) {
			c.Auth.Token = &TokenStep{URL: "https://x.example.com/t", Method: "POST", Field: "a", ExpiresField: "b", Lifetime: 1}
		}, "/auth/sign_in"},
		{"no token placed", func(c *Connection) { c.Auth.Headers = nil }, "/auth"},
		{"an authorize address with a query", func(c *Connection) { c.Auth.SignIn.Authorize = "https://login.example.com/a?x=1" }, "/auth/sign_in/authorize"},
		{"a token address on port 25", func(c *Connection) { c.Auth.SignIn.Token = "https://identity.example.com:25/token" }, "/auth/sign_in/token"},
		{"a revocation elsewhere", func(c *Connection) { c.Auth.SignIn.Revoke = "https://evil.example.net/revoke" }, "/auth/sign_in/revoke"},
		{"no client", func(c *Connection) { c.Auth.SignIn.ClientID = "" }, "/auth/sign_in/client_id"},
		{"a client reading the request", func(c *Connection) { c.Auth.SignIn.ClientID = "{request.path}" }, "/auth/sign_in/client_id"},
		{"a secret reading the token", func(c *Connection) { c.Auth.SignIn.ClientSecret = "{token}" }, "/auth/sign_in/client_secret"},
		{"the vendor app unnamed", func(c *Connection) { c.Auth.SignIn.ClientID = "{vendor.client_id}" }, "/auth/sign_in/client_id"},
		{"another client auth", func(c *Connection) { c.Auth.SignIn.ClientAuth = "jwt" }, "/auth/sign_in/client_auth"},
		{"a scope with a space", func(c *Connection) { c.Auth.SignIn.Scopes = []string{"a b"} }, "/auth/sign_in/scopes/0"},
		{"a scope twice", func(c *Connection) { c.Auth.SignIn.Scopes = []string{"a", "a"} }, "/auth/sign_in/scopes/1"},
		{"a parameter Whisk sets", func(c *Connection) {
			c.Auth.SignIn.Params = map[string]string{"redirect_uri": "https://evil.example.net"}
		}, "/auth/sign_in/params/redirect_uri"},
		{"an empty parameter", func(c *Connection) { c.Auth.SignIn.Params = map[string]string{"prompt": ""} }, "/auth/sign_in/params/prompt"},
		{"a label that is a sentence", func(c *Connection) { c.Auth.SignIn.Label = "Xero! Click here, now, to win a prize today" }, "/auth/sign_in/label"},
	}
	for _, tc := range cases {
		c := books()
		si := *c.Auth.SignIn
		c.Auth.SignIn = &si
		tc.change(&c)
		ps, _ := Check(c)
		found := false
		for _, p := range ps {
			found = found || p.Path == tc.at
		}
		if !found {
			t.Errorf("%s: want a problem at %s, got %v", tc.name, tc.at, ps)
		}
	}
	vendor := books()
	si := *vendor.Auth.SignIn
	si.ClientID, si.ClientSecret = "{vendor.client_id}", "{vendor.client_secret}"
	vendor.Auth.SignIn, vendor.Auth.VendorApp = &si, "books"
	if ps, _ := Check(vendor); len(ps) != 0 {
		t.Errorf("a vendor app's client: %v", ps)
	}
}

func TestAuthorizeURL(t *testing.T) {
	si := *books().Auth.SignIn
	got, err := url.Parse(AuthorizeURL(si, "id 1", "https://auth.whisk.run/connections/callback", "st", "verifier"))
	if err != nil || got.Host != "login.example.com" || got.Path != "/authorize" {
		t.Fatalf("%v %v", got, err)
	}
	q := got.Query()
	want := map[string]string{"response_type": "code", "client_id": "id 1", "redirect_uri": "https://auth.whisk.run/connections/callback",
		"scope": "offline_access payroll.read", "state": "st", "code_challenge_method": "S256", "prompt": "consent",
		// RFC 7636 appendix B's shape: the unpadded base64url SHA-256 of the verifier.
		"code_challenge": Challenge("verifier")}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Error("the challenge is not RFC 7636's")
	}
}

func TestSignInRequest(t *testing.T) {
	si := *books().Auth.SignIn
	env := NewEnv(map[string]string{"BOOKS_ID": "id:1", "BOOKS_SECRET": "s&2"}, nil, "", time.Now())
	client, err := RenderClient(si, env)
	if err != nil || client.ID != "id:1" || client.Secret != "s&2" {
		t.Fatalf("%+v %v", client, err)
	}
	basic := SignInRequest(si, client, si.Token, RenewForm("r1"))
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("id%3A1:s%262"))
	if basic.Headers.Get("Authorization") != want || strings.Contains(string(basic.Body), "client_secret") {
		t.Errorf("basic: %v %s", basic.Headers, basic.Body)
	}
	if f, _ := url.ParseQuery(string(basic.Body)); f.Get("grant_type") != "refresh_token" || f.Get("refresh_token") != "r1" {
		t.Errorf("form %s", basic.Body)
	}
	si.ClientAuth = ClientAuthForm
	form := SignInRequest(si, client, si.Token, ExchangeForm("c", "https://r", "v"))
	f, _ := url.ParseQuery(string(form.Body))
	if form.Headers.Get("Authorization") != "" || f.Get("client_id") != "id:1" || f.Get("client_secret") != "s&2" || f.Get("code_verifier") != "v" || f.Get("grant_type") != "authorization_code" {
		t.Errorf("form: %v %s", form.Headers, form.Body)
	}
	if _, err := RenderClient(si, NewEnv(nil, nil, "", time.Now())); err == nil {
		t.Error("a client with no value rendered")
	}
}

func TestReadSignIn(t *testing.T) {
	got, err := ReadSignIn([]byte(`{"access_token":"a","refresh_token":"r","expires_in":1800,"scope":"offline_access payroll.read","token_type":"Bearer"}`))
	if err != nil || got.Access != "a" || got.Refresh != "r" || got.ExpiresIn != 1800 || len(got.Scopes) != 2 {
		t.Errorf("%+v %v", got, err)
	}
	got, err = ReadSignIn([]byte(`{"access_token":"a","expires_in":"3600"}`))
	if err != nil || got.ExpiresIn != 3600 || got.Refresh != "" {
		t.Errorf("a string lifetime: %+v %v", got, err)
	}
	for _, body := range []string{`{}`, `nope`, `{"access_token":"a\nb"}`} {
		if _, err := ReadSignIn([]byte(body)); err == nil {
			t.Errorf("%s read", body)
		}
	}
	if !Refused(400) || !Refused(401) || Refused(500) || Refused(429) {
		t.Error("refused")
	}
}

func TestSignInIsPartOfTheGrant(t *testing.T) {
	a, b := books(), books()
	si := *b.Auth.SignIn
	si.Scopes = []string{"offline_access", "payroll.write"}
	b.Auth.SignIn = &si
	if changed, _ := Gaps(&a, b); !changed {
		t.Error("a changed scope does not wait for a person")
	}
	if SignInHash(a) == SignInHash(b) {
		t.Error("a changed scope keeps the sign-in")
	}
	c := books()
	c.Operations = append(c.Operations, Operation{Name: "Read payslips", Method: "GET", Path: "/payslips/*"})
	if SignInHash(a) != SignInHash(c) {
		t.Error("an added operation needs a new sign-in")
	}
	if SignInHash(Connection{}) != "" {
		t.Error("a connection that does not sign in has a hash")
	}
}

func TestDescribeSignIn(t *testing.T) {
	s := Describe(books())
	if s.SignIn == nil || s.SignIn.Host != "login.example.com" || s.SignIn.Label != "Books" || !s.SignIn.Revokes || len(s.SignIn.Scopes) != 2 || s.TokenHost != "identity.example.com" {
		t.Errorf("%+v %+v", s.SignIn, s)
	}
	for _, u := range s.Secrets {
		if !u.Sent || len(u.To) != 1 || u.To[0] != "identity.example.com" {
			t.Errorf("%+v", u)
		}
	}
	c := books()
	si := *c.Auth.SignIn
	si.Label = ""
	c.Auth.SignIn = &si
	if Describe(c).SignIn.Label != "login.example.com" {
		t.Error("no label names the host")
	}
	placed, err := Apply(books(), NewEnv(nil, &Request{Method: "GET"}, "tok", time.Now()))
	if err != nil || placed.Headers.Get("Authorization") != "Bearer tok" {
		t.Errorf("%v %v", placed, err)
	}
}
