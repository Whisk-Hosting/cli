package webpush

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/whisk-run/contract/apitypes"
)

func code(p *Problem) string {
	if p == nil {
		return ""
	}
	return p.Code
}

func TestRecipients(t *testing.T) {
	many := make([]any, MaxRecipients+1)
	for i := range many {
		many[i] = strings.Repeat("u", i+1)[:1] + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
	}
	cases := []struct {
		name string
		in   any
		want []string
		ok   bool
	}{
		{"one person", "dana", []string{"dana"}, true},
		{"a list, without duplicates, in order", []any{"b", "a", "b"}, []string{"b", "a"}, true},
		{"nothing", nil, nil, false},
		{"an empty list", []any{}, nil, false},
		{"an empty key", "", nil, false},
		{"a blank key", "  ", nil, false},
		{"a number", []any{"a", 3}, nil, false},
		{"a control character", "a\nb", nil, false},
		{"too long", strings.Repeat("k", MaxUserKey+1), nil, false},
		{"too many", many, nil, false},
	}
	for _, c := range cases {
		got, err := Recipients(c.in)
		if (err == nil) != c.ok || strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: %v %v", c.name, got, err)
		}
	}
}

func TestPlanSend(t *testing.T) {
	ok := apitypes.SendPushRequest{To: "u1", Title: "New mail", Body: "From Dana"}
	cases := []struct {
		name string
		edit func(*apitypes.SendPushRequest)
		code string
		test func(Send) bool
	}{
		{"the defaults", func(*apitypes.SendPushRequest) {}, "", func(p Send) bool {
			return p.TTL == DefaultTTL && p.Urgency == "normal" && p.Notification.Title == "New mail"
		}},
		{"a path stays a path", func(r *apitypes.SendPushRequest) { r.URL = "/mail/42" }, "", func(p Send) bool { return p.Notification.URL == "/mail/42" }},
		{"an https link", func(r *apitypes.SendPushRequest) { r.URL = "https://example.com/a" }, "", func(p Send) bool { return p.Notification.URL == "https://example.com/a" }},
		{"a title is trimmed", func(r *apitypes.SendPushRequest) { r.Title = "  Hi  " }, "", func(p Send) bool { return p.Notification.Title == "Hi" }},
		{"no title", func(r *apitypes.SendPushRequest) { r.Title = " " }, "INVALID_REQUEST", nil},
		{"a long title", func(r *apitypes.SendPushRequest) { r.Title = strings.Repeat("é", MaxTitle+1) }, "INVALID_REQUEST", nil},
		{"a long body", func(r *apitypes.SendPushRequest) { r.Body = strings.Repeat("a", MaxBody+1) }, "INVALID_REQUEST", nil},
		{"an http link", func(r *apitypes.SendPushRequest) { r.URL = "http://example.com" }, "INVALID_REQUEST", nil},
		{"a javascript link", func(r *apitypes.SendPushRequest) { r.URL = "javascript:alert(1)" }, "INVALID_REQUEST", nil},
		{"a protocol-relative link", func(r *apitypes.SendPushRequest) { r.URL = "//evil.example/x" }, "INVALID_REQUEST", nil},
		{"a relative link", func(r *apitypes.SendPushRequest) { r.URL = "mail/42" }, "INVALID_REQUEST", nil},
		{"a ttl", func(r *apitypes.SendPushRequest) { r.TTL = 600 }, "", func(p Send) bool { return p.TTL == 600 }},
		{"a negative ttl", func(r *apitypes.SendPushRequest) { r.TTL = -1 }, "INVALID_REQUEST", nil},
		{"a ttl past four weeks", func(r *apitypes.SendPushRequest) { r.TTL = MaxTTL + 1 }, "INVALID_REQUEST", nil},
		{"high urgency", func(r *apitypes.SendPushRequest) { r.Urgency = "high" }, "", func(p Send) bool { return p.Urgency == "high" }},
		{"an unknown urgency", func(r *apitypes.SendPushRequest) { r.Urgency = "asap" }, "INVALID_REQUEST", nil},
		{"a fallback address", func(r *apitypes.SendPushRequest) { r.FallbackEmail = "dana@example.com" }, "", func(p Send) bool { return p.FallbackEmail == "dana@example.com" }},
		{"two fallback addresses", func(r *apitypes.SendPushRequest) { r.FallbackEmail = "a@x.com, b@x.com" }, "INVALID_REQUEST", nil},
		{"not an address", func(r *apitypes.SendPushRequest) { r.FallbackEmail = "dana" }, "INVALID_REQUEST", nil},
		{"too much data", func(r *apitypes.SendPushRequest) { r.Data = map[string]any{"x": strings.Repeat("a", 4000)} }, "PUSH_TOO_LARGE", nil},
	}
	for _, c := range cases {
		in := ok
		c.edit(&in)
		p, err := CheckSend(in)
		if code(err) != c.code {
			t.Errorf("%s: code %q (%v), want %q", c.name, code(err), err, c.code)
			continue
		}
		if c.test != nil && !c.test(p) {
			t.Errorf("%s: plan %+v", c.name, p)
		}
	}
}

func TestParse(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	pub := base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	auth := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 16))
	cases := []struct {
		name, endpoint, p256dh, auth string
		ok                           bool
	}{
		{"chrome", "https://fcm.googleapis.com/fcm/send/x:y", pub, auth, true},
		{"padded keys", "https://fcm.googleapis.com/fcm/send/x", pub + "=", auth + "==", true},
		{"firefox", "https://updates.push.services.mozilla.com/wpush/v2/abc", pub, auth, true},
		{"safari", "https://web.push.apple.com/QGx", pub, auth, true},
		{"edge", "https://wns2-par02p.notify.windows.com/w/?token=abc", pub, auth, true},
		{"any other host", "https://evil.example/push", pub, auth, false},
		{"lookalike host", "https://fcm.googleapis.com.evil.example/x", pub, auth, false},
		{"suffix without a dot", "https://xfcm.googleapis.com/x", pub, auth, false},
		{"plain http", "http://fcm.googleapis.com/fcm/send/x", pub, auth, false},
		{"a port", "https://fcm.googleapis.com:8443/x", pub, auth, false},
		{"credentials", "https://u:p@fcm.googleapis.com/x", pub, auth, false},
		{"no keys", "https://fcm.googleapis.com/x", "", "", false},
		{"not a point", "https://fcm.googleapis.com/x", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, 65)), auth, false},
		{"short auth", "https://fcm.googleapis.com/x", pub, "AAAA", false},
	}
	for _, c := range cases {
		_, problem := CheckSubscription(c.endpoint, c.p256dh, c.auth)
		if (problem == nil) != c.ok {
			t.Errorf("%s: problem %v, want ok %v", c.name, problem, c.ok)
		}
	}
}

func TestUrgency(t *testing.T) {
	for in, want := range map[string]string{"": "normal", "high": "high", "very-low": "very-low", "urgent": ""} {
		got, ok := Urgency(in)
		if got != want || ok != (want != "") {
			t.Errorf("%q = %q %v", in, got, ok)
		}
	}
}
