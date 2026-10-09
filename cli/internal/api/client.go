// Package api is the thin client of the control plane API (CONTROL-PLANE.md §5): bearer
// token, a Whisk-Signature on every request when the token is bound to this computer,
// idempotency keys on every mutating call, the error object as a typed error, and the few
// resources the CLI's offline half and account commands need.
package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	// nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- retry jitter, not a secret
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/routes"
	"github.com/whisk-run/contract/run/runhttp"
	"github.com/whisk-run/contract/tokensig"
)

// Client talks to one API origin with one token.
type Client struct {
	Base  string // https://api.whisk.run
	Token string
	// Key is the private key of this computer when Token is bound to it (whisk login); every
	// request is then signed with it (CONTROL-PLANE.md §4.4). Nil sends Token as a plain bearer.
	Key       ed25519.PrivateKey
	UserAgent string
	HTTP      *http.Client
	// OnCLIVersion receives the X-Whisk-CLI-Version header, the CLI the platform serves, from
	// every response that carries it.
	OnCLIVersion func(string)
	nonce        string
	// sleep, streamIdle and jitter replace the real wait, the 45 s stream idle deadline and
	// the random spread in tests; zero values are the real ones.
	sleep      func(context.Context, time.Duration) error
	streamIdle time.Duration
	jitter     func() float64
}

// New builds a client. agent is WHISK_AGENT (may be empty); version is the CLI version.
func New(base, token, agent, version string) *Client {
	ua := fmt.Sprintf("whisk-cli/%s (%s", version, runtime.GOOS)
	if agent != "" {
		ua += "; agent=" + agent
	}
	ua += ")"
	return &Client{
		Base:      strings.TrimRight(base, "/"),
		Token:     token,
		UserAgent: ua,
		HTTP:      httpClient(),
		nonce:     randomHex(8),
	}
}

// httpClient is the client for every request whose answer is read whole: a minute covers the
// slowest of them.
func httpClient() *http.Client {
	c := runhttp.Client(60 * time.Second)
	c.CheckRedirect = SafeRedirect
	return c
}

// streamFirstByte is how long a stream may take to start answering; once it has, the context
// bounds it. A build log's headers go out with its first line, which a build still pulling
// its base image can take minutes to write.
const streamFirstByte = 5 * time.Minute

// SafeRedirect is the redirect policy for every request the CLI makes: never from https down to
// http, at most ten hops, and the token is dropped whenever a hop leaves the original host, so a
// redirect cannot carry it anywhere the CLI was not pointed at.
func SafeRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing a redirect from https to %s", req.URL.Scheme)
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		req.Header.Del("Authorization")
		req.Header.Del(tokensig.Header)
	}
	return nil
}

// pathSeg escapes a name for one segment of an API path. "." and ".." are escaped as well,
// because a client or server that resolves dot segments would otherwise climb out of the
// segment to another endpoint.
func pathSeg(s string) string {
	switch s {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	}
	return url.PathEscape(s)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Error is the platform's error object (CONTROL-PLANE.md §9) as a Go error.
type Error struct {
	Status  int
	Code    string
	Message string
	Fix     string
	Docs    string
	Details map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Unavailable wraps a transport failure: the platform could not be reached at all.
type Unavailable struct{ Err error }

func (u *Unavailable) Error() string { return "platform unavailable: " + u.Err.Error() }
func (u *Unavailable) Unwrap() error { return u.Err }

// IdempotencyKey derives the key for a mutating call from what the call is and this
// invocation's nonce, so a retry of the same command sends the same key and a new command a
// new one.
func (c *Client) IdempotencyKey(method, path string, body []byte) string {
	sum := sha256.Sum256([]byte(c.nonce + "\n" + method + " " + path + "\n" + string(body)))
	return hex.EncodeToString(sum[:16])
}

// Do performs one JSON request. out may be nil. A non-2xx answer is returned as *Error; a
// transport failure as *Unavailable.
func (c *Client) Do(ctx context.Context, method, path string, in any, out any) error {
	_, err := c.do(ctx, method, path, in, out)
	return err
}

// authorize sets the bearer token and, when the token is bound to this computer, the signature
// over the request as it will be sent: method, request URI, body and token.
func (c *Client) authorize(req *http.Request, body []byte) {
	if c.Token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if c.Key != nil {
		req.Header.Set(tokensig.Header, tokensig.Sign(c.Key, req.Method, req.URL.RequestURI(), time.Now(), body, c.Token))
	}
}

// do performs a JSON request, trying a safe one again (retryWait) when the platform could not
// be reached or a gateway answered 502, 503 or 504. A write to a no-replay route
// (routes.NoReplay) carries no Idempotency-Key, because the platform would ignore it and run
// the write again, so it is never retried: a second db/query or token would be a second write. Each attempt has the HTTP client's deadline.
func (c *Client) do(ctx context.Context, method, path string, in any, out any) (int, error) {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return 0, err
		}
	}
	key := ""
	if method != http.MethodGet && method != http.MethodHead && !routes.NoReplay(path) {
		key = c.IdempotencyKey(method, path, body)
	}
	safe := method == http.MethodGet || method == http.MethodHead || key != ""
	for attempt := 1; ; attempt++ {
		status, retryAfter, raw, err := c.once(ctx, method, path, body, in != nil, key)
		if err == nil && status < 300 {
			return status, decodeBody(method, path, status, raw, out)
		}
		if err == nil {
			err = parseError(status, raw)
		}
		failed := status
		var u *Unavailable
		if errors.As(err, &u) {
			failed = 0
		}
		wait, again := retryWait(attempt, safe, failed, retryAfter, c.random(), time.Now())
		if !again || ctx.Err() != nil || c.pause(ctx, wait) != nil {
			return status, err
		}
	}
}

