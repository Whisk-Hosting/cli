package connect

import (
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// FuzzTemplate (HARNESS.md §8.3): any text parses or is refused without panicking, and a
// template that parses evaluates or fails cleanly, whatever the secrets and request hold.
func FuzzTemplate(f *testing.F) {
	f.Add("Bearer {secret.API_KEY}", "k", "/a")
	f.Add("{base64(hmac_sha256(secret.API_KEY, concat(request.method, request.path)))}", "k", "/a")
	f.Add("{jwt('HS256', secret.API_KEY, json('a', time.unix, 'exp', add(time.unix, 60)))}", "k", "/a")
	for _, s := range naughty.Strings() {
		f.Add(s, s, s)
		f.Add("{"+s+"}", s, "/"+s)
	}
	f.Fuzz(func(t *testing.T, src, secret, path string) {
		tm, _, err := ParseTemplate(src, Context{Request: true, Token: true})
		if err != nil {
			return
		}
		_, _ = tm.Eval(env(map[string]string{"API_KEY": secret}, &Request{Method: "GET", Path: path, Query: path, Body: []byte(path)}))
	})
}

// FuzzMatch: a path with a dot segment or an encoded separator never matches, whatever the
// operations say.
func FuzzMatch(f *testing.F) {
	for _, s := range naughty.Strings() {
		f.Add(s, "/"+s)
	}
	f.Add("/**", "/a/%2e%2e/b")
	f.Fuzz(func(t *testing.T, pattern, path string) {
		_, ok := Match([]Operation{{Name: "x", Method: "*", Path: pattern}}, "GET", path, nil)
		lower := strings.ToLower(path)
		if ok && (strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "/../") || strings.HasSuffix(path, "/..")) {
			t.Fatalf("%q matched %q", path, pattern)
		}
	})
}
