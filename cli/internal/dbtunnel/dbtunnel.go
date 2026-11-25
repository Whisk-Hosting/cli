// Package dbtunnel is whisk db connect's local end (CLI.md §5.9): a listener on 127.0.0.1 that
// any Postgres tool reaches without TLS, each of whose connections it carries to the platform's
// database proxy over TLS that checks the proxy's name. The tool's own sign-in, with the
// read-only login's password, then crosses the tunnel unread.
package dbtunnel

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/whisk-run/contract/run"
)

// Postgres's request codes a client may send before its startup message.
const (
	sslRequest = 80877103
	gssRequest = 80877104
)

// maxStartup bounds the startup message a local client may send.
const maxStartup = 10000

// Local is the connection string a tool uses through the tunnel: the remote one's login and
// database at addr, without TLS, which the tunnel adds. Pure.
func Local(remote, addr string) (string, error) {
	u, err := url.Parse(remote)
	if err != nil {
		return "", err
	}
	u.Host = addr
	u.RawQuery = url.Values{"sslmode": {"disable"}}.Encode()
	return u.String(), nil
}

// Remote is the proxy's address and name in a connection string. Pure.
func Remote(remote string) (addr, name string, err error) {
	u, err := url.Parse(remote)
	if err != nil {
		return "", "", err
	}
	if u.Hostname() == "" {
		return "", "", errors.New("the connection string names no host")
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	return net.JoinHostPort(u.Hostname(), port), u.Hostname(), nil
}

// Startup reads a local client's startup message, answering "no" to any request for TLS or
// GSSAPI first, so the client goes on in the clear to the tunnel. It returns the message as sent.
func Startup(conn io.ReadWriter) ([]byte, error) {
	for range 3 {
		head := make([]byte, 8)
		if _, err := io.ReadFull(conn, head); err != nil {
			return nil, err
		}
		n := binary.BigEndian.Uint32(head[:4])
		code := binary.BigEndian.Uint32(head[4:])
		if n == 8 && (code == sslRequest || code == gssRequest) {
			if _, err := conn.Write([]byte{'N'}); err != nil {
				return nil, err
			}
			continue
		}
		if n < 8 || n > maxStartup {
			return nil, fmt.Errorf("a startup message of %d bytes", n)
		}
		rest := make([]byte, n-8)
		if _, err := io.ReadFull(conn, rest); err != nil {
			return nil, err
		}
		return append(head, rest...), nil
	}
	return nil, errors.New("the client asked for encryption three times")
}

// Options are a tunnel's.
type Options struct {
	Listener net.Listener
	// Proxy is the proxy's address, Name the name its certificate must carry.
	Proxy, Name string
	// Roots verifies the proxy's certificate; nil is the system's.
	TLS *tls.Config
	// Until closes the tunnel when the login stops working.
	Until time.Time
	// Opened is told of each connection carried, for the log.
	Opened func(from string)
}

// Serve carries the listener's connections until ctx ends or Until passes, and answers how many
// it carried.
func Serve(ctx context.Context, o Options) (int, error) {
	ctx, cancel := context.WithDeadline(ctx, o.Until)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = o.Listener.Close() })
	defer stop()
	var wg sync.WaitGroup
	var open sync.Map
	defer func() {
		open.Range(func(k, _ any) bool { _ = k.(net.Conn).Close(); return true })
		wg.Wait()
	}()
	n := 0
	for {
		conn, err := o.Listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return n, nil
			}
			return n, err
		}
		n++
		if o.Opened != nil {
			o.Opened(conn.RemoteAddr().String())
		}
		open.Store(conn, true)
		wg.Add(1)
		run.Spawn(ctx, "db connect: a connection", func(ctx context.Context) error {
			defer wg.Done()
			defer open.Delete(conn)
			defer func() { _ = conn.Close() }()
			_ = carry(ctx, conn, o)
			return nil
		})
	}
}

// carry takes one local connection to the proxy.
func carry(ctx context.Context, local net.Conn, o Options) error {
	_ = local.SetDeadline(time.Now().Add(30 * time.Second))
	startup, err := Startup(local)
	if err != nil {
		return err
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", o.Proxy)
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))
	req := make([]byte, 8)
	binary.BigEndian.PutUint32(req[:4], 8)
	binary.BigEndian.PutUint32(req[4:], sslRequest)
	if _, err := raw.Write(req); err != nil {
		return err
	}
	answer := make([]byte, 1)
	if _, err := io.ReadFull(raw, answer); err != nil {
		return err
	}
	if answer[0] != 'S' {
		return errors.New("the database proxy would not use TLS")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if o.TLS != nil {
		cfg = o.TLS.Clone()
	}
	cfg.ServerName = o.Name
	remote := tls.Client(raw, cfg)
	if err := remote.HandshakeContext(ctx); err != nil {
		return err
	}
	if _, err := remote.Write(startup); err != nil {
		return err
	}
	_ = local.SetDeadline(time.Time{})
	_ = raw.SetDeadline(time.Time{})
	done := make(chan struct{}, 2)
	run.Spawn(ctx, "db connect: to the proxy", func(context.Context) error { _, _ = io.Copy(remote, local); done <- struct{}{}; return nil })
	run.Spawn(ctx, "db connect: from the proxy", func(context.Context) error { _, _ = io.Copy(local, remote); done <- struct{}{}; return nil })
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}
