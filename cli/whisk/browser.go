package whisk

import (
	"context"
	"net"
	"net/url"
	"runtime"

	"github.com/whisk-run/contract/run"
)

// openBrowser tries to open a URL for a human and stays silent when it cannot; the URL is
// always printed as well, so nothing depends on this working. The URL comes from the platform,
// and the system opener runs whatever it is handed (a file:// path or a share on Windows runs
// a program), so only a web address is opened.
func openBrowser(raw string) {
	if !webAddress(raw) {
		return
	}
	name, args := opener(runtime.GOOS, raw)
	_ = run.Process(context.Background(), name, args...).Start()
}

// opener is the system's command that opens a URL in the default browser. The opener may
// become the browser itself, so it runs without a deadline and is left running when it starts.
func opener(goos, raw string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{raw}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", raw}
	}
	return "xdg-open", []string{raw}
}

// webAddress is an https URL with a host, or http to the machine itself for a local stack.
func webAddress(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		host := u.Hostname()
		ip := net.ParseIP(host)
		return host == "localhost" || (ip != nil && ip.IsLoopback())
	}
	return false
}
