package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// Every name the CLI puts in an API path stays one path segment, whatever it holds: a slash,
// a dot segment, a query or a fragment in it cannot reach another endpoint. Values in the
// query stay one value.
func TestNaughtyPaths(t *testing.T) {
	var mu sync.Mutex
	var last *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		last = r
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "", "test")
	ctx := context.Background()
	const S = "\x00naughty"
	calls := []struct {
		want  string // the path with S where the input goes
		query map[string]string
		call  func(s string) error
	}{
		{"/v1/orgs/S/apps/S", nil, func(s string) error { _, err := c.GetApp(ctx, s, s); return err }},
		{"/v1/orgs/S/apps/S", nil, func(s string) error { return c.DeleteApp(ctx, s, s) }},
		{"/v1/orgs/S/apps", nil, func(s string) error { _, err := c.CreateApp(ctx, s, CreateAppRequest{Slug: s}); return err }},
		{"/v1/orgs/S/apps/S/deploys/S", nil, func(s string) error { _, err := c.GetDeploy(ctx, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/functions/S/graph", nil, func(s string) error { _, err := c.FunctionGraph(ctx, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/runs/S", nil, func(s string) error { _, err := c.GetRun(ctx, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/runs/S/replay", nil, func(s string) error { _, err := c.ReplayRun(ctx, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/cron/S/run", nil, func(s string) error { _, err := c.RunCron(ctx, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/runs", map[string]string{"event": S}, func(s string) error { _, err := c.RunsOfEvent(ctx, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/approvals", map[string]string{"status": S}, func(s string) error { _, err := c.ListApprovals(ctx, s, s, s); return err }},
		{"/v1/orgs/S/secrets/S/versions", map[string]string{"app": S, "limit": "100"}, func(s string) error { _, err := c.SecretVersions(ctx, s, s, s); return err }},
		{"/v1/orgs/S/secrets/S", map[string]string{"app": S}, func(s string) error { return c.DeleteSecret(ctx, s, s, s) }},
		{"/v1/orgs/S/apps/S/webhooks/S/events/S/replay", nil, func(s string) error { _, err := c.ReplayWebhookEvent(ctx, s, s, s, s); return err }},
		{"/v1/orgs/S/apps/S/domains/S", nil, func(s string) error { return c.DeleteDomain(ctx, s, s, s) }},
		{"/v1/orgs/S/members/S", nil, func(s string) error { return c.RemoveMember(ctx, s, s) }},
		{"/v1/orgs/S/tokens/S", nil, func(s string) error { return c.RevokeToken(ctx, s, s) }},
		{"/v1/orgs/S/export/S", nil, func(s string) error { _, err := c.GetExport(ctx, s, s); return err }},
		{"/v1/feedback/S", nil, func(s string) error { _, err := c.ShowFeedback(ctx, s); return err }},
		{"/v1/orgs/S/apps/S/environments/S", nil, func(s string) error { return c.DeleteEnvironment(ctx, s, s, s) }},
		{"/v1/orgs/S/apps/S/customers/S", nil, func(s string) error { return c.RemoveCustomer(ctx, s, s, s) }},
	}
	for _, s := range naughty.Strings() {
		if len(s) > 4096 {
			continue // a URL that long is refused by servers before it is routed
		}
		for _, call := range calls {
			if err := call.call(s); err != nil {
				t.Errorf("%s with %q: %v", call.want, s, err)
				continue
			}
			mu.Lock()
			r := last
			mu.Unlock()
			got := strings.Split(r.URL.EscapedPath(), "/")
			want := strings.Split(call.want, "/")
			if len(got) != len(want) {
				t.Errorf("%s with %q went to %s", call.want, s, r.URL.EscapedPath())
				continue
			}
			for i, w := range want {
				seg, err := url.PathUnescape(got[i])
				if w == "S" {
					w = s
				}
				if err != nil || seg != w || (w == s && (got[i] == "." || got[i] == "..")) {
					t.Errorf("%s with %q: segment %d is %q", call.want, s, i, got[i])
				}
			}
			q := r.URL.Query()
			for k, v := range call.query {
				if s == "" && v == S {
					continue // an empty optional value is left out
				}
				if v == S {
					v = s
				}
				if len(q[k]) != 1 || q[k][0] != v {
					t.Errorf("%s with %q: query %s = %q", call.want, s, k, q[k])
				}
			}
		}
	}
}

// The error object is read from any body; one that is not the platform's becomes an error with
// a code and a message no longer than 200 characters plus the ellipsis, still valid UTF-8.
func TestNaughtyErrors(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, status := range []int{400, 404, 500, 502} {
			for _, body := range []string{s, `{"error":{"code":` + jsonQuote(s) + `,"message":` + jsonQuote(s) + `}}`} {
				err := parseError(status, []byte(body))
				e, ok := err.(*Error)
				if !ok || e.Code == "" || e.Status != status {
					t.Errorf("parseError(%d, %q) = %#v", status, body, err)
					continue
				}
				if utf8.ValidString(body) && !utf8.ValidString(e.Message) {
					t.Errorf("parseError(%d, %q) cut a character in half: %q", status, body, e.Message)
				}
				if len(e.Message) > len("HTTP 500: ")+200+len("…") && !strings.HasPrefix(body, `{"error"`) {
					t.Errorf("parseError(%d, %q) message is %d bytes", status, body, len(e.Message))
				}
			}
		}
		if k := c0().IdempotencyKey("POST", s, []byte(s)); len(k) != 32 {
			t.Errorf("IdempotencyKey(%q) = %q", s, k)
		}
	}
}

// A redirect to a naughty location never carries the token off the host and never goes from
// https down to http.
func TestNaughtyRedirect(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, loc := range []string{s, "http://" + s + "/x", "https://" + s + "/x", "https://api.whisk.run/" + s} {
			u, err := url.Parse(loc)
			if err != nil {
				continue
			}
			first := &http.Request{URL: &url.URL{Scheme: "https", Host: "api.whisk.run", Path: "/v1"}, Header: http.Header{}}
			next := &http.Request{URL: first.URL.ResolveReference(u), Header: http.Header{"Authorization": {"Bearer t"}}}
			err = SafeRedirect(next, []*http.Request{first})
			if err == nil && next.URL.Scheme != "https" {
				t.Errorf("redirect to %q went down to %s", loc, next.URL.Scheme)
			}
			if err == nil && !strings.EqualFold(next.URL.Host, "api.whisk.run") && next.Header.Get("Authorization") != "" {
				t.Errorf("redirect to %q carried the token to %q", loc, next.URL.Host)
			}
		}
	}
}

func c0() *Client { return &Client{nonce: "n", HTTP: &http.Client{Timeout: time.Second}} }

func jsonQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`, "\x00", `\u0000`).Replace(s) + `"`
}
