package medialink

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

var key = bytes.Repeat([]byte{7}, 32)

const app = "01J9APP"

func query(s string) url.Values {
	q, err := url.ParseQuery(s)
	if err != nil {
		panic(err)
	}
	return q
}

func TestImageResourceCoversWhatChangesThePicture(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"w=800", "w=800&h=&fit=contain&format=auto"},
		{"w=0800", "w=800"},
		{"format=jpg", "format=jpeg"},
		{"fit=COVER&w=1&h=1", "w=1&h=1&fit=cover"},
		{"w=800&v=3", "w=800"},
		{"", "fit=&format="},
	} {
		if ImageResource("U", query(tc.a)) != ImageResource("U", query(tc.b)) {
			t.Errorf("%q and %q name different pictures: %s vs %s", tc.a, tc.b, ImageResource("U", query(tc.a)), ImageResource("U", query(tc.b)))
		}
	}
	for _, tc := range []struct{ a, b string }{
		{"w=800", "w=801"},
		{"w=800", "h=800"},
		{"w=8&h=8", "w=8&h=8&fit=cover"},
		{"w=8", "w=8&format=png"},
		{"w=8", "w=8x"},
	} {
		if ImageResource("U", query(tc.a)) == ImageResource("U", query(tc.b)) {
			t.Errorf("%q and %q are the same resource", tc.a, tc.b)
		}
	}
	if ImageResource("U", nil) == ImageResource("V", nil) {
		t.Error("two uploads are the same resource")
	}
}

func TestResource(t *testing.T) {
	for _, tc := range []struct {
		path, query, want string
		ok                bool
	}{
		{"/.whisk/img/U", "w=10", "img/U?w=10&h=0&fit=contain&format=auto", true},
		{"/.whisk/img/U/x", "", "", false},
		{"/.whisk/img/", "", "", false},
		{"/.whisk/media/U", "", "media/U", true},
		{"/.whisk/media/U/720.mp4", "", "media/U", true},
		{"/.whisk/media/U/embed", "", "media/U", true},
		{"/.whisk/media/", "", "", false},
		{"/.whisk/player.js", "", "", false},
		{"/notes", "", "", false},
	} {
		got, ok := Resource(tc.path, query(tc.query))
		if got != tc.want || ok != tc.ok {
			t.Errorf("Resource(%s?%s) = %q %v, want %q %v", tc.path, tc.query, got, ok, tc.want, tc.ok)
		}
	}
}

func TestVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	exp := now.Add(time.Hour).Unix()
	res := ImageResource("U", query("w=800"))
	good := Sign(key, app, res, exp, 1)
	claim := func(s string) Claim {
		c, err := Parse(query(s))
		if err != nil {
			t.Fatalf("Parse(%s): %v", s, err)
		}
		return c
	}
	signed := claim(Query(exp, 1, good))
	for _, tc := range []struct {
		name     string
		key      []byte
		app, res string
		c        Claim
		at       time.Time
		want     error
	}{
		{"genuine", key, app, res, signed, now, nil},
		{"one second before expiry", key, app, res, signed, time.Unix(exp-1, 0), nil},
		{"at expiry", key, app, res, signed, time.Unix(exp, 0), ErrExpired},
		{"another size", key, app, ImageResource("U", query("w=1600")), signed, now, ErrBad},
		{"another upload", key, app, ImageResource("V", query("w=800")), signed, now, ErrBad},
		{"another app", key, "01J9OTHER", res, signed, now, ErrBad},
		{"another key", bytes.Repeat([]byte{8}, 32), app, res, signed, now, ErrBad},
		{"a later exp", key, app, res, Claim{Expires: exp + 3600, Key: 1, Sig: signed.Sig}, now, ErrBad},
		{"another kid", key, app, res, Claim{Expires: exp, Key: 2, Sig: signed.Sig}, now, ErrBad},
		{"tampered after expiry is bad, not expired", key, app, ImageResource("U", query("w=1")), signed, time.Unix(exp+10, 0), ErrBad},
	} {
		if err := Verify(tc.key, tc.app, tc.res, tc.c, tc.at); !errors.Is(err, tc.want) && !(err == nil && tc.want == nil) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	sig := Sign(key, app, "media/U", 1_900_000_000, 1)
	if _, err := Parse(query(Query(1_900_000_000, 1, sig))); err != nil {
		t.Fatalf("a signed query does not parse: %v", err)
	}
	for _, q := range []string{
		"",
		"exp=1900000000&kid=1",
		"exp=1900000000&sig=" + sig,
		"kid=1&sig=" + sig,
		"exp=x&kid=1&sig=" + sig,
		"exp=-5&kid=1&sig=" + sig,
		"exp=1900000000&kid=0&sig=" + sig,
		"exp=1900000000&kid=1&sig=" + sig + "AA",
		"exp=1900000000&kid=1&sig=***",
		"exp=1900000000&kid=1&sig=" + sig + "&sig=" + sig,
		"exp=1900000000&exp=1900000001&kid=1&sig=" + sig,
	} {
		if _, err := Parse(query(q)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Parse(%q) = %v, want ErrMalformed", q, err)
		}
	}
	if !Present(query("sig=")) || Present(query("w=1")) {
		t.Error("Present does not follow sig")
	}
}

func TestCarry(t *testing.T) {
	q := query("w=1&" + Query(1_900_000_000, 3, "abc_-"))
	if got := Carry(q); got != "exp=1900000000&kid=3&sig=abc_-" {
		t.Errorf("Carry = %q", got)
	}
}

