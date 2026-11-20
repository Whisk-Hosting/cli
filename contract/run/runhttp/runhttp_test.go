package runhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"syscall"
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

// A socket that is served a moment after the call starts is reached within the call's deadline;
// one that never is fails when the deadline does; any other dial error fails at once.
func TestDialSocketWaitsForItsSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sock")
	go func() {
		time.Sleep(250 * time.Millisecond)
		l, err := net.Listen("unix", path)
		if err != nil {
			t.Error(err)
			return
		}
		t.Cleanup(func() { l.Close() })
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := DialSocket(path, time.Second)(ctx, "", "")
	if err != nil {
		t.Fatalf("the socket served after 250 ms: %v", err)
	}
	c.Close()

	ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := DialSocket(filepath.Join(t.TempDir(), "never.sock"), time.Second)(ctx, "", ""); !errors.Is(err, syscall.ENOENT) {
		t.Errorf("a socket never served: %v", err)
	}
	if took := time.Since(start); took < 250*time.Millisecond || took > 2*time.Second {
		t.Errorf("gave up after %v, want the call's deadline", took)
	}

	long := filepath.Join(t.TempDir(), strings.Repeat("x", 200))
	start = time.Now()
	if _, err := DialSocket(long, time.Second)(context.Background(), "", ""); err == nil || time.Since(start) > time.Second {
		t.Errorf("a path no socket can have: %v after %v", err, time.Since(start))
	}
}
