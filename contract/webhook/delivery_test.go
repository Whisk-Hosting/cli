package webhook

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtures/deliveries holds a signed delivery: the key, the headers, the raw body and the
// signature the platform put on it. Every implementation of the check must agree with it.
func loadDeliveryFixture(t *testing.T) (key []byte, h http.Header, body []byte, sig string) {
	t.Helper()
	dir := filepath.Join("..", "fixtures", "deliveries")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(read("key.b64"))))
	if err != nil {
		t.Fatal(err)
	}
	var flat map[string]string
	if err := json.Unmarshal(read("headers.json"), &flat); err != nil {
		t.Fatal(err)
	}
	h = http.Header{}
	for k, v := range flat {
		h.Set(k, v)
	}
	return key, h, read("body"), strings.TrimSpace(string(read("signature")))
}

func TestDeliverySignatureMatchesFixture(t *testing.T) {
	key, h, body, sig := loadDeliveryFixture(t)
	if got := DeliverySignature(key, h.Get("X-Whisk-Webhook-Id"), h.Get("X-Whisk-Webhook-Received-At"), body); got != sig {
		t.Fatalf("signature = %s, fixture says %s", got, sig)
	}
	h.Set(DeliverySignatureHeader, sig)
	if why := VerifyDelivery(key, h, body); why != "" {
		t.Fatalf("fixture does not verify: %s", why)
	}
}

func TestVerifyDeliveryRefusals(t *testing.T) {
	key, h, body, sig := loadDeliveryFixture(t)
	h.Set(DeliverySignatureHeader, sig)
	cases := []struct {
		name string
		mut  func(h http.Header) []byte
		want string
	}{
		{"wrong signature", func(h http.Header) []byte { h.Set(DeliverySignatureHeader, "v1="+strings.Repeat("0", 64)); return body }, "mismatch"},
		{"altered body", func(h http.Header) []byte { return append([]byte(" "), body...) }, "mismatch"},
		{"missing header", func(h http.Header) []byte { h.Del(DeliverySignatureHeader); return body }, "missing"},
		{"missing id", func(h http.Header) []byte { h.Del("X-Whisk-Webhook-Id"); return body }, "missing"},
		{"service app present", func(h http.Header) []byte { h.Set("X-Whisk-Service-App", "01APP"); return body }, "service_app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hh := h.Clone()
			b := tc.mut(hh)
			if got := VerifyDelivery(key, hh, b); got != tc.want {
				t.Errorf("verdict = %q, want %q", got, tc.want)
			}
		})
	}
	if got := VerifyDelivery(nil, h, body); got != "no_key" {
		t.Errorf("no key: %q", got)
	}
}
