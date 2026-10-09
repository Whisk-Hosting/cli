package webhook

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

var verdictReasons = map[string]bool{"": true, "missing_signature": true, "signature_mismatch": true, "missing_timestamp": true, "bad_timestamp": true, "stale_timestamp": true, "missing_id": true, "no_secret": true}

// naughtyRequest is a delivery to preset p signed with secret, with s as its body, URL and
// method, and the timestamp and id headers the preset reads.
func naughtyRequest(p Preset, s string, now time.Time) (Request, string, string) {
	ts := strconv.FormatInt(now.Unix(), 10)
	if p.TimestampFormat == "unix_ms" {
		ts = strconv.FormatInt(now.UnixMilli(), 10)
	}
	h := http.Header{}
	if p.Timestamp != nil && p.Timestamp.From == "header_name" {
		h.Set(p.Timestamp.Name, ts)
	}
	if p.ID != nil && p.ID.From == "header_name" {
		h.Set(p.ID.Name, "msg_1")
	}
	return Request{Method: "POST", URL: "https://hooks.whisk.run/acme/app/src?q=" + s, Header: h, Body: []byte(s)}, ts, "msg_1"
}

// A naughty body, URL or secret signed by the provider verifies, the same request with the
// wrong secret does not, and a naughty signature header never verifies.
func TestNaughtyVerify(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for name, p := range Presets() {
		if name == PresetHMAC || name == PresetToken {
			continue
		}
		for _, s := range naughty.Strings() {
			req, ts, id := naughtyRequest(p, s, now)
			req.Header.Set(p.Header, Sign(p, "whsec_test", req, ts, id))
			if v := Verify(p, "whsec_test", req, now); !v.Verified {
				t.Errorf("%s: body %q signed but not verified: %+v", name, s, v)
			}
			if v := Verify(p, "whsec_other", req, now); v.Verified || v.Reason != "signature_mismatch" {
				t.Errorf("%s: body %q verified with the wrong secret: %+v", name, s, v)
			}
			if s != "" {
				signed := req.Header.Get(p.Header)
				if v := Verify(p, s, req, now); v.Verified {
					t.Errorf("%s: naughty secret %q verified a delivery signed with another", name, s)
				}
				req.Header.Set(p.Header, Sign(p, s, req, ts, id))
				if v := Verify(p, s, req, now); !v.Verified {
					t.Errorf("%s: naughty secret %q did not verify its own signature: %+v", name, s, v)
				}
				req.Header.Set(p.Header, signed)
			}

			forged := Request{Method: s, URL: s, Header: http.Header{}, Body: []byte(s)}
			for k, v := range req.Header {
				forged.Header[k] = v
			}
			forged.Header[http.CanonicalHeaderKey(p.Header)] = []string{s}
			if p.Timestamp != nil && p.Timestamp.From == "header_name" {
				forged.Header[http.CanonicalHeaderKey(p.Timestamp.Name)] = []string{s}
			}
			v := Verify(p, "whsec_test", forged, now)
			if v.Verified || !verdictReasons[v.Reason] {
				t.Errorf("%s: forged header %q gave %+v", name, s, v)
			}
		}
	}
}

