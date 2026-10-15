package tokensig

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

func TestVerify(t *testing.T) {
	pubB64, privB64, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublic(pubB64)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ParsePrivate(privB64)
	if err != nil {
		t.Fatal(err)
	}
	_, otherB64, _ := NewKey()
	other, _ := ParsePrivate(otherB64)

	now := time.Unix(1790000000, 0)
	const token = "whsk_agent_abc"
	body := []byte(`{"name":"crm"}`)
	good := Sign(priv, "POST", "/v1/orgs/acme/apps?x=1", now, body, token)

	cases := []struct {
		name    string
		header  string
		method  string
		uri     string
		body    []byte
		token   string
		now     time.Time
		wantErr error
	}{
		{"good", good, "POST", "/v1/orgs/acme/apps?x=1", body, token, now, nil},
		{"method case does not matter", good, "post", "/v1/orgs/acme/apps?x=1", body, token, now, nil},
		{"four minutes later", good, "POST", "/v1/orgs/acme/apps?x=1", body, token, now.Add(4 * time.Minute), nil},
		{"wrong key", Sign(other, "POST", "/v1/orgs/acme/apps?x=1", now, body, token), "POST", "/v1/orgs/acme/apps?x=1", body, token, now, ErrBad},
		{"exactly MaxSkew later", good, "POST", "/v1/orgs/acme/apps?x=1", body, token, now.Add(MaxSkew), nil},
		{"exactly MaxSkew early", good, "POST", "/v1/orgs/acme/apps?x=1", body, token, now.Add(-MaxSkew), nil},
		{"stale t", good, "POST", "/v1/orgs/acme/apps?x=1", body, token, now.Add(301 * time.Second), ErrStale},
		{"t from the future", good, "POST", "/v1/orgs/acme/apps?x=1", body, token, now.Add(-301 * time.Second), ErrStale},
		{"body changed", good, "POST", "/v1/orgs/acme/apps?x=1", []byte(`{"name":"crm2"}`), token, now, ErrBad},
		{"path changed", good, "POST", "/v1/orgs/other/apps?x=1", body, token, now, ErrBad},
		{"query changed", good, "POST", "/v1/orgs/acme/apps?x=2", body, token, now, ErrBad},
		{"method changed", good, "DELETE", "/v1/orgs/acme/apps?x=1", body, token, now, ErrBad},
		{"another token", good, "POST", "/v1/orgs/acme/apps?x=1", body, "whsk_agent_other", now, ErrBad},
		{"missing header", "", "POST", "/v1/orgs/acme/apps?x=1", body, token, now, ErrMissing},
		{"no sig", "t=1790000000", "POST", "/v1/orgs/acme/apps?x=1", body, token, now, ErrMalformed},
		{"garbage", "Bearer nonsense", "POST", "/v1/orgs/acme/apps?x=1", body, token, now, ErrMalformed},
		{"short sig", "t=1790000000, sig=AAAA", "POST", "/v1/orgs/acme/apps?x=1", body, token, now, ErrMalformed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Verify(pub, c.header, c.method, c.uri, c.body, c.token, c.now)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("got %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestEmptyBodyAndKeys(t *testing.T) {
	_, privB64, _ := NewKey()
	priv, _ := ParsePrivate(privB64)
	pub := priv.Public().(ed25519.PublicKey)
	now := time.Now()
	h := Sign(priv, "GET", "/v1/whoami", now, nil, "tok")
	if err := Verify(pub, h, "GET", "/v1/whoami", []byte{}, "tok", now); err != nil {
		t.Fatalf("nil and empty bodies differ: %v", err)
	}
	if _, err := ParsePublic("bm90IGEga2V5"); err == nil {
		t.Fatal("a short public key parsed")
	}
	if _, err := ParsePrivate("bm90IGEga2V5"); err == nil {
		t.Fatal("a short private key parsed")
	}
	if err := Verify(ed25519.PublicKey{1, 2}, h, "GET", "/v1/whoami", nil, "tok", now); !errors.Is(err, ErrBad) {
		t.Fatalf("a malformed stored key verified: %v", err)
	}
}
