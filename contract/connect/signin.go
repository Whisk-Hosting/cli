package connect

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// SignIn is a connection that a person signs in to (OAuth 2's authorization code, RFC 6749 §4.1,
// with PKCE, RFC 7636): Xero, MYOB, QuickBooks, Google, Microsoft. The control plane keeps the
// sign-in and renews it; the broker places its current access token as {token}
// (CONTRACT.md §3.1 "Signing in", CONTROL-PLANE.md §6.8 "Signing in").
type SignIn struct {
	// Label is the system's name as a person knows it ("Xero"), for the button that says where
	// they sign in; the sign-in's host is shown beside it.
	Label string `json:"label,omitempty"`
	// Authorize is where the person signs in; Token is where a code and a refresh token become
	// an access token.
	Authorize string `json:"authorize"`
	Token     string `json:"token"`
	// ClientID and ClientSecret are templates naming the system's app the person signs in to:
	// the business's own (secret.NAME) or Whisk's (vendor.client_id). No secret is a public
	// client, kept by PKCE alone.
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	// ClientAuth is how the client's ID and secret reach the token address: basic (an
	// Authorization header, RFC 6749 §2.3.1's default) or form.
	ClientAuth string            `json:"client_auth,omitempty"`
	Scopes     []string          `json:"scopes,omitempty"`
	Params     map[string]string `json:"params,omitempty"`
	// Revoke, on the token address's host, is where revoking the grant cancels the refresh
	// token (RFC 7009).
	Revoke string `json:"revoke,omitempty"`
}

// Client authentication at the token address.
const (
	ClientAuthBasic = "basic"
	ClientAuthForm  = "form"
)

// Sign-in limits.
const (
	MaxScopes       = 50
	MaxScope        = 200
	MaxSignInParams = 4
	MaxParam        = 200
)

// scopeToken is RFC 6749 §3.3's scope-token: printable ASCII without space, quote or backslash.
var scopeToken = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]+$`)

// Label is a sign-in's name for the system, as a person reads it on its button.
var Label = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .&'()-]{0,39}$`)

// paramName is a query name the recipe may add to the sign-in address.
var paramName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// reservedParams are the sign-in address's values Whisk sets itself.
var reservedParams = map[string]bool{"response_type": true, "client_id": true, "redirect_uri": true, "scope": true, "state": true, "code_challenge": true, "code_challenge_method": true}

// withDefaults fills client_auth.
func (s SignIn) withDefaults() SignIn {
	if s.ClientAuth == "" {
		s.ClientAuth = ClientAuthBasic
	}
	s.ClientAuth = strings.ToLower(s.ClientAuth)
	return s
}

// checkSignIn adds every problem with a sign-in; tmpl checks one template.
func checkSignIn(c Connection, add func(string, string, ...any), tmpl func(string, string, Context)) {
	s := c.Auth.SignIn
	if s == nil {
		return
	}
	if c.Auth.Token != nil {
		add("/auth/sign_in", "a connection signs in or has a token step, not both")
	}
	if len(c.Auth.Headers) == 0 && len(c.Auth.Query) == 0 && len(c.Auth.Body) == 0 {
		add("/auth", "the sign-in's token is never placed; use {token} in a header or query value")
	}
	if s.Label != "" && !Label.MatchString(s.Label) {
		add("/auth/sign_in/label", "the system's name in plain words, up to 40 characters, such as Xero")
	}
	if msg := checkURL(s.Authorize); msg != "" {
		add("/auth/sign_in/authorize", "%s", msg)
	}
	if msg := checkURL(s.Token); msg != "" {
		add("/auth/sign_in/token", "%s", msg)
	}
	if s.Revoke != "" {
		if msg := checkURL(s.Revoke); msg != "" {
			add("/auth/sign_in/revoke", "%s", msg)
		} else if hostOf(s.Revoke) != hostOf(s.Token) {
			add("/auth/sign_in/revoke", "on the token address's own host, %s", hostOf(s.Token))
		}
	}
	if strings.TrimSpace(s.ClientID) == "" {
		add("/auth/sign_in/client_id", "the client ID of the system's app, such as {secret.XERO_CLIENT_ID}")
	} else {
		tmpl("/auth/sign_in/client_id", s.ClientID, Context{})
	}
	if s.ClientSecret != "" {
		tmpl("/auth/sign_in/client_secret", s.ClientSecret, Context{})
	}
	if s.ClientAuth != ClientAuthBasic && s.ClientAuth != ClientAuthForm {
		add("/auth/sign_in/client_auth", "basic or form")
	}
	if len(s.Scopes) > MaxScopes {
		add("/auth/sign_in/scopes", "at most %d scopes", MaxScopes)
	}
	seen := map[string]bool{}
	for i, sc := range s.Scopes {
		if len(sc) > MaxScope || !scopeToken.MatchString(sc) {
			add(fmt.Sprintf("/auth/sign_in/scopes/%d", i), "1 to %d characters without spaces, quotes or backslashes", MaxScope)
		}
		if seen[sc] {
			add(fmt.Sprintf("/auth/sign_in/scopes/%d", i), "%s is listed twice", sc)
		}
		seen[sc] = true
	}
	if len(s.Params) > MaxSignInParams {
		add("/auth/sign_in/params", "at most %d parameters", MaxSignInParams)
	}
	for _, k := range sortedKeys(s.Params) {
		p := "/auth/sign_in/params/" + escapePointer(k)
		switch {
		case !paramName.MatchString(k):
			add(p, "a parameter name: letters, digits, _, . and -")
		case reservedParams[k]:
			add(p, "Whisk sets %s itself", k)
		}
		if v := s.Params[k]; v == "" || len(v) > MaxParam || strings.ContainsAny(v, "\r\n") {
			add(p, "a value of 1 to %d characters on one line", MaxParam)
		}
	}
}

