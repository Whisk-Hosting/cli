package tokensig

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

// A request with a naughty method, URI, body or token verifies exactly when it is the request
// that was signed; a naughty header is refused with one of the stated reasons.
func TestNaughtySignatures(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1788782400, 0)
	reasons := []error{ErrMissing, ErrMalformed, ErrStale, ErrBad}
	known := func(err error) bool {
		for _, r := range reasons {
			if errors.Is(err, r) {
				return true
			}
		}
		return false
	}
	for _, s := range naughty.Strings() {
		h := Sign(priv, "POST", "/v1/orgs/"+s, now, []byte(s), s)
		if err := Verify(pub, h, "POST", "/v1/orgs/"+s, []byte(s), s, now); err != nil {
			t.Errorf("signed request with %q refused: %v", s, err)
		}
		if err := Verify(pub, h, "POST", "/v1/orgs/"+s+"x", []byte(s), s, now); !errors.Is(err, ErrBad) {
			t.Errorf("changed URI with %q answered %v", s, err)
		}
		if err := Verify(pub, h, "POST", "/v1/orgs/"+s, []byte(s+"x"), s, now); !errors.Is(err, ErrBad) {
			t.Errorf("changed body with %q answered %v", s, err)
		}
		if err := Verify(pub, h, "POST", "/v1/orgs/"+s, []byte(s), s+"x", now); !errors.Is(err, ErrBad) {
			t.Errorf("changed token with %q answered %v", s, err)
		}
		if s != "" {
			m := Sign(priv, s, "/", now, nil, "t")
			if err := Verify(pub, m, s, "/", nil, "t", now); err != nil {
				t.Errorf("signed method %q refused: %v", s, err)
			}
		}
		for _, header := range []string{s, "t=" + s + ", sig=" + s, "t=1788782400, sig=" + s, "t=" + s, h + "," + s} {
			if err := Verify(pub, header, "POST", "/", nil, "t", now); err == nil || !known(err) {
				t.Errorf("header %q answered %v", header, err)
			}
			if ts, sig, err := Parse(header); err == nil && len(sig) != ed25519.SignatureSize {
				t.Errorf("Parse(%q) accepted a %d-byte signature at %d", header, len(sig), ts)
			}
		}
		if _, err := ParsePublic(s); err == nil {
			t.Errorf("ParsePublic accepted %q", s)
		}
		if _, err := ParsePrivate(s); err == nil {
			t.Errorf("ParsePrivate accepted %q", s)
		}
		if err := Verify(ed25519.PublicKey(s), h, "POST", "/v1/orgs/"+s, []byte(s), s, now); !errors.Is(err, ErrBad) {
			t.Errorf("public key %q answered %v", s, err)
		}
	}
}
