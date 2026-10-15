package runhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientNeedsADeadline(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a client with no deadline was made")
		}
	}()
	Client(0)
}

func TestClientEndsASlowCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	start := time.Now()
	_, err := Client(50 * time.Millisecond).Get(srv.URL)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
}

// TestDefaultTransportIsHonoured: a program that sets its own default transport, as the harness
// does with its test CA, has every client trust what that transport trusts.
func TestDefaultTransportIsHonoured(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	if _, err := Client(5 * time.Second).Get(srv.URL); err == nil {
		t.Fatal("a server signed by an unknown CA was trusted")
	}
	saved := http.DefaultTransport
	defer func() { http.DefaultTransport = saved }()
	http.DefaultTransport = srv.Client().Transport
	clients := map[string]*http.Client{"Client": Client(5 * time.Second), "StreamClient": StreamClient(5 * time.Second),
		"Transport": ClientWith(5*time.Second, Transport())}
	for name, c := range clients {
		resp, err := c.Get(srv.URL)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		resp.Body.Close()
	}
}
