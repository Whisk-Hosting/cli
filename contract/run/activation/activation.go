// Package activation serves on the listening socket systemd holds for a service (socket
// activation), so a restart of the service leaves its port open: connections wait in the
// socket's queue for the new process instead of being refused. Without one it listens itself.
package activation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// firstFD is the first descriptor systemd passes (SD_LISTEN_FDS_START).
const firstFD = 3

// Env is what systemd's socket activation tells a process: LISTEN_PID, LISTEN_FDS and
// LISTEN_FDNAMES as set, and the process's own pid.
type Env struct {
	PID, FDs, Names string
	Self            int
}

// Pick is the descriptor of the socket named name among those systemd passed, and whether it
// passed any to this process at all. Sockets passed to another process (LISTEN_PID names
// another, as in a program this one started) or none count as not passed; passed sockets without
// name among them, or a count and names that disagree, are an error, since listening anew would
// race the socket systemd holds. With no names given, a single socket is the one. Pure.
func Pick(e Env, name string) (int, bool, error) {
	if e.FDs == "" {
		return 0, false, nil
	}
	if pid, err := strconv.Atoi(e.PID); err != nil || pid != e.Self {
		return 0, false, nil
	}
	n, err := strconv.Atoi(e.FDs)
	if err != nil || n < 0 {
		return 0, false, fmt.Errorf("LISTEN_FDS %q is not a count", e.FDs)
	}
	if n == 0 {
		return 0, false, nil
	}
	if e.Names == "" {
		if n == 1 {
			return firstFD, true, nil
		}
		return 0, false, fmt.Errorf("systemd passed %d sockets without names, so none is known to be %q", n, name)
	}
	names := strings.Split(e.Names, ":")
	if len(names) != n {
		return 0, false, fmt.Errorf("LISTEN_FDS is %d but LISTEN_FDNAMES names %d", n, len(names))
	}
	for i, got := range names {
		if got == name {
			return firstFD + i, true, nil
		}
	}
	return 0, false, fmt.Errorf("systemd passed the sockets %q and none is named %q", e.Names, name)
}

// Listen is the TCP listener systemd passed under name (the socket unit's FileDescriptorName),
// or a new one on addr when it passed none. Programs the process starts ignore the passed
// variables, which name this process's pid; the listener's descriptor closes on exec.
func Listen(name, addr string) (*Listener, error) {
	fd, ok, err := Pick(Env{PID: os.Getenv("LISTEN_PID"), FDs: os.Getenv("LISTEN_FDS"), Names: os.Getenv("LISTEN_FDNAMES"), Self: os.Getpid()}, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, err
		}
		return Track(lis), nil
	}
	f := os.NewFile(uintptr(fd), name)
	// FileListener duplicates the descriptor with close-on-exec; systemd's copy stays its own.
	defer f.Close()
	lis, err := net.FileListener(f)
	if err != nil {
		return nil, fmt.Errorf("socket %q from systemd: %w", name, err)
	}
	return Track(lis), nil
}

// Listener is a listener that knows the connections it accepted and has not seen closed, so a
// stop can wait for them: http.Server.Shutdown can return while a connection it accepted in the
// same instant is still unanswered, and a process that then exits resets it, the one request a
// restart was meant to keep (24 November, build2's push helper: TestDrainWaitsForAnAcceptedConnection).
type Listener struct {
	net.Listener
	open    sync.WaitGroup
	stopped atomic.Bool
}

// Track wraps lis so its accepted connections can be waited for.
func Track(lis net.Listener) *Listener { return &Listener{Listener: lis} }

// Accept is the next connection, or http.ErrServerClosed once Stop has closed the listener, so a
// server's Serve ends as it does after Shutdown.
func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		if l.stopped.Load() {
			return nil, http.ErrServerClosed
		}
		return nil, err
	}
	l.open.Add(1)
	return &trackedConn{Conn: c, done: l.open.Done}, nil
}

// Stop stops srv, which serves on l, without losing a request. It closes the listener, so the
// connections still in the socket's queue wait there for the next process; turns keep-alives
// off, so each connection closes once its request is answered; waits until every connection l
// accepted is closed or ctx ends; and only then shuts srv down. Shutting down first loses a
// request: Go's server closes, without an answer, a connection whose request it reads after its
// shutdown began, and a connection accepted in the instant before the stop had sent its request
// a moment later (TestStopAnswersARequestThatArrivesAfterItBegins).
func (l *Listener) Stop(ctx context.Context, srv *http.Server) error {
	l.stopped.Store(true)
	_ = l.Listener.Close()
	srv.SetKeepAlivesEnabled(false)
	err := l.Drain(ctx)
	// Shutdown closes the listener again, which answers net.ErrClosed: that is the stop itself.
	if serr := srv.Shutdown(ctx); err == nil && !errors.Is(serr, net.ErrClosed) {
		err = serr
	}
	return err
}

// Drain waits until every connection the listener accepted is closed, or ctx ends. Call it after
// the server's Shutdown, before the process exits.
func (l *Listener) Drain(ctx context.Context) error {
	done := make(chan struct{})
	go func() { l.open.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("connections still open when the stop's time ran out: %w", ctx.Err())
	}
}

type trackedConn struct {
	net.Conn
	once sync.Once
	done func()
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.done)
	return err
}

// Serve serves srv on l until ctx ends, then stops it within grace (Stop), so a stop answers
// every request that reached the process.
func (l *Listener) Serve(ctx context.Context, srv *http.Server, grace time.Duration) error {
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shut, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancel()
	err := l.Stop(shut, srv)
	if serr := <-served; err == nil && !errors.Is(serr, http.ErrServerClosed) {
		err = serr
	}
	return err
}
