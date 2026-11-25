package dbtunnel

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestLocalAndRemote(t *testing.T) {
	remote := "postgres://app_x_ro_0a1b2c3d:pw@db.whisk.run:6543/app_x?sslmode=verify-full"
	local, err := Local(remote, "127.0.0.1:5433")
	if err != nil || local != "postgres://app_x_ro_0a1b2c3d:pw@127.0.0.1:5433/app_x?sslmode=disable" {
		t.Errorf("Local = %q, %v", local, err)
	}
	addr, name, err := Remote(remote)
	if err != nil || addr != "db.whisk.run:6543" || name != "db.whisk.run" {
		t.Errorf("Remote = %q %q %v", addr, name, err)
	}
	if _, _, err := Remote("postgres:///app_x"); err == nil {
		t.Error("a string with no host was taken")
	}
}

func message(code uint32, body []byte) []byte {
	b := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(b[:4], uint32(8+len(body)))
	binary.BigEndian.PutUint32(b[4:], code)
	return append(b, body...)
}

// selfSigned is a certificate for name and a pool that trusts it.
func selfSigned(t *testing.T, name string) (tls.Certificate, *x509.CertPool) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// A tool asking for TLS is told no, goes on in the clear, and its startup message reaches the
// proxy over TLS that checked the proxy's name; the proxy's answer comes back.
func TestTunnelCarriesAConnection(t *testing.T) {
	cert, pool := selfSigned(t, "db.test")
	proxy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer proxy.Close()
	startup := message(196608, []byte("user\x00app_x_ro_0a1b2c3d\x00\x00"))
	got := make(chan []byte, 1)
	go func() {
		c, err := proxy.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		pre := make([]byte, 8)
		if _, err := io.ReadFull(c, pre); err != nil || binary.BigEndian.Uint32(pre[4:]) != sslRequest {
			got <- nil
			return
		}
		_, _ = c.Write([]byte{'S'})
		s := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
		buf := make([]byte, len(startup))
		if _, err := io.ReadFull(s, buf); err != nil {
			got <- nil
			return
		}
		got <- buf
		_, _ = s.Write([]byte("R"))
	}()
	lis, _ := net.Listen("tcp", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		_, _ = Serve(ctx, Options{Listener: lis, Proxy: proxy.Addr().String(), Name: "db.test", TLS: &tls.Config{RootCAs: pool}, Until: time.Now().Add(time.Minute)})
	}()
	tool, err := net.Dial("tcp", lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close()
	_, _ = tool.Write(message(sslRequest, nil))
	no := make([]byte, 1)
	if _, err := io.ReadFull(tool, no); err != nil || no[0] != 'N' {
		t.Fatalf("the tunnel answered TLS with %q %v", no, err)
	}
	_, _ = tool.Write(startup)
	if b := <-got; !bytes.Equal(b, startup) {
		t.Fatalf("the proxy got %q", b)
	}
	answer := make([]byte, 1)
	if _, err := io.ReadFull(tool, answer); err != nil || answer[0] != 'R' {
		t.Fatalf("the tool got %q %v", answer, err)
	}
}

// A tunnel closes when the login stops working.
func TestTunnelEndsWithItsLogin(t *testing.T) {
	lis, _ := net.Listen("tcp", "127.0.0.1:0")
	done := make(chan error, 1)
	go func() {
		_, err := Serve(t.Context(), Options{Listener: lis, Proxy: "127.0.0.1:1", Name: "db.test", Until: time.Now().Add(100 * time.Millisecond)})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the tunnel outlived its login")
	}
}

func TestStartupRefusesAGiantMessage(t *testing.T) {
	var rw struct {
		io.Reader
		io.Writer
	}
	rw.Reader = bytes.NewReader(message(196608, make([]byte, maxStartup)))
	rw.Writer = io.Discard
	if _, err := Startup(rw); err == nil {
		t.Error("a startup message past the cap was read")
	}
}
