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

// TestRefusalNamesNoAddress proves the error a client answers for a refused connection does not
// carry the address the name resolved to, as net.OpError would.
func TestRefusalNamesNoAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	for _, target := range []string{"http://localhost:" + port + "/", "http://127.0.0.1:" + port + "/"} {
		_, err := Client(5 * time.Second).Get(target)
		if !errors.Is(err, ErrRefused) {
			t.Fatalf("%s: %v", target, err)
		}
		var op *net.OpError
		if errors.As(err, &op) {
			t.Fatalf("%s: the refusal is wrapped in a net.OpError: %v", target, err)
		}
	}
	conn, err := Policy{}.DialContext(context.Background(), "tcp", srv.Listener.Addr().String())
	if conn != nil || err == nil || err.Error() != "the address is not on the public internet" {
		t.Fatalf("DialContext: conn = %v, err = %v", conn, err)
	}
}

func TestResolve(t *testing.T) {
	for _, c := range []struct {
		name    string
		entries []string
		want    []string
		err     bool
	}{
		{"an IPv4 literal", []string{"127.0.0.1:9100"}, []string{"127.0.0.1:9100"}, false},
		{"an IPv6 literal", []string{"[::1]:9000"}, []string{"[::1]:9000"}, false},
		{"a mapped literal", []string{"[::ffff:10.0.0.1]:80"}, []string{"10.0.0.1:80"}, false},
		{"several", []string{"127.0.0.1:1", "10.0.0.2:2"}, []string{"127.0.0.1:1", "10.0.0.2:2"}, false},
		{"no port", []string{"127.0.0.1"}, nil, true},
		{"port zero", []string{"127.0.0.1:0"}, nil, true},
		{"a port too large", []string{"127.0.0.1:70000"}, nil, true},
		{"one good, one bad", []string{"127.0.0.1:9100", "nope"}, []string{"127.0.0.1:9100"}, true},
		{"nothing", nil, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolve(context.Background(), net.DefaultResolver, c.entries...)
			if (err != nil) != c.err {
				t.Fatalf("err = %v, want error %v", err, c.err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != netip.MustParseAddrPort(c.want[i]) {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
	got, err := Resolve(context.Background(), net.DefaultResolver, "localhost:9100")
	if err != nil || len(got) == 0 || !got[0].Addr().IsLoopback() || got[0].Port() != 9100 {
		t.Fatalf("localhost: %v, %v", got, err)
	}
}