// once makes one attempt: the status, the Retry-After header, and the body. A transport
// failure, or a body cut off on the way, is *Unavailable.
func (c *Client) once(ctx context.Context, method, path string, body []byte, hasBody bool, key string) (int, string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Base+"/v1"+path, bytes.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	c.authorize(req, body)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, "", nil, &Unavailable{Err: err}
	}
	defer resp.Body.Close()
	c.noteCLIVersion(resp)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, resp.Header.Get("Retry-After"), raw, &Unavailable{Err: err}
	}
	return resp.StatusCode, resp.Header.Get("Retry-After"), raw, nil
}

// decodeBody reads a 2xx answer into out. An answer that is not JSON did not come from the
// API (a proxy's page, a captive portal), so it is PLATFORM_UNAVAILABLE, not a CLI fault.
func decodeBody(method, path string, status int, raw []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &Error{Status: status, Code: "PLATFORM_UNAVAILABLE",
			Message: fmt.Sprintf("%s %s answered HTTP %d with a body that is not the API's JSON: %v.", method, path, status, err),
			Fix:     "Retry with backoff. If it persists, check that WHISK_API points at the Whisk API and that nothing between you and it (a proxy, a captive portal) answers instead; status is at https://whisk.run/status."}
	}
	return nil
}

// random is the jitter for one wait, in [0,1).
func (c *Client) random() float64 {
	if c.jitter != nil {
		return c.jitter()
	}
	return mathrand.Float64()
}

// CLIVersionHeader names the CLI version the platform serves under /dl/.
const CLIVersionHeader = "X-Whisk-CLI-Version"

func (c *Client) noteCLIVersion(resp *http.Response) {
	if v := strings.TrimSpace(resp.Header.Get(CLIVersionHeader)); v != "" && c.OnCLIVersion != nil {
		c.OnCLIVersion(v)
	}
}

