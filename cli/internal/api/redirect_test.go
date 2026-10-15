package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/contract/tokensig"
)

// A redirect never downgrades to http and never carries the token to another host.
func TestSafeRedirect(t *testing.T) {
	var seen string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(204)
	}))
	defer other.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/elsewhere", http.StatusFound)
	}))
	defer api.Close()
	c := New(api.URL, "whsk_agent_secret", "", "dev")
	req, _ := http.NewRequest(http.MethodGet, api.URL+"/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer whsk_agent_secret")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if seen != "" {
		t.Fatalf("the token reached another host: %q", seen)
	}

	mk := func(raw string) *http.Request { r, _ := http.NewRequest(http.MethodGet, raw, nil); return r }
	if err := SafeRedirect(mk("http://api.whisk.run/x"), []*http.Request{mk("https://api.whisk.run/v1")}); err == nil {
		t.Fatal("an https to http redirect was followed")
	}
	if err := SafeRedirect(mk("https://api.whisk.run/x"), []*http.Request{mk("https://api.whisk.run/v1")}); err != nil {
		t.Fatalf("a same-host https redirect was refused: %v", err)
	}
}

// The hop limit, the downgrade and the host check, one at a time.
func TestSafeRedirectRules(t *testing.T) {
	mk := func(raw string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		r.Header.Set("Authorization", "Bearer whsk_agent_secret")
		r.Header.Set(tokensig.Header, "sig")
		return r
	}
	hops := func(n int, first string) []*http.Request {
		via := []*http.Request{mk(first)}
		for len(via) < n {
			via = append(via, mk("https://api.whisk.run/hop"))
		}
		return via
	}
	cases := []struct {
		name      string
		to        string
		via       []*http.Request
		ok        bool
		keepsAuth bool
	}{
		{"nine hops", "https://api.whisk.run/x", hops(9, "https://api.whisk.run/v1"), true, true},
		{"ten hops", "https://api.whisk.run/x", hops(10, "https://api.whisk.run/v1"), false, true},
		{"https to http", "http://api.whisk.run/x", hops(1, "https://api.whisk.run/v1"), false, true},
		{"http to https", "https://api.whisk.run/x", hops(1, "http://api.whisk.run/v1"), true, true},
		{"http to http", "http://localhost:8080/x", hops(1, "http://localhost:8080/v1"), true, true},
		{"same host in another case", "https://API.whisk.run/x", hops(1, "https://api.whisk.run/v1"), true, true},
		{"another host", "https://evil.example/x", hops(1, "https://api.whisk.run/v1"), true, false},
		{"a subdomain", "https://evil.api.whisk.run/x", hops(1, "https://api.whisk.run/v1"), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mk(tc.to)
			err := SafeRedirect(req, tc.via)
			if (err == nil) != tc.ok {
				t.Fatalf("SafeRedirect = %v, want ok=%v", err, tc.ok)
			}
			if !tc.ok {
				return
			}
			kept := req.Header.Get("Authorization") != "" || req.Header.Get(tokensig.Header) != ""
			if kept != tc.keepsAuth {
				t.Fatalf("credentials kept = %v, want %v", kept, tc.keepsAuth)
			}
		})
	}
}

// A stream (logs, deploy events, a build log) follows redirects under the same policy as every
// other request: its credentials never reach another host.
func TestStreamsKeepTheRedirectPolicy(t *testing.T) {
	var auth, sig string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, sig = r.Header.Get("Authorization"), r.Header.Get(tokensig.Header)
		w.Header().Set("Content-Type", "text/event-stream")
	}))
	defer other.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/elsewhere", http.StatusFound)
	}))
	defer api.Close()
	_, priv, _ := tokensig.NewKey()
	c := New(api.URL, "whsk_agent_secret", "", "dev")
	c.Key, _ = tokensig.ParsePrivate(priv)
	resp, err := c.open(context.Background(), "/orgs/acme/apps/crm/logs", "text/event-stream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if auth != "" || sig != "" {
		t.Fatalf("the stream's credentials reached another host: %q %q", auth, sig)
	}
}
