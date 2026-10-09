package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// recorder answers every request with status and body and keeps what it was sent.
type recorder struct {
	status int
	body   string
	got    []*http.Request
}

func (r *recorder) serve(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.got = append(r.got, req)
		w.WriteHeader(r.status)
		_, _ = w.Write([]byte(r.body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "whsk_agent_x", "claude", "test")
}

// A mutating call carries an Idempotency-Key so a retry is not done twice; a read and a poll
// (whose first answer must not be replayed) carry none.
func TestIdempotencyKeyOnlyOnMutations(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{http.MethodPost, "/orgs/acme/apps", true},
		{http.MethodDelete, "/orgs/acme/apps/crm", true},
		{http.MethodGet, "/whoami", false},
		{http.MethodHead, "/whoami", false},
		{http.MethodPost, "/device/token", false},
		{http.MethodPost, "/tokens/git", false},
		{http.MethodPost, "/orgs/acme/apps/crm/db/query", false},
	}
	for _, tc := range cases {
		rec := &recorder{status: 204}
		c := rec.serve(t)
		if err := c.Do(context.Background(), tc.method, tc.path, nil, nil); err != nil {
			t.Fatal(err)
		}
		got := rec.got[0].Header.Get("Idempotency-Key")
		if (got != "") != tc.want {
			t.Errorf("%s %s: Idempotency-Key %q, want present=%v", tc.method, tc.path, got, tc.want)
		}
		if tc.want && got != c.IdempotencyKey(tc.method, tc.path, nil) {
			t.Errorf("%s %s: Idempotency-Key %q is not the derived key", tc.method, tc.path, got)
		}
		if ua := rec.got[0].Header.Get("User-Agent"); !strings.HasSuffix(ua, "; agent=claude)") {
			t.Errorf("User-Agent %q", ua)
		}
	}
	if New("http://x", "", "", "v1").UserAgent != "whisk-cli/v1 ("+runtime.GOOS+")" {
		t.Errorf("User-Agent without an agent: %q", New("http://x", "", "", "v1").UserAgent)
	}
}

// Every status from 300 up is an error with a stable code: the platform's own error object when
// it sent one, else INVALID_REQUEST below 500 and PLATFORM_UNAVAILABLE from 500.
func TestStatusesAndErrorCodes(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
	}{
		{200, `{}`, ""},
		{299, ``, ""},
		{300, `moved`, "INVALID_REQUEST"},
		{499, `nope`, "INVALID_REQUEST"},
		{500, `down`, "PLATFORM_UNAVAILABLE"},
		{409, `{"error":{"code":"SLUG_TAKEN","message":"m","fix":"f"}}`, "SLUG_TAKEN"},
		{502, `{"error":{"message":"no code"}}`, "PLATFORM_UNAVAILABLE"},
	}
	for _, tc := range cases {
		rec := &recorder{status: tc.status, body: tc.body}
		err := rec.serve(t).Do(context.Background(), http.MethodGet, "/whoami", nil, nil)
		var e *Error
		switch {
		case tc.code == "" && err != nil:
			t.Errorf("%d: %v", tc.status, err)
		case tc.code != "" && (!errors.As(err, &e) || e.Code != tc.code || e.Status != tc.status):
			t.Errorf("%d: %v, want %s", tc.status, err, tc.code)
		}
	}
}

// A body without the error object is quoted in the message, cut at 200 bytes on a character
// boundary.
func TestErrorMessageIsCut(t *testing.T) {
	cases := map[string]string{
		strings.Repeat("a", 200):        "HTTP 500: " + strings.Repeat("a", 200),
		strings.Repeat("a", 201):        "HTTP 500: " + strings.Repeat("a", 200) + "…",
		strings.Repeat("a", 199) + "éé": "HTTP 500: " + strings.Repeat("a", 199) + "…",
	}
	for body, want := range cases {
		var e *Error
		if !errors.As(parseError(500, []byte(body)), &e) || e.Message != want || !utf8.ValidString(e.Message) {
			t.Errorf("parseError(%d bytes) = %q, want %q", len(body), e.Message, want)
		}
	}
}

// A page asks for its limit and cursor, joined to a path that may already carry a query.
func TestGetPageQuery(t *testing.T) {
	cases := []struct {
		path, cursor string
		limit        int
		want         string
	}{
		{"/orgs/acme/apps", "", 0, "/v1/orgs/acme/apps"},
		{"/orgs/acme/apps", "", 1, "/v1/orgs/acme/apps?limit=1"},
		{"/orgs/acme/apps", "c2", 100, "/v1/orgs/acme/apps?cursor=c2&limit=100"},
		{"/orgs/acme/audit?actor=x", "c2", 0, "/v1/orgs/acme/audit?actor=x&cursor=c2"},
	}
	for _, tc := range cases {
		rec := &recorder{status: 200, body: `{"next_cursor":""}`}
		p, err := getPage[map[string]any](context.Background(), rec.serve(t), tc.path, tc.cursor, tc.limit)
		if err != nil || p.Items == nil {
			t.Fatalf("getPage: %+v %v", p, err)
		}
		if got := rec.got[0].URL.RequestURI(); got != tc.want {
			t.Errorf("getPage(%q, %q, %d) asked %q, want %q", tc.path, tc.cursor, tc.limit, got, tc.want)
		}
	}
}

// A stream that answers 300 or more is an error, not a body to read.
func TestStreamStatus(t *testing.T) {
	for status, ok := range map[int]bool{200: true, 299: true, 300: false, 404: false} {
		rec := &recorder{status: status, body: "x"}
		resp, err := rec.serve(t).open(context.Background(), "/orgs/acme/apps/crm/logs", "text/event-stream")
		if (err == nil) != ok {
			t.Errorf("open answered %d: %v, want ok=%v", status, err, ok)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
}
