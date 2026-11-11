package whisk

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// platformStore is whether goos verifies with an operating-system store (the Windows certificate
// store, the macOS keychain) instead of a bundle on disk.
func platformStore(goos string) bool {
	return goos == "windows" || goos == "darwin" || goos == "ios"
}

// extraRoots is the pool the CLI verifies with on Windows and macOS when SSL_CERT_FILE or
// SSL_CERT_DIR is set: the system store, with the certificates those name added to it. Go would
// otherwise trust only the file, so a stale copy of a proxy's root broke every command even
// though the store held the current one. A system pool with certificates added is checked both
// ways, and a certificate either one trusts is accepted. nil when Go's own choice stands: on any
// other system, where the variables replace the default bundle as OpenSSL's do, or when neither
// is set.
func extraRoots(goos string, getenv func(string) string, system func() (*x509.CertPool, error),
	readFile func(string) ([]byte, error), readDir func(string) ([]os.DirEntry, error)) *x509.CertPool {
	file, dirs := getenv("SSL_CERT_FILE"), getenv("SSL_CERT_DIR")
	if !platformStore(goos) || (file == "" && dirs == "") {
		return nil
	}
	pool, err := system()
	if err != nil || pool == nil {
		return nil
	}
	add := func(path string) {
		if pem, err := readFile(path); err == nil {
			pool.AppendCertsFromPEM(pem)
		}
	}
	if file != "" {
		add(file)
	}
	for _, dir := range filepath.SplitList(dirs) {
		entries, _ := readDir(dir)
		for _, e := range entries {
			if !e.IsDir() {
				add(filepath.Join(dir, e.Name()))
			}
		}
	}
	return pool
}

// TrustSystemStoreToo makes every HTTPS call the process makes verify with extraRoots, when it
// applies. The main package calls it once, before anything connects. Go loads the system pool
// once, and on Windows and macOS loads the file in place of the store while either variable is
// set, so they are left out of that one load and put back for the programs whisk runs, such as
// git.
func TrustSystemStoreToo() {
	file, dirs := os.Getenv("SSL_CERT_FILE"), os.Getenv("SSL_CERT_DIR")
	getenv := func(k string) string { return map[string]string{"SSL_CERT_FILE": file, "SSL_CERT_DIR": dirs}[k] }
	system := func() (*x509.CertPool, error) {
		_ = os.Unsetenv("SSL_CERT_FILE")
		_ = os.Unsetenv("SSL_CERT_DIR")
		defer restoreEnv("SSL_CERT_FILE", file)
		defer restoreEnv("SSL_CERT_DIR", dirs)
		return x509.SystemCertPool()
	}
	pool := extraRoots(runtime.GOOS, getenv, system, os.ReadFile, os.ReadDir)
	t, ok := http.DefaultTransport.(*http.Transport)
	if pool == nil || !ok {
		return
	}
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	t.TLSClientConfig.RootCAs = pool
}

func restoreEnv(key, value string) {
	if value != "" {
		_ = os.Setenv(key, value)
	}
}