// Inline hmac settings with a naughty field either fail Check or verify only what they sign;
// a preset that does not check never verifies anything.
func TestNaughtyPresetCheck(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for _, s := range naughty.Strings() {
		presets := []Preset{
			{Header: s, Algorithm: "sha256", Encoding: "hex", Payload: "{body}"},
			{Header: "X-Sig", Algorithm: s, Encoding: "hex", Payload: "{body}"},
			{Header: "X-Sig", Algorithm: s, Encoding: "hex", Payload: "{body}", SignaturePrefix: "v1="},
			{Header: "X-Sig", Algorithm: s, Encoding: "base64", Payload: "{body}", SignaturePrefix: "sha256="},
			{Header: "X-Sig", Algorithm: "sha256", Encoding: s, Payload: "{body}"},
			{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: s},
			{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: "{body}", SignaturePattern: s},
			{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: "{body}", SignaturePrefix: s},
			{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: "{timestamp}.{body}", Timestamp: &HeaderSource{From: "header", Pattern: s}},
			{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: "{body}", TimestampFormat: s},
		}
		for i, p := range presets {
			if p.Check() != nil {
				// Verify never compiles a pattern that did not check, and an unknown algorithm or
				// encoding must not verify an empty signature against an empty MAC.
				if p.SignaturePattern == "" && (p.Timestamp == nil || p.Timestamp.Pattern == "") {
					req := Request{Method: "POST", URL: "https://x", Header: http.Header{"X-Sig": {"v1="}}, Body: []byte(s)}
					if p.Header != "" {
						req.Header.Set(p.Header, p.SignaturePrefix)
					}
					if v := Verify(p, "k", req, now); v.Verified {
						t.Errorf("preset %d with %q does not check yet verified an empty signature", i, s)
					}
				}
				continue
			}
			req := Request{Method: "POST", URL: "https://hooks.whisk.run/x", Header: http.Header{}, Body: []byte(s)}
			if p.Timestamp != nil {
				continue // a pattern that compiles may still find no timestamp in a signature it did not write
			}
			req.Header.Set(p.Header, Sign(p, "k", req, "", ""))
			if v := Verify(p, "k", req, now); !v.Verified && p.SignaturePattern == "" && p.SignaturePrefix == "" {
				t.Errorf("preset %d with %q checks but does not verify its own signature: %+v", i, s, v)
			}
			if v := Verify(p, "other", req, now); v.Verified {
				t.Errorf("preset %d with %q verified with the wrong secret", i, s)
			}
		}
		for _, doc := range []string{s, "x:\n  header: " + s + "\n  algorithm: sha256\n  encoding: hex\n  payload: \"{body}\"\n"} {
			loaded, err := Load([]byte(doc))
			if err != nil {
				continue
			}
			for name, p := range loaded {
				if name != PresetHMAC && name != PresetToken && p.Check() != nil {
					t.Errorf("Load(%q) returned preset %q that does not check", doc, name)
				}
			}
		}
	}
}

// The platform's own delivery signature: a naughty id, time or body signed is verified, and
// any naughty header in its place is refused with a stated reason.
func TestNaughtyDelivery(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	reasons := map[string]bool{"no_key": true, "service_app": true, "missing": true, "mismatch": true}
	delivery := "01J8Z3WQ9XN4M6P8R0T2V4X6Y8"
	for _, s := range naughty.Strings() {
		h := http.Header{}
		h.Set("X-Whisk-Webhook-Id", delivery)
		h.Set("X-Whisk-Webhook-Received-At", "2026-09-23T10:15:30Z")
		h.Set(DeliverySignatureHeader, DeliverySignature(key, delivery, "2026-09-23T10:15:30Z", []byte(s)))
		if why := VerifyDelivery(key, h, []byte(s)); why != "" {
			t.Errorf("body %q signed but refused: %s", s, why)
		}
		if why := VerifyDelivery(key, h, []byte(s+"x")); why != "mismatch" {
			t.Errorf("body %q changed but answered %q", s, why)
		}
		if s != "" {
			h2 := http.Header{"X-Whisk-Webhook-Id": {s}, "X-Whisk-Webhook-Received-At": {s}}
			h2[DeliverySignatureHeader] = []string{DeliverySignature(key, s, s, []byte(s))}
			if why := VerifyDelivery(key, h2, []byte(s)); why != "" {
				t.Errorf("id and time %q signed but refused: %s", s, why)
			}
		}
		h3 := http.Header{"X-Whisk-Webhook-Id": {s}, "X-Whisk-Webhook-Received-At": {s}, DeliverySignatureHeader: {s}, "X-Whisk-Service-App": {s}}
		if why := VerifyDelivery(key, h3, []byte(s)); why == "" || !reasons[why] {
			t.Errorf("forged headers %q answered %q", s, why)
		}
		if why := VerifyDelivery([]byte(s), h3, []byte(s)); why == "" || !reasons[why] {
			t.Errorf("key %q with forged headers answered %q", s, why)
		}
	}
}
