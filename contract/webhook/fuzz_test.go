package webhook

import (
	"net/http"
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

// Fuzz targets for webhook verification (docs/HARNESS.md §8.3).

// FuzzVerify sends every provider preset a fuzzed delivery: a made-up signature header never
// verifies under a secret it was not made with, every refusal has a stated reason, and the same
// delivery signed by the provider verifies.
func FuzzVerify(f *testing.F) {
	f.Add("whsec_test", "POST", "https://hooks.whisk.run/a/b/c", []byte("{}"), "v1=00")
	for _, s := range naughty.Strings() {
		f.Add(s, s, s, []byte(s), s)
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	names := Names()
	f.Fuzz(func(t *testing.T, secret, method, url string, body []byte, sig string) {
		for _, name := range names {
			p := Presets()[name]
			if name == PresetHMAC || name == PresetToken {
				continue
			}
			req, ts, id := naughtyRequest(p, "", now)
			req.Method, req.URL, req.Body = method, url, body
			req.Header.Set(p.Header, sig)
			v := Verify(p, secret, req, now)
			if !verdictReasons[v.Reason] || v.Verified != (v.Reason == "") {
				t.Fatalf("%s: verdict %+v", name, v)
			}
			if v.Verified && Verify(p, secret+"x", req, now).Verified {
				t.Fatalf("%s: header %q verified under two secrets", name, sig)
			}
			if secret == "" {
				if v.Reason != "no_secret" {
					t.Fatalf("%s: no secret answered %+v", name, v)
				}
				continue
			}
			req.Header.Set(p.Header, Sign(p, secret, req, ts, id))
			if v := Verify(p, secret, req, now); !v.Verified {
				t.Fatalf("%s: own signature refused: %+v", name, v)
			}
			if v := Verify(p, secret+"x", req, now); v.Verified || v.Reason != "signature_mismatch" {
				t.Fatalf("%s: verified with the wrong secret: %+v", name, v)
			}
		}
	})
}

// FuzzLoad reads preset files: whatever Load accepts checks, and a preset that does not check
// never verifies a delivery.
func FuzzLoad(f *testing.F) {
	f.Add([]byte("x:\n  header: X-Sig\n  algorithm: sha256\n  encoding: hex\n  payload: \"{body}\"\n"))
	for _, s := range naughty.Strings() {
		f.Add([]byte(s))
		f.Add([]byte("x:\n  header: X-Sig\n  algorithm: sha256\n  encoding: hex\n  payload: \"" + s + "\"\n  signature_pattern: '" + s + "'\n"))
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, src []byte) {
		loaded, err := Load(src)
		if err != nil {
			return
		}
		for name, p := range loaded {
			if name == PresetHMAC || name == PresetToken {
				continue
			}
			if p.Check() != nil {
				t.Fatalf("Load returned %q that does not check", name)
			}
			req := Request{Method: "POST", URL: "https://x", Header: http.Header{}, Body: src}
			if p.Timestamp == nil && p.ID == nil {
				req.Header.Set(p.Header, Sign(p, "k", req, "", ""))
				if v := Verify(p, "other", req, now); v.Verified {
					t.Fatalf("%q verified with the wrong secret", name)
				}
			}
		}
	})
}

// FuzzVerifyDelivery: the platform's delivery signature verifies exactly what it signed.
func FuzzVerifyDelivery(f *testing.F) {
	f.Add([]byte("0123456789abcdef"), "01J8Z3WQ9XN4M6P8R0T2V4X6Y8", "2026-09-23T10:15:30Z", []byte("{}"), "v1=00")
	for _, s := range naughty.Strings() {
		f.Add([]byte(s), s, s, []byte(s), s)
	}
	reasons := map[string]bool{"no_key": true, "service_app": true, "missing": true, "mismatch": true}
	f.Fuzz(func(t *testing.T, key []byte, id, at string, body []byte, sig string) {
		h := http.Header{}
		h.Set("X-Whisk-Webhook-Id", id)
		h.Set("X-Whisk-Webhook-Received-At", at)
		h.Set(DeliverySignatureHeader, sig)
		why := VerifyDelivery(key, h, body)
		if why != "" && !reasons[why] {
			t.Fatalf("unstated reason %q", why)
		}
		if why == "" && sig != DeliverySignature(key, h.Get("X-Whisk-Webhook-Id"), h.Get("X-Whisk-Webhook-Received-At"), body) {
			t.Fatalf("accepted %q which is not the signature", sig)
		}
		h.Set(DeliverySignatureHeader, DeliverySignature(key, h.Get("X-Whisk-Webhook-Id"), h.Get("X-Whisk-Webhook-Received-At"), body))
		why = VerifyDelivery(key, h, body)
		want := ""
		switch {
		case len(key) == 0:
			want = "no_key"
		case h.Get("X-Whisk-Webhook-Id") == "" || h.Get("X-Whisk-Webhook-Received-At") == "":
			want = "missing"
		}
		if why != want {
			t.Fatalf("signed delivery answered %q, want %q", why, want)
		}
		if want == "" {
			if VerifyDelivery(key, h, append(append([]byte{}, body...), 'x')) != "mismatch" {
				t.Fatalf("changed body verified")
			}
		}
	})
}