func parseError(status int, raw []byte) error {
	var body struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Fix     string         `json:"fix"`
			Docs    string         `json:"docs"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err == nil && body.Error.Code != "" {
		return &Error{Status: status, Code: body.Error.Code, Message: body.Error.Message, Fix: body.Error.Fix, Docs: body.Error.Docs, Details: body.Error.Details}
	}
	code := "PLATFORM_UNAVAILABLE"
	fix := "Retry with backoff; if it persists, check https://whisk.run/status."
	if status < 500 {
		code = "INVALID_REQUEST"
		fix = "The platform answered without its error object; report the status and body."
	}
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 200 {
		// Cut on a character boundary so the message stays valid UTF-8.
		cut := 200
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut] + "…"
	}
	return &Error{Status: status, Code: code, Message: fmt.Sprintf("HTTP %d: %s", status, msg), Fix: fix}
}

// IsCode reports whether err is a platform error with the given code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Device-code flow, CONTROL-PLANE.md §4.5.

// RequestDeviceCode starts the flow.
func (c *Client) RequestDeviceCode(ctx context.Context, in DeviceCodeRequest) (DeviceCode, error) {
	var out DeviceCode
	err := c.Do(ctx, http.MethodPost, "/device/code", in, &out)
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return out, err
}

// PollDeviceToken asks once whether the code was approved.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (DeviceToken, error) {
	var out DeviceToken
	status, err := c.do(ctx, http.MethodPost, "/device/token", map[string]string{"device_code": deviceCode}, &out)
	if err != nil {
		return out, err
	}
	if status == http.StatusAccepted || out.Status == "pending" {
		out.Status = "pending"
	} else {
		out.Status = "approved"
	}
	return out, nil
}

// Whoami describes the current token.
func (c *Client) Whoami(ctx context.Context) (Whoami, error) {
	var out Whoami
	return out, c.Do(ctx, http.MethodGet, "/whoami", nil, &out)
}

// GitPassword trades the bound token for a short git password (CONTROL-PLANE.md §6.3): git
// cannot sign a request, so it is handed this instead of the token.
func (c *Client) GitPassword(ctx context.Context) (GitPassword, error) {
	var out GitPassword
	return out, c.Do(ctx, http.MethodPost, "/tokens/git", nil, &out)
}

// RevokeSelf revokes the token in use (DELETE /tokens/self).
func (c *Client) RevokeSelf(ctx context.Context) error {
	return c.Do(ctx, http.MethodDelete, "/tokens/self", nil, nil)
}

// CreateApp creates an app: its repository and hostname come back.
func (c *Client) CreateApp(ctx context.Context, org string, req CreateAppRequest) (App, error) {
	var out App
	return out, c.Do(ctx, http.MethodPost, "/orgs/"+pathSeg(org)+"/apps", req, &out)
}

// getPage fetches one page of a list. path may already carry a query string.
func getPage[T any](ctx context.Context, c *Client, path string, cursor string, limit int) (Page[T], error) {
	var p Page[T]
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	full := path
	if enc := q.Encode(); enc != "" {
		if strings.Contains(path, "?") {
			full += "&" + enc
		} else {
			full += "?" + enc
		}
	}
	err := c.Do(ctx, http.MethodGet, full, nil, &p)
	if p.Items == nil {
		p.Items = []T{}
	}
	return p, err
}

// listAll follows pagination to the end.
func listAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	all := []T{}
	cursor := ""
	for {
		p, err := getPage[T](ctx, c, path, cursor, 100)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Items...)
		if p.NextCursor == "" {
			return all, nil
		}
		cursor = p.NextCursor
	}
}

// open performs a request whose body is read by the caller: a stream (SSE, a build log).
// The answer must start within streamFirstByte; after that no overall timeout applies: the
// context bounds it, and stream adds an idle deadline. Redirects follow the client's policy
// (SafeRedirect) like every other request. A non-2xx answer is returned as *Error.
func (c *Client) open(ctx context.Context, path, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.UserAgent)
	c.authorize(req, nil)
	streaming := runhttp.StreamClient(streamFirstByte)
	streaming.CheckRedirect = c.HTTP.CheckRedirect
	resp, err := streaming.Do(req)
	if err != nil {
		return nil, &Unavailable{Err: err}
	}
	c.noteCLIVersion(resp)
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, parseError(resp.StatusCode, raw)
	}
	return resp, nil
}

// ListApps lists every app in the org, following pagination.
func (c *Client) ListApps(ctx context.Context, org string) ([]App, error) {
	return listAll[App](ctx, c, "/orgs/"+pathSeg(org)+"/apps")
}

// GetApp fetches one app.
func (c *Client) GetApp(ctx context.Context, org, app string) (App, error) {
	var out App
	return out, c.Do(ctx, http.MethodGet, "/orgs/"+pathSeg(org)+"/apps/"+pathSeg(app), nil, &out)
}

// DeleteApp deletes an app.
func (c *Client) DeleteApp(ctx context.Context, org, app string) error {
	return c.Do(ctx, http.MethodDelete, "/orgs/"+pathSeg(org)+"/apps/"+pathSeg(app), nil, nil)
}

// DeletedApp is an app a person deleted that can still be restored, until ReleaseAt
// (CONTROL-PLANE.md §6.5).
type DeletedApp = apitypes.DeletedApp

// ListDeletedApps lists the org's deleted apps that can still be restored.
func (c *Client) ListDeletedApps(ctx context.Context, org string) ([]DeletedApp, error) {
	return listAll[DeletedApp](ctx, c, "/orgs/"+pathSeg(org)+"/deleted-apps")
}

// RestoreDeletedApp brings a deleted app back, stopped, by its id.
func (c *Client) RestoreDeletedApp(ctx context.Context, org, id string) (App, error) {
	var out App
	return out, c.Do(ctx, http.MethodPost, "/orgs/"+pathSeg(org)+"/deleted-apps/"+pathSeg(id)+"/restore", nil, &out)
}

// Validate sends the manifest for the platform's view of it.
func (c *Client) Validate(ctx context.Context, org, app string, manifestYAML []byte) (Validation, error) {
	var out Validation
	return out, c.Do(ctx, http.MethodPost, "/orgs/"+pathSeg(org)+"/apps/"+pathSeg(app)+"/validate", map[string]string{"manifest": string(manifestYAML)}, &out)
}
