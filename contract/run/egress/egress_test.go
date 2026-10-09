package egress

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestPublic(t *testing.T) {
	for _, c := range []struct {
		addr string
		want bool
	}{
		{"1.1.1.1", true},
		{"8.8.8.8", true},
		{"95.217.38.236", true},
		{"2606:4700:4700::1111", true},
		{"10.97.0.1", false},
		{"10.99.0.1", false},
		{"127.0.0.1", false},
		{"127.1.2.3", false},
		{"0.0.0.0", false},
		{"0.1.2.3", false},
		{"169.254.169.254", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"100.64.0.1", false},
		{"198.18.0.1", false},
		{"192.0.2.1", false},
		{"224.0.0.1", false},
		{"255.255.255.255", false},
		{"::", false},
		{"::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:10.97.0.1", false},
		{"::ffff:1.1.1.1", true},
		{"::10.0.0.1", false},
		{"64:ff9b::a00:1", false},
		{"2002:a00:1::", false},
		{"2001::1", false},
		{"2001:db8::1", false},
		{"fc00::1", false},
		{"fd12:3456::1", false},
		{"fe80::1", false},
		{"fec0::1", false},
		{"ff02::1", false},
	} {
		if got := Public(netip.MustParseAddr(c.addr)); got != c.want {
			t.Errorf("Public(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
	if Public(netip.MustParseAddr("fe80::1%eth0")) || Public(netip.Addr{}) {
		t.Error("a zoned or invalid address is not public")
	}
}

func TestAllowed(t *testing.T) {
	p := Policy{Allow: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:9100")}}
	for _, c := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:9100", true},
		{"[::ffff:127.0.0.1]:9100", true},
		{"127.0.0.1:9101", false},
		{"127.0.0.1:2019", false},
		{"10.97.0.1:8080", false},
		{"1.1.1.1:443", true},
	} {
		if got := p.Allowed(netip.MustParseAddrPort(c.addr)); got != c.want {
			t.Errorf("Allowed(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// TestClientRefusesLoopback proves the check happens at connect time, after the name resolved:
// localhost is a name, not a literal, and the server is really listening.
func TestClientRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("inside")) }))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	_, err := Client(5 * time.Second).Get("http://localhost:" + port + "/")
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("a loopback server was reached or failed otherwise: %v", err)
	}
	var re *RefusedError
	if !errors.As(err, &re) || !re.Addr.Addr().IsLoopback() {
		t.Fatalf("the refusal does not carry the address: %v", err)
	}
	if got := re.Error(); got != "the address is not on the public internet" {
		t.Fatalf("the refusal's message names more than it should: %q", got)
	}
}

// TestRedirectNotFollowed proves a redirect, here to a loopback address, is handed back.
func TestRedirectNotFollowed(t *testing.T) {
	inside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("inside")) }))
	defer inside.Close()
	front := httptest.NewServer(http.RedirectHandler(inside.URL, http.StatusTemporaryRedirect))
	defer front.Close()
	ap := netip.MustParseAddrPort(front.Listener.Addr().String())
	c := Policy{Allow: []netip.AddrPort{ap}}.Client(5 * time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, front.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("answered %d; the redirect was followed", resp.StatusCode)
	}
}

// TestAllowedAddressReached proves an allowed private address is reached, and only it.
func TestAllowedAddressReached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("store")) }))
	defer srv.Close()
	ap := netip.MustParseAddrPort(srv.Listener.Addr().String())
	resp, err := Policy{Allow: []netip.AddrPort{ap}}.Client(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("answered %d", resp.StatusCode)
	}
}