func TestExpiry(t *testing.T) {
	now := time.Unix(1_800_000_123, 0)
	for _, ttl := range []time.Duration{MinLifetime, 5 * time.Minute, DefaultLifetime, MaxLifetime, 37*time.Minute + 3*time.Second} {
		exp := Expiry(now, ttl)
		if life := exp.Sub(now); life > ttl || life < ttl*3/4-time.Second {
			t.Errorf("ttl %s: lives %s", ttl, life)
		}
		// Issued again later in the same step, the link is the same address.
		if again := Expiry(now.Add(time.Second), ttl); !again.Equal(exp) && again.Sub(exp) < max(ttl/4, 15*time.Second)-time.Second {
			t.Errorf("ttl %s: a second later moved the expiry by %s", ttl, again.Sub(exp))
		}
	}
	if !Expiry(now, time.Hour).Equal(Expiry(now.Add(10*time.Second), time.Hour)) {
		t.Error("two links ten seconds apart are different addresses")
	}
}

func TestLifetime(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want time.Duration
		ok   bool
	}{
		{0, DefaultLifetime, true},
		{60, time.Minute, true},
		{43200, MaxLifetime, true},
		{59, 0, false},
		{43201, 0, false},
		{-1, 0, false},
		{1 << 62, 0, false},
	} {
		got, ok := Lifetime(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Lifetime(%d) = %s %v, want %s %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// FuzzVerify (HARNESS.md §8.3): a query verifies only when its signature is the key's over
// exactly the resource asked for, and every refusal is one of the stated reasons.
func FuzzVerify(f *testing.F) {
	now := time.Unix(1_800_000_000, 0)
	f.Add("/.whisk/img/U", "w=800&"+Query(1_800_003_600, 1, Sign(key, app, ImageResource("U", query("w=800")), 1_800_003_600, 1)))
	f.Add("/.whisk/media/U/720.mp4", Query(1_800_003_600, 1, Sign(key, app, "media/U", 1_800_003_600, 1)))
	f.Add("/.whisk/img/U", "w=800&exp=1&kid=1&sig=x")
	f.Fuzz(func(t *testing.T, path, raw string) {
		q, err := url.ParseQuery(raw)
		if err != nil {
			return
		}
		res, ok := Resource(path, q)
		if !ok {
			return
		}
		c, err := Parse(q)
		if err != nil {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("unstated reason %v", err)
			}
			return
		}
		err = Verify(key, app, res, c, now)
		if err != nil && !errors.Is(err, ErrBad) && !errors.Is(err, ErrExpired) {
			t.Fatalf("unstated reason %v", err)
		}
		if err == nil && Sign(key, app, res, c.Expires, c.Key) != q.Get(ParamSignature) {
			t.Fatalf("verified a signature that is not the key's: %s", raw)
		}
	})
}

func TestAddress(t *testing.T) {
	for _, tc := range []struct {
		raw, kind, id string
	}{
		{"/.whisk/img/U", KindImage, "U"},
		{"/.whisk/img/U?w=800&h=600&fit=cover", KindImage, "U"},
		{"/.whisk/img/U?w=800&v=2", KindImage, "U"},
		{"/.whisk/media/U", KindMedia, "U"},
		{"/.whisk/media/U/720.mp4", KindMedia, "U"},
		{"/.whisk/media/U/embed", KindMedia, "U"},
	} {
		kind, id, _, err := Address(tc.raw)
		if err != nil || kind != tc.kind || id != tc.id {
			t.Errorf("Address(%q) = %s %s %v, want %s %s", tc.raw, kind, id, err, tc.kind, tc.id)
		}
	}
	for _, raw := range []string{
		"", "U", ".whisk/img/U", "/.whisk/img/", "/.whisk/img/U/x", "/.whisk/media/", "/.whisk/media/U/", "/.whisk/media/U/a/b",
		"/.whisk/media/U?w=1", "/.whisk/player.js", "/notes", "https://app.example/.whisk/img/U", "//evil.example/.whisk/img/U",
		"/.whisk/img/U?w=1&sig=x", "/.whisk/img/U?exp=1", "/.whisk/img/U?kid=1", "/.whisk/img/U#x", "/.whisk/img/U?%zz",
	} {
		if _, _, _, err := Address(raw); err == nil {
			t.Errorf("Address(%q) was accepted", raw)
		}
	}
}

// A path signed as asked is let in at itself: what Address reads and SignedPath writes is
// what Resource and Parse read back.
func TestSignedPathRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	exp := Expiry(now, time.Hour).Unix()
	for _, raw := range []string{"/.whisk/img/U", "/.whisk/img/U?w=96&h=96&fit=cover", "/.whisk/media/U", "/.whisk/media/U/poster.jpg"} {
		_, _, q, err := Address(raw)
		if err != nil {
			t.Fatal(err)
		}
		path, _, _ := strings.Cut(raw, "?")
		res, _ := Resource(path, q)
		signed := SignedPath(raw, exp, 1, Sign(key, app, res, exp, 1))
		u, err := url.Parse(signed)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := Resource(u.Path, u.Query())
		c, perr := Parse(u.Query())
		if !ok || got != res || perr != nil || Verify(key, app, got, c, now) != nil {
			t.Errorf("%s signed as %s is not let in: %v %v", raw, signed, perr, Verify(key, app, got, c, now))
		}
	}
}
