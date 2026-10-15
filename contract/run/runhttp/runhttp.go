// Package runhttp is contract/run's half for calls to other services (ARCHITECTURE.md §4.9): an
// HTTP client cannot be made without a deadline. It is its own package so a program that makes
// no calls, such as whisk-init, does not carry net/http.
package runhttp

import (
	"net"
	"net/http"
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
