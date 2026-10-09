package tokensig

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

// FuzzVerify (docs/HARNESS.md §8.3): any header verifies only when it is a signature over
// exactly this request made with the key, and every refusal is one of the stated reasons. A
// request signed by the key verifies whatever its method, URI, body and token.
func FuzzVerify(f *testing.F) {
	seed := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	now := time.Unix(1788782400, 0)
	f.Add(Sign(seed, "POST", "/v1/orgs", now, []byte("{}"), "tok"), "POST", "/v1/orgs", []byte("{}"), "tok", int64(0))
	for _, s := range naughty.Strings() {
		f.Add(s, s, s, []byte(s), s, int64(len(s)))
		f.Add("t=1788782400, sig="+s, "GET", "/"+s, []byte(nil), s, int64(-301))
	}
	pub := seed.Public().(ed25519.PublicKey)
	f.Fuzz(func(t *testing.T, header, method, uri string, body []byte, token string, skew int64) {
		at := now.Add(time.Duration(skew%3600) * time.Second)
		err := Verify(pub, header, method, uri, body, token, at)
		if err != nil && !errors.Is(err, ErrMissing) && !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrStale) && !errors.Is(err, ErrBad) {
			t.Fatalf("unstated reason %v for %q", err, header)
		}
		if ts, sig, perr := Parse(header); perr == nil {
			if len(sig) != ed25519.SignatureSize {
				t.Fatalf("Parse accepted a %d-byte signature", len(sig))
			}
			if err == nil && !ed25519.Verify(pub, Canonical(method, uri, ts, body, token), sig) {
				t.Fatalf("Verify accepted %q without a valid signature", header)
			}
		} else if err == nil {
			t.Fatalf("Verify accepted %q that does not parse", header)
		}
		signed := Sign(seed, method, uri, at, body, token)
		if err := Verify(pub, signed, method, uri, body, token, at); err != nil {
			t.Fatalf("own signature refused: %v", err)
		}
		if err := Verify(pub, signed, method, uri+"x", body, token, at); !errors.Is(err, ErrBad) {
			t.Fatalf("signature verified for another URI: %v", err)
		}
		if err := Verify(pub, signed, method, uri, body, token+"x", at); !errors.Is(err, ErrBad) {
			t.Fatalf("signature verified for another token: %v", err)
		}
		if err := Verify(pub, signed, method, uri, body, token, at.Add(MaxSkew+time.Second)); !errors.Is(err, ErrStale) {
			t.Fatalf("stale signature answered %v", err)
		}
		if k, err := ParsePublic(header); err == nil && len(k) != ed25519.PublicKeySize {
			t.Fatalf("ParsePublic accepted a %d-byte key", len(k))
		}
		if k, err := ParsePrivate(header); err == nil {
			if len(k) != ed25519.PrivateKeySize {
				t.Fatalf("ParsePrivate accepted a %d-byte key", len(k))
			}
			if p, err := ParsePublic(EncodePublic(k.Public().(ed25519.PublicKey))); err != nil || !p.Equal(k.Public()) {
				t.Fatalf("public key does not round trip: %v", err)
			}
		}
	})
}
