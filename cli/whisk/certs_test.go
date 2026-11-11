package whisk

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestExtraRoots(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cert := srv.Certificate()
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	files := map[string][]byte{"/certs/bundle.pem": bundle}
	readFile := func(p string) ([]byte, error) {
		if b, ok := files[p]; ok {
			return b, nil
		}
		return nil, os.ErrNotExist
	}
	system := func() (*x509.CertPool, error) { return x509.NewCertPool(), nil }
	cases := []struct {
		name    string
		goos    string
		env     map[string]string
		system  func() (*x509.CertPool, error)
		pool    bool
		trusted bool
	}{
		{"Linux keeps Go's own choice", "linux", map[string]string{"SSL_CERT_FILE": "/certs/bundle.pem"}, system, false, false},
		{"no variable keeps the store alone", "windows", nil, system, false, false},
		{"Windows adds the file to the store", "windows", map[string]string{"SSL_CERT_FILE": "/certs/bundle.pem"}, system, true, true},
		{"macOS adds the file to the keychain", "darwin", map[string]string{"SSL_CERT_FILE": "/certs/bundle.pem"}, system, true, true},
		{"a missing file still keeps the store", "windows", map[string]string{"SSL_CERT_FILE": "/gone.pem"}, system, true, false},
		{"no store keeps Go's own choice", "windows", map[string]string{"SSL_CERT_FILE": "/certs/bundle.pem"}, func() (*x509.CertPool, error) { return nil, errors.New("no store") }, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := extraRoots(c.goos, func(k string) string { return c.env[k] }, c.system, readFile, func(string) ([]os.DirEntry, error) { return nil, os.ErrNotExist })
			if (pool != nil) != c.pool {
				t.Fatalf("pool %v, want %v", pool != nil, c.pool)
			}
			if pool == nil {
				return
			}
			_, err := cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.com"})
			if (err == nil) != c.trusted {
				t.Fatalf("verify: %v, want trusted %v", err, c.trusted)
			}
		})
	}
}

func TestExtraRootsReadsCertDir(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/proxy.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	pool := extraRoots("windows", func(k string) string { return map[string]string{"SSL_CERT_DIR": dir}[k] },
		func() (*x509.CertPool, error) { return x509.NewCertPool(), nil }, os.ReadFile, os.ReadDir)
	if _, err := srv.Certificate().Verify(x509.VerifyOptions{Roots: pool, DNSName: "example.com"}); err != nil {
		t.Fatalf("a certificate in SSL_CERT_DIR is trusted: %v", err)
	}
}
