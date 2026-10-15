package webhook

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// A fixture is one recorded delivery with the secret it was signed with, the time it should
// be verified at, and the expected verdict. Contributors add one directory per preset.
type fixture struct {
	Preset  string            `json:"preset"`
	Secret  string            `json:"secret"`
	Now     string            `json:"now"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Expect  Verdict           `json:"expect"`
	Note    string            `json:"note,omitempty"`
}

func TestPresetsLoad(t *testing.T) {
	for _, name := range []string{"stripe", "github", "shopify", "xero", "slack", "hubspot", "twilio", "zoom", "linear", "standard-webhooks", "hmac", "token"} {
		if _, ok := Presets()[name]; !ok {
			t.Errorf("preset %s missing", name)
		}
	}
}

func TestFixtures(t *testing.T) {
	files, _ := filepath.Glob("../fixtures/webhooks/*/*.json")
	if len(files) == 0 {
		t.Fatal("no webhook fixtures")
	}
	covered := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		p, ok := Presets()[fx.Preset]
		if !ok {
			t.Fatalf("%s: unknown preset %s", f, fx.Preset)
		}
		now, err := time.Parse(time.RFC3339, fx.Now)
		if err != nil {
			t.Fatalf("%s: now: %v", f, err)
		}
		h := http.Header{}
		for k, v := range fx.Headers {
			h.Set(k, v)
		}
		got := Verify(p, fx.Secret, Request{Method: fx.Method, URL: fx.URL, Header: h, Body: []byte(fx.Body)}, now)
		if got != fx.Expect {
			t.Errorf("%s: got %+v, want %+v", filepath.Base(filepath.Dir(f))+"/"+filepath.Base(f), got, fx.Expect)
		}
		covered[fx.Preset] = true
	}
	for name := range Presets() {
		if name == PresetHMAC || name == PresetToken {
			continue
		}
		if !covered[name] {
			t.Errorf("preset %s has no fixture", name)
		}
	}
}

func TestSignRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	ts := "1788782400"
	body := []byte(`{"id":"evt_1","type":"invoice.paid"}`)
	for name, p := range Presets() {
		if name == PresetHMAC || name == PresetToken {
			continue
		}
		req := Request{Method: "POST", URL: "https://hooks.whisk.run/acme/app/" + name, Body: body}
		if name == "twilio" {
			req.Body = []byte("CallSid=CA123&From=%2B6421&To=%2B6427")
		}
		h := http.Header{}
		tsValue := ts
		if p.TimestampFormat == "unix_ms" {
			tsValue = ts + "000"
		}
		if p.Timestamp != nil && p.Timestamp.From == "header_name" {
			h.Set(p.Timestamp.Name, tsValue)
		}
		if p.ID != nil && p.ID.From == "header_name" {
			h.Set(p.ID.Name, "msg_1")
		}
		req.Header = h
		h.Set(p.Header, Sign(p, "whsec_test", req, tsValue, "msg_1"))
		if v := Verify(p, "whsec_test", req, now); !v.Verified {
			t.Errorf("%s: signed request did not verify: %+v", name, v)
		}
		if v := Verify(p, "wrong", req, now); v.Verified || v.Reason != "signature_mismatch" {
			t.Errorf("%s: wrong secret verified or wrong reason: %+v", name, v)
		}
	}
}

func TestCheckRejectsBadPresets(t *testing.T) {
	bad := []Preset{
		{Algorithm: "sha256", Encoding: "hex", Payload: "{body}"},
		{Header: "X", Encoding: "hex", Payload: "{body}"},
		{Header: "X", Algorithm: "sha256", Payload: "{body}"},
		{Header: "X", Algorithm: "sha256", Encoding: "hex"},
		{Header: "X", Algorithm: "md5", Encoding: "hex", Payload: "{body}"},
		{Header: "X", Algorithm: "sha256", Encoding: "hex", Payload: "{timestamp}.{body}"},
		{Header: "X", Algorithm: "sha256", Encoding: "hex", Payload: "{nope}"},
		{Header: "X", Algorithm: "sha256", Encoding: "hex", Payload: "{body}", SignaturePattern: "("},
	}
	for i, p := range bad {
		if err := p.Check(); err == nil {
			t.Errorf("preset %d should be rejected", i)
		}
	}
	if err := (Preset{Header: "X", Algorithm: "sha256", Encoding: "hex", Payload: "{body}"}).Check(); err != nil {
		t.Errorf("the smallest complete preset was refused: %v", err)
	}
}

// Every preset with a tolerance accepts a delivery signed up to exactly that many seconds
// either side of now and refuses one a second further out: the replay window, not a test of
// "now equals the signed time".
func TestTimestampTolerance(t *testing.T) {
	signed := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"evt_1"}`)
	checked := 0
	for name, p := range Presets() {
		if p.Timestamp == nil || p.ToleranceSeconds <= 0 {
			continue
		}
		checked++
		tol := time.Duration(p.ToleranceSeconds) * time.Second
		ts := strconv.FormatInt(signed.Unix(), 10)
		switch p.TimestampFormat {
		case "unix_ms":
			ts = strconv.FormatInt(signed.UnixMilli(), 10)
		case "rfc3339":
			ts = signed.Format(time.RFC3339)
		}
		req := Request{Method: "POST", URL: "https://hooks.whisk.run/acme/app/" + name, Body: body, Header: http.Header{}}
		if name == "twilio" {
			req.Body = []byte("CallSid=CA123")
		}
		if p.Timestamp.From == "header_name" {
			req.Header.Set(p.Timestamp.Name, ts)
		}
		if p.ID != nil && p.ID.From == "header_name" {
			req.Header.Set(p.ID.Name, "msg_1")
		}
		req.Header.Set(p.Header, Sign(p, "whsec_test", req, ts, "msg_1"))
		cases := []struct {
			at     time.Duration
			reason string
		}{
			{0, ""},
			{tol - time.Second, ""},
			{tol, ""},
			{-tol, ""},
			{tol + time.Second, "stale_timestamp"},
			{-tol - time.Second, "stale_timestamp"},
		}
		for _, c := range cases {
			v := Verify(p, "whsec_test", req, signed.Add(c.at))
			if v.Reason != c.reason || v.Verified != (c.reason == "") {
				t.Errorf("%s at %v: got %+v, want reason %q", name, c.at, v, c.reason)
			}
		}
	}
	if checked < 5 {
		t.Fatalf("only %d presets with a tolerance; the catalogue has at least 5", checked)
	}
}

// A preset naming an unknown placeholder anywhere in its payload is refused, not only in the
// first position.
func TestCheckSeesEveryPlaceholder(t *testing.T) {
	p := Preset{Header: "X", Algorithm: "sha256", Encoding: "hex", Payload: "{body}.{method}.{nope}"}
	if err := p.Check(); err == nil {
		t.Fatal("a payload with an unknown third placeholder was accepted")
	}
}
