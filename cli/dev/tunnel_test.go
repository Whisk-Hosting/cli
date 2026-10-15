package dev

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func delivery(id string, at time.Time, verified bool) Delivery {
	return Delivery{ID: id, ReceivedAt: at, Verified: verified, Body: `{"id":"` + id + `"}`}
}

func TestNewDeliveries(t *testing.T) {
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	newest := delivery("c", base.Add(2*time.Minute), true)
	middle := delivery("b", base.Add(time.Minute), true)
	oldest := delivery("a", base, true)
	unverified := delivery("x", base.Add(90*time.Second), false)

	for _, tc := range []struct {
		name string
		list []Delivery
		seen map[string]bool
		want []string
	}{
		{
			name: "newest first in, oldest first out",
			list: []Delivery{newest, unverified, middle, oldest},
			seen: map[string]bool{},
			want: []string{"a", "b", "c"},
		},
		{
			name: "what was forwarded already is not forwarded twice",
			list: []Delivery{newest, middle, oldest},
			seen: map[string]bool{"a": true, "b": true},
			want: []string{"c"},
		},
		{
			name: "a delivery that never verified is never forwarded",
			list: []Delivery{unverified},
			seen: map[string]bool{},
		},
		{
			name: "nothing new",
			list: []Delivery{newest},
			seen: map[string]bool{"c": true},
		},
		{
			name: "two at the same instant keep a stable order",
			list: []Delivery{delivery("b", base, true), delivery("a", base, true)},
			seen: map[string]bool{},
			want: []string{"a", "b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewDeliveries(tc.list, tc.seen)
			if len(got) != len(tc.want) {
				t.Fatalf("NewDeliveries = %v, want %v", ids(got), tc.want)
			}
			for i, id := range tc.want {
				if got[i].ID != id {
					t.Fatalf("NewDeliveries = %v, want %v", ids(got), tc.want)
				}
			}
		})
	}
}

func ids(list []Delivery) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.ID)
	}
	return out
}

// A tunnel forwards what arrives while it is open, and nothing that was already there.
func TestTunnelForwardsOnlyWhatArrivesWhileItIsOpen(t *testing.T) {
	base := time.Now()
	history := []Delivery{delivery("old", base.Add(-time.Hour), true)}
	arrived := append([]Delivery{delivery("new", base, true)}, history...)

	calls := 0
	var forwarded []string
	n, err := Tunnel(context.Background(), TunnelOptions{
		Source: "stripe", Handler: "/hooks/stripe", Lifetime: 200 * time.Millisecond, Every: 10 * time.Millisecond,
		Deliveries: func(context.Context) ([]Delivery, error) {
			calls++
			if calls == 1 {
				return history, nil
			}
			return arrived, nil
		},
		Post: func(_ context.Context, d Delivery) (int, error) {
			forwarded = append(forwarded, d.ID)
			return 200, nil
		},
		Log: io.Discard,
	})
	if err != nil {
		t.Fatalf("Tunnel: %v", err)
	}
	if len(forwarded) != 1 || forwarded[0] != "new" {
		t.Fatalf("forwarded %v, want only the delivery that arrived while the tunnel was open", forwarded)
	}
	if n != 1 {
		t.Errorf("Tunnel reported %d forwards, want 1", n)
	}
}

// A tunnel that cannot read the stored history does not open: every old delivery would
// otherwise look new and be replayed.
func TestTunnelFailsWhenTheHistoryCannotBeRead(t *testing.T) {
	posted := 0
	_, err := Tunnel(context.Background(), TunnelOptions{
		Source: "stripe", Handler: "/hooks/stripe", Lifetime: 100 * time.Millisecond, Every: 10 * time.Millisecond,
		Deliveries: func(context.Context) ([]Delivery, error) {
			return nil, errors.New("platform unavailable")
		},
		Post: func(context.Context, Delivery) (int, error) {
			posted++
			return 200, nil
		},
		Log: io.Discard,
	})
	if err == nil {
		t.Fatal("Tunnel opened without the delivery history, want an error")
	}
	if posted != 0 {
		t.Errorf("posted %d deliveries, want none", posted)
	}
}

// The local gate sees what the deployed app would: the provider's headers, the platform's
// webhook id, and the body untouched.
func TestPostDelivery(t *testing.T) {
	var got *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	status, err := PostDelivery(context.Background(), srv.Client(), srv.URL, "hooks/stripe", Delivery{
		ID:      "01EVT",
		Headers: map[string]string{"Stripe-Signature": "t=1,v1=abc", "Content-Type": "application/json"},
		Body:    `{"id":"evt_1"}`,
	})
	if err != nil {
		t.Fatalf("PostDelivery: %v", err)
	}
	if status != http.StatusAccepted {
		t.Errorf("status %d, want 202", status)
	}
	if got.URL.Path != "/hooks/stripe" {
		t.Errorf("path %q, want /hooks/stripe", got.URL.Path)
	}
	if got.Header.Get("Stripe-Signature") != "t=1,v1=abc" {
		t.Errorf("the provider's own header did not survive: %q", got.Header.Get("Stripe-Signature"))
	}
	if got.Header.Get("X-Whisk-Webhook-Id") != "01EVT" {
		t.Errorf("webhook id %q, want the delivery's", got.Header.Get("X-Whisk-Webhook-Id"))
	}
	if !strings.Contains(body, "evt_1") {
		t.Errorf("body %q, want it untouched", body)
	}
}

func TestPostDeliveryDefaultsTheContentType(t *testing.T) {
	var ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
	}))
	defer srv.Close()
	if _, err := PostDelivery(context.Background(), srv.Client(), srv.URL, "/hooks/x", Delivery{ID: "1", Body: "{}"}); err != nil {
		t.Fatalf("PostDelivery: %v", err)
	}
	if ctype != "application/json" {
		t.Errorf("content type %q, want application/json", ctype)
	}
}
