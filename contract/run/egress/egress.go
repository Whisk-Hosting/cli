// Package egress is the one way platform code calls an address a tenant chose: a bucket an owner
// brings, a webhook, an SSO issuer, a link the platform is handed back (ARCHITECTURE.md §4.10).
// Its dialer checks the address it is about to connect to, after the name has resolved, on every
// connection the client opens, so a name that resolves to a private, loopback, link-local or mesh
// address cannot reach inside, and neither can a second answer from the same name. It never uses
// a proxy and never follows a redirect, so a public server cannot hand the request on.
package egress

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"syscall"
	"time"

	"github.com/whisk-run/contract/run/runhttp"
)

// Policy is what a client may reach besides the public internet.
type Policy struct {
	// Allow names exact addresses that are not public but are the platform's own, such as the
	// platform's object store on the host's loopback. Nothing a tenant sends belongs here.
	Allow []netip.AddrPort
}

// ErrRefused is the error a refused connection wraps.
var ErrRefused = errors.New("egress refused")

// RefusedError says which address a connection was refused for. Its message names no address,
// because callers often record a failure where the tenant can read it, and the address a name
// resolved to is a fact about the platform's network.
type RefusedError struct{ Addr netip.AddrPort }

func (e *RefusedError) Error() string { return "the address is not on the public internet" }

// Unwrap lets errors.Is(err, ErrRefused) match.
func (e *RefusedError) Unwrap() error { return ErrRefused }

// Allowed decides one connection: a public address, or one the policy names exactly. It is pure.
func (p Policy) Allowed(a netip.AddrPort) bool {
	if Public(a.Addr()) {
		return true
	}
	for _, x := range p.Allow {
		if x.Addr().Unmap() == a.Addr().Unmap() && x.Port() == a.Port() {
			return true
		}
	}
	return false
}

// control runs just before each connect, with the address the socket is about to reach.
func (p Policy) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || !p.Allowed(ap) {
		return &RefusedError{Addr: ap}
	}
	return nil
}

// Dialer is a dialer that connects only where the policy allows.
func (p Policy) Dialer() *net.Dialer {
	return &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second, Control: p.control}
}

// DialContext connects only where the policy allows. A refused connection answers the
// RefusedError itself rather than the net.OpError the dialer wraps it in, whose message would
// name the address the name resolved to.
func (p Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c, err := p.Dialer().DialContext(ctx, network, address)
	var re *RefusedError
	if errors.As(err, &re) {
		return nil, re
	}
	return c, err
}

// Transport is an HTTP transport over DialContext, with no proxy and short connect and handshake
// deadlines. A caller may set its TLS settings; it must not replace DialContext or Proxy.
func (p Policy) Transport(responseHeader time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           p.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: responseHeader,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// Client is an HTTP client whose every call ends within timeout, that reaches only where the
// policy allows and follows no redirect: a 3xx is the answer.
func (p Policy) Client(timeout time.Duration) *http.Client {
	c := runhttp.ClientWith(timeout, p.Transport(timeout))
	c.CheckRedirect = NoRedirects
	return c
}

// Resolve turns host:port entries naming the platform's own addresses (an IP literal or a name)
// into what Policy.Allow holds. It is for configuration read at startup, never for anything a
// tenant sent. An entry that is not host:port, or a name that does not resolve, is an error; the
// entries that did resolve are answered with it.
func Resolve(ctx context.Context, r *net.Resolver, entries ...string) ([]netip.AddrPort, error) {
	var out []netip.AddrPort
	var errs []error
	for _, e := range entries {
		host, port, err := net.SplitHostPort(e)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pn, err := strconv.ParseUint(port, 10, 16)
		if err != nil || pn == 0 {
			errs = append(errs, errors.New("egress: "+e+" has no port"))
			continue
		}
		if a, err := netip.ParseAddr(host); err == nil {
			out = append(out, netip.AddrPortFrom(a.Unmap(), uint16(pn)))
			continue
		}
		addrs, err := r.LookupNetIP(ctx, "ip", host)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, a := range addrs {
			out = append(out, netip.AddrPortFrom(a.Unmap(), uint16(pn)))
		}
	}
	return out, errors.Join(errs...)
}

// Client is the client for the public internet only.
func Client(timeout time.Duration) *http.Client { return Policy{}.Client(timeout) }

// NoRedirects is a CheckRedirect that hands back the 3xx instead of following it.
func NoRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// notPublic is every range that is not one host on the public internet (IANA's special-purpose
// registries), and the IPv6 ranges that carry an IPv4 address inside them, which could be one of
// the ranges above.
var notPublic = prefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16",
	"3fff::/20", "5f00::/16",
)

// globalUnicast is the only IPv6 range assigned to hosts on the public internet.
var globalUnicast = netip.MustParsePrefix("2000::/3")

func prefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// Public says whether a is one host on the public internet. An IPv4 address written as IPv6
// (::ffff:10.0.0.1) is judged as the IPv4 address; an address with a zone never is public. It is
// pure.
func Public(a netip.Addr) bool {
	if !a.IsValid() || a.Zone() != "" {
		return false
	}
	a = a.Unmap()
	if a.Is6() && !globalUnicast.Contains(a) {
		return false
	}
	for _, p := range notPublic {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// PublicIP is Public for a net.IP.
func PublicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	return ok && Public(a)
}
