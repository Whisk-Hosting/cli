// Package runhttp is contract/run's half for calls to other services (ARCHITECTURE.md §4.9): an
// HTTP client cannot be made without a deadline. It is its own package so a program that makes
// no calls, such as whisk-init, does not carry net/http.
package runhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
	"time"
)

// fallback is the transport's settings when the process's default transport is not an
// *http.Transport: connecting and the TLS handshake have their own short deadlines whatever the
// call's.
var fallback = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: time.Second,
}

// base is a copy of the process's default transport, so a program that sets its own (the
// harness, with its test CA and host mapping) has every client use it.
func base() *http.Transport {
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		return t.Clone()
	}
	return fallback.Clone()
}

// Client is an HTTP client whose every call ends within timeout, connecting included. It uses
// the process's default transport, sharing its idle connections.
func Client(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		panic("runhttp.Client: a call needs a deadline")
	}
	return &http.Client{Timeout: timeout}
}

// ClientWith is Client over a transport of the caller's own, such as one with a private CA or
// a client certificate. The caller still sets its dial and handshake deadlines.
func ClientWith(timeout time.Duration, rt http.RoundTripper) *http.Client {
	if timeout <= 0 {
		panic("runhttp.ClientWith: a call needs a deadline")
	}
	return &http.Client{Timeout: timeout, Transport: rt}
}

// StreamClient is a client for a response read for as long as it lasts (a log tail, an event
// stream): connecting and the first byte of the answer have deadlines, the body has none. Read
// it under Idle, so a stream that goes quiet is noticed.
func StreamClient(firstByte time.Duration) *http.Client {
	t := base()
	t.ResponseHeaderTimeout = firstByte
	return &http.Client{Transport: t}
}

// StreamClientWith is StreamClient over a transport of the caller's own, such as one that dials a
// unix socket.
func StreamClientWith(firstByte time.Duration, t *http.Transport) *http.Client {
	t = t.Clone()
	t.ResponseHeaderTimeout = firstByte
	return &http.Client{Transport: t}
}

// Transport is a copy of the process's default transport, for a caller that needs its own (a
// private CA, a client certificate).
func Transport() *http.Transport { return base() }

// RetryStatus reports whether an HTTP status is worth a retry of an idempotent call.
func RetryStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}

// SocketWaits are the pauses between tries of DialSocket while its socket is not there yet.
var SocketWaits = []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 500 * time.Millisecond}

// DialSocket is a DialContext for a Unix socket another service on the machine serves, such as
// the node agent's. A socket that is not there yet, or that refuses, is tried again after each
// of SocketWaits and then every last one, until the call's context ends: the service that binds
// it starts with this one in every release and binds it a second or so later, and a dial is safe
// to repeat. Each try has attempt to connect.
func DialSocket(path string, attempt time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		d := net.Dialer{Timeout: attempt}
		var missed error
		for i := 0; ; i++ {
			c, err := d.DialContext(ctx, "unix", path)
			if err != nil && missed != nil && ended(ctx, err) {
				// The deadline fell between tries: the socket's absence is the answer.
				return nil, missed
			}
			if err == nil || !notYetServed(err) {
				return c, err
			}
			missed = err
			wait := SocketWaits[min(i, len(SocketWaits)-1)]
			select {
			case <-ctx.Done():
				return nil, err
			case <-time.After(wait):
			}
		}
	}
}

// ended is true for a dial cut short by the call's deadline, which a dial meets a moment
// before its context reports it.
func ended(ctx context.Context, err error) bool {
	var ne net.Error
	return ctx.Err() != nil || errors.As(err, &ne) && ne.Timeout()
}

// notYetServed is true for a dial whose socket is missing or has no one listening.
func notYetServed(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}