// SignInHash identifies a sign-in's recipe: the SHA-256 of its canonical JSON, in hex. A person's
// sign-in holds while the recipe's sign_in block is the one they signed in under, so adding an
// operation keeps it and changing a scope or the client asks them again. Empty without one.
func SignInHash(c Connection) string {
	if c.Auth.SignIn == nil {
		return ""
	}
	b, _ := json.Marshal(c.Auth.SignIn.withDefaults())
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Challenge is PKCE's S256 code challenge for a verifier (RFC 7636 §4.2).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL is where the person's browser goes to sign in: the recipe's authorize address
// with the values Whisk sets and the recipe's own parameters.
func AuthorizeURL(s SignIn, clientID, redirect, state, verifier string) string {
	q := url.Values{}
	for k, v := range s.Params {
		q.Set(k, v)
	}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirect)
	if len(s.Scopes) > 0 {
		q.Set("scope", strings.Join(s.Scopes, " "))
	}
	q.Set("state", state)
	q.Set("code_challenge", Challenge(verifier))
	q.Set("code_challenge_method", "S256")
	return s.Authorize + "?" + q.Encode()
}

// Client is a sign-in's client ID and secret, rendered.
type Client struct {
	ID     string
	Secret string
}

// RenderClient renders a sign-in's client ID and secret from the connection's secrets and
// vendor app. A secret with no value is a MissingSecretError.
func RenderClient(s SignIn, env Env) (Client, error) {
	env.Request, env.Token = nil, ""
	id, err := render(s.ClientID, Context{}, env)
	if err != nil {
		return Client{}, err
	}
	out := Client{ID: id}
	if s.ClientSecret != "" {
		if out.Secret, err = render(s.ClientSecret, Context{}, env); err != nil {
			return Client{}, err
		}
	}
	return out, nil
}

// SignInRequest is a POST of form to the sign-in's token address (or another address on its
// host, the revocation's), carrying the client's ID and secret as the recipe says.
func SignInRequest(s SignIn, client Client, to string, form url.Values) TokenRequest {
	s = s.withDefaults()
	f := url.Values{}
	for k, v := range form {
		f[k] = v
	}
	h := http.Header{}
	switch {
	case s.ClientAuth == ClientAuthBasic && client.Secret != "":
		// RFC 6749 §2.3.1: each form-encoded, then joined for Basic.
		h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(client.ID)+":"+url.QueryEscape(client.Secret))))
	default:
		f.Set("client_id", client.ID)
		if client.Secret != "" {
			f.Set("client_secret", client.Secret)
		}
	}
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	h.Set("Accept", "application/json")
	return TokenRequest{Method: http.MethodPost, URL: to, Headers: h, Body: []byte(f.Encode())}
}

// ExchangeForm is the token request's form that turns a code into a sign-in.
func ExchangeForm(code, redirect, verifier string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "code_verifier": {verifier}}
}

// RenewForm is the token request's form that renews a sign-in.
func RenewForm(refresh string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}
}

// RevokeForm asks the provider to cancel a refresh token (RFC 7009).
func RevokeForm(refresh string) url.Values {
	return url.Values{"token": {refresh}, "token_type_hint": {"refresh_token"}}
}

// Signed is what a token address answered for a sign-in: the access token, the refresh token
// when it gave one, the access token's lifetime in seconds (0 when it gave none) and the scopes
// it granted when it named them.
type Signed struct {
	Access    string
	Refresh   string
	ExpiresIn int
	Scopes    []string
}

// ErrNoAccessToken is a token answer without an access token.
var ErrNoAccessToken = errors.New("the answer has no access token")

// ReadSignIn reads a token address's JSON answer (RFC 6749 §5.1).
func ReadSignIn(body []byte) (Signed, error) {
	var doc struct {
		Access    string          `json:"access_token"`
		Refresh   string          `json:"refresh_token"`
		ExpiresIn json.RawMessage `json:"expires_in"`
		Scope     string          `json:"scope"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Signed{}, errors.New("the answer is not JSON")
	}
	if doc.Access == "" || strings.ContainsAny(doc.Access, "\r\n") || strings.ContainsAny(doc.Refresh, "\r\n") {
		return Signed{}, ErrNoAccessToken
	}
	out := Signed{Access: doc.Access, Refresh: doc.Refresh, Scopes: strings.Fields(doc.Scope)}
	var n float64
	if json.Unmarshal(doc.ExpiresIn, &n) != nil {
		var s string
		if json.Unmarshal(doc.ExpiresIn, &s) == nil {
			_, _ = fmt.Sscanf(s, "%g", &n)
		}
	}
	if n > 0 {
		out.ExpiresIn = int(min(n, 10*365*86400))
	}
	return out, nil
}

// Refused reports whether a token address's answer to a renewal means the provider no longer
// accepts the sign-in (RFC 6749 §5.2: 400 or 401, invalid_grant or invalid_client), as against
// a failure worth trying again.
func Refused(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusUnauthorized
}

// SignInSummary is what a sign-in asks of the person, for the grant screen.
type SignInSummary struct {
	Host string `json:"host"`
	// Label is the app's name for the system, or the host when it gives none.
	Label   string   `json:"label"`
	Scopes  []string `json:"scopes"`
	Revokes bool     `json:"revokes"`
}

func describeSignIn(s *SignIn) *SignInSummary {
	if s == nil {
		return nil
	}
	h := hostOf(s.Authorize)
	scopes := append([]string{}, s.Scopes...)
	label := s.Label
	if label == "" {
		label = h
	}
	return &SignInSummary{Host: h, Label: label, Scopes: scopes, Revokes: s.Revoke != ""}
}
