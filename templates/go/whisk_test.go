package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The signed delivery from contract/fixtures/deliveries when the contract sits beside this
// template, otherwise the same vector inline: a copy of the template must still test alone.
type deliveryFixture struct {
	keyB64, signature string
	headers           map[string]string
	body              []byte
}

func loadFixture(t *testing.T) deliveryFixture {
	t.Helper()
	f := deliveryFixture{
		keyB64:    "t/XIoLwasp+aMSftgZasZlBv9QaR0yaNJQmq+JgCphc=", // gitleaks:allow: the public test vector, not a secret
		signature: "v1=1d94b46d0053ffcb73494700dd0a91ea76a7a92bf735be3abbefbaaf802e9191",
		headers: map[string]string{
			"X-Whisk-Webhook-Id": "01J8Z3WQ9XN4M6P8R0T2V4X6Y8", "X-Whisk-Webhook-Source": "stripe", "X-Whisk-Webhook-Received-At": "2026-09-23T10:15:30Z",
			"X-Whisk-Audience": "service", "Content-Type": "application/json",
		},
		body: []byte(`{"type":"invoice.paid","id":"in_123"}`),
	}
	dir := filepath.Join("..", "..", "contract", "fixtures", "deliveries")
	if _, err := os.Stat(dir); err != nil {
		return f
	}
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	f.keyB64, f.signature, f.body = strings.TrimSpace(string(read("key.b64"))), strings.TrimSpace(string(read("signature"))), read("body")
	if err := json.Unmarshal(read("headers.json"), &f.headers); err != nil {
		t.Fatal(err)
	}
	return f
}

// origSig stands in for the provider's own signature header, which the platform forwards
// with the X-Whisk-Webhook-Orig- prefix.
const origSig = "t=1790000130,v1=0f7f0c2b6f7a3c1e4d5b6a7988776655443322110099aabbccddeeff00112233"

// memoryRecord stands in for db.RecordEvent: an insert-once map.
func memoryRecord(seen map[string]any) recordFunc {
	return func(_ context.Context, id, kind string, payload any, verified bool) (Recorded, error) {
		if _, dup := seen[id]; dup {
			return Recorded{ID: id, Kind: kind, Duplicate: true}, nil
		}
		seen[id] = map[string]any{"payload": payload, "verified": verified}
		return Recorded{ID: id, Kind: kind}, nil
	}
}

func TestDeliveriesHandle(t *testing.T) {
	f := loadFixture(t)
	t.Setenv("WHISK_DELIVERY_KEY", f.keyB64)
	seen := map[string]any{}
	calls := 0
	var got Delivery
	h := deliveries.Handle(memoryRecord(seen), func(_ *http.Request, d Delivery) error { calls++; got = d; return nil })
	send := func(mut func(h http.Header), body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/hooks/stripe", strings.NewReader(string(body)))
		for k, v := range f.headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("X-Whisk-Delivery-Signature", f.signature)
		req.Header.Set("X-Whisk-Webhook-Orig-Stripe-Signature", origSig)
		mut(req.Header)
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	refused := func(name string, mut func(h http.Header), body []byte) {
		rec := send(mut, body)
		if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"code":"DELIVERY_UNVERIFIED"`) || !strings.Contains(rec.Body.String(), `"fix":`) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	refused("wrong signature", func(h http.Header) { h.Set("X-Whisk-Delivery-Signature", "v1="+strings.Repeat("0", 64)) }, f.body)
	refused("altered body", func(http.Header) {}, append([]byte(" "), f.body...))
	refused("missing signature", func(h http.Header) { h.Del("X-Whisk-Delivery-Signature") }, f.body)
	refused("service app present", func(h http.Header) { h.Set("X-Whisk-Service-App", "01OTHERAPP") }, f.body)
	if calls != 0 || len(seen) != 0 {
		t.Fatalf("a refused delivery reached the app or the store: calls=%d seen=%v", calls, seen)
	}

	rec := send(func(http.Header) {}, f.body)
	if rec.Code != 200 || calls != 1 || !strings.Contains(rec.Body.String(), `"duplicate":false`) {
		t.Fatalf("valid delivery: %d %s calls=%d", rec.Code, rec.Body.String(), calls)
	}
	if got.ID != f.headers["X-Whisk-Webhook-Id"] || got.Source != "stripe" || got.ReceivedAt != f.headers["X-Whisk-Webhook-Received-At"] || string(got.Body) != string(f.body) || got.Headers.Get("Stripe-Signature") != origSig {
		t.Fatalf("delivery = %+v", got)
	}
	row, _ := seen[got.ID].(map[string]any)
	if v, _ := row["verified"].(bool); !v {
		t.Fatalf("recorded row is not marked verified: %v", row)
	}

	rec = send(func(http.Header) {}, f.body)
	if rec.Code != 200 || calls != 1 || !strings.Contains(rec.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate id: %d %s calls=%d", rec.Code, rec.Body.String(), calls)
	}

	t.Setenv("WHISK_DELIVERY_KEY", "")
	refused("no key", func(http.Header) {}, f.body)
}
