package config

import (
	"testing"
	"time"
)

func TestRenewDue(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	login := Credential{Token: "whsk_agent_x", PrivateKey: "key", ExpiresAt: now.Add(3 * 24 * time.Hour)}
	with := func(change func(*Credential)) Credential {
		c := login
		change(&c)
		return c
	}
	for _, tc := range []struct {
		name string
		c    Credential
		want bool
	}{
		{"three days left", login, true},
		{"a minute left", with(func(c *Credential) { c.ExpiresAt = now.Add(time.Minute) }), true},
		{"just under a week left", with(func(c *Credential) { c.ExpiresAt = now.Add(RenewWithin - time.Second) }), true},
		{"exactly a week left", with(func(c *Credential) { c.ExpiresAt = now.Add(RenewWithin) }), false},
		{"a month left", with(func(c *Credential) { c.ExpiresAt = now.Add(30 * 24 * time.Hour) }), false},
		{"already expired", with(func(c *Credential) { c.ExpiresAt = now.Add(-time.Second) }), false},
		{"expires now", with(func(c *Credential) { c.ExpiresAt = now }), false},
		{"no expiry recorded", with(func(c *Credential) { c.ExpiresAt = time.Time{} }), false},
		{"not bound to this computer", with(func(c *Credential) { c.PrivateKey = "" }), false},
		{"no token", with(func(c *Credential) { c.Token = "" }), false},
		{"a login waiting for --resume", with(func(c *Credential) { c.PendingCode = "abc" }), false},
	} {
		if got := RenewDue(tc.c, now); got != tc.want {
			t.Errorf("%s: RenewDue = %v, want %v", tc.name, got, tc.want)
		}
	}
}
