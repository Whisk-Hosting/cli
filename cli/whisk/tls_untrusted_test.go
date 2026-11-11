package whisk

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

func TestAPITLSUntrusted(t *testing.T) {
	unknown := fmt.Errorf("Get \"https://api.whisk.run/v1/me\": %w", x509.UnknownAuthorityError{})
	flattened := errors.New(`tls: failed to verify certificate: x509: certificate signed by unknown authority`)
	cases := []struct {
		name      string
		err       error
		goos      string
		env       map[string]string
		want      bool
		trusts    string
		inMessage string
		inFix     string
	}{
		{"refused connection is not TLS", errors.New("dial tcp: connection refused"), "linux", nil, false, "", "", ""},
		{"nil", nil, "linux", nil, false, "", "", ""},
		{"SSL_CERT_FILE on Windows trusts the store too", unknown, "windows", map[string]string{"SSL_CERT_FILE": `C:\certs\bundle.pem`}, true, "system+env", `the Windows certificate store and the certificates in SSL_CERT_FILE=C:\certs\bundle.pem`, "add its current root"},
		{"SSL_CERT_FILE on Linux replaces the bundle", unknown, "linux", map[string]string{"SSL_CERT_FILE": "/a.pem"}, true, "env", "only the certificates in SSL_CERT_FILE=/a.pem", "Unset the variable"},
		{"SSL_CERT_DIR", flattened, "linux", map[string]string{"SSL_CERT_DIR": "/etc/certs"}, true, "env", "SSL_CERT_DIR=/etc/certs", "proxy or antivirus"},
		{"both variables on macOS", unknown, "darwin", map[string]string{"SSL_CERT_FILE": "/a.pem", "SSL_CERT_DIR": "/d"}, true, "system+env", "macOS keychain and the certificates in SSL_CERT_FILE=/a.pem and SSL_CERT_DIR=/d", "re-signing traffic"},
		{"system store", flattened, "windows", nil, true, "system", "Windows certificate store", "re-signing traffic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := apiTLSUntrusted(c.err, c.goos, func(k string) string { return c.env[k] })
			if (e != nil) != c.want {
				t.Fatalf("got %v, want TLS_UNTRUSTED %v", e, c.want)
			}
			if e == nil {
				return
			}
			if e.Code != "TLS_UNTRUSTED" || e.Details["trusts"] != c.trusts || !strings.Contains(e.Message, c.inMessage) || !strings.Contains(e.Fix, c.inFix) {
				t.Fatalf("got %+v", e)
			}
			if strings.Contains(strings.ToLower(e.Fix), "retry") {
				t.Fatalf("a certificate error must not say retry: %q", e.Fix)
			}
		})
	}
}

// A platform behind a certificate the CLI does not trust answers TLS_UNTRUSTED, never the
// "retry with backoff" of PLATFORM_UNAVAILABLE.
func TestUntrustedPlatformCertificateIsTLSUntrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude"}, &memStore{m: map[string]config.Credential{}}, "whoami", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code == 0 || e == nil || e["code"] != "TLS_UNTRUSTED" {
		t.Fatalf("whoami against an untrusted certificate: %+v", r)
	}
}
