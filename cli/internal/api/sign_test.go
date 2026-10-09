package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/whisk-run/contract/tokensig"
)

// With a key, every request carries a signature over exactly what was sent; without one (a
// WHISK_TOKEN, a token from before device keys) the token goes as a plain bearer.
func TestRequestsAreSignedWithTheKey(t *testing.T) {
	pubB64, privB64, _ := tokensig.NewKey()
	pub, _ := tokensig.ParsePublic(pubB64)
	priv, _ := tokensig.ParsePrivate(privB64)
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h := r.Header.Get(tokensig.Header)
		switch {
		case h == "":
			got = append(got, "plain")
		case tokensig.Verify(pub, h, r.Method, r.URL.RequestURI(), body, "whsk_agent_x", time.Now()) == nil:
			got = append(got, "signed")
		default:
			got = append(got, "bad")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"next_cursor":""}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "whsk_agent_x", "", "test")
	c.Key = priv
	ctx := context.Background()
	_ = c.Do(ctx, http.MethodPost, "/orgs/acme/apps", CreateAppRequest{Slug: "crm", Name: "CRM"}, nil)
	_, _ = c.ListApps(ctx, "acme")
	resp, err := c.open(ctx, "/orgs/acme/apps/crm/logs?follow=1", "text/event-stream")
	if err == nil {
		resp.Body.Close()
	}
	c.Key = nil
	_ = c.Do(ctx, http.MethodGet, "/whoami", nil, nil)

	want := []string{"signed", "signed", "signed", "plain"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
