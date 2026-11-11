package whisk

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"

	"github.com/whisk-run/cli/internal/output"
)

// apiTLSUntrusted is TLS_UNTRUSTED: the CLI reached the platform but refused its certificate.
// Retrying never fixes that, so it is told apart from PLATFORM_UNAVAILABLE, and the message
// names the certificates the CLI trusted: on Windows and macOS the system store plus the file
// or directory SSL_CERT_FILE and SSL_CERT_DIR name (extraRoots), elsewhere those in place of the
// system's bundle when set, otherwise the system store alone. nil for any other error.
func apiTLSUntrusted(err error, goos string, getenv func(string) string) *output.Error {
	if !isTLSUntrusted(err) {
		return nil
	}
	details := map[string]any{"cause": err.Error()}
	var trusts string
	switch file, dir := getenv("SSL_CERT_FILE"), getenv("SSL_CERT_DIR"); {
	case file != "" && dir != "":
		trusts = "only the certificates in SSL_CERT_FILE=" + file + " and SSL_CERT_DIR=" + dir
		details["ssl_cert_file"], details["ssl_cert_dir"] = file, dir
	case file != "":
		trusts = "only the certificates in SSL_CERT_FILE=" + file
		details["ssl_cert_file"] = file
	case dir != "":
		trusts = "only the certificates in SSL_CERT_DIR=" + dir
		details["ssl_cert_dir"] = dir
	}
	switch {
	case trusts != "" && platformStore(goos):
		details["trusts"] = "system+env"
		return output.New("TLS_UNTRUSTED", "whisk does not trust the platform's certificate. It is trusting the "+systemStore(goos)+" and "+strings.TrimPrefix(trusts, "only ")+".",
			"Neither holds the root that signed it. Something on this machine or network, usually an HTTPS-inspecting proxy or antivirus, is re-signing traffic: add its current root to the "+systemStore(goos)+", then run the command again.", details)
	case trusts != "":
		details["trusts"] = "env"
		return output.New("TLS_UNTRUSTED", "whisk does not trust the platform's certificate. It is trusting "+trusts+".",
			"Unset the variable so whisk uses the "+systemStore(goos)+", or update the file with the current root certificate. A root that changed is usually an HTTPS-inspecting proxy or antivirus that re-signs traffic and has rotated its certificate. Then run the command again.", details)
	}
	details["trusts"] = "system"
	return output.New("TLS_UNTRUSTED", "whisk does not trust the platform's certificate. It is trusting the "+systemStore(goos)+".",
		"Something on this machine or network, usually an HTTPS-inspecting proxy or antivirus, is re-signing traffic with a root the "+systemStore(goos)+" does not hold. Add that root to the store (or point SSL_CERT_FILE at a bundle that includes it), then run the command again.", details)
}

// isTLSUntrusted is whether err is a failed certificate check, as Go reports it or, when a
// wrapper flattened it to text, as its message reads.
func isTLSUntrusted(err error) bool {
	if err == nil {
		return false
	}
	var (
		unknown  x509.UnknownAuthorityError
		invalid  x509.CertificateInvalidError
		hostname x509.HostnameError
		verify   *tls.CertificateVerificationError
	)
	if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &hostname) || errors.As(err, &verify) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "x509: ") || strings.Contains(msg, "tls: failed to verify certificate")
}

// systemStore names the store Go verifies with when no variable overrides it.
func systemStore(goos string) string {
	switch goos {
	case "windows":
		return "Windows certificate store"
	case "darwin":
		return "macOS keychain"
	default:
		return "system's CA certificates"
	}
}
