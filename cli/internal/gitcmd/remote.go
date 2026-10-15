package gitcmd

import (
	"net/url"
	"strings"
)

// RemoteDisagrees reports whether an existing remote URL names another repository than an
// app's git_url. An absent remote ("") agrees: the first deploy adds it. Pure.
func RemoteDisagrees(remote, gitURL string) bool {
	norm := func(u string) string { return strings.TrimSuffix(strings.TrimSpace(u), "/") }
	return norm(remote) != "" && norm(remote) != norm(gitURL)
}

// Redact drops any user name or password from a URL before it is printed. Pure.
func Redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}
