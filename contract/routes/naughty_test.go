package routes

import (
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

// Every naughty string as a pattern and as a path: a path always matches itself as a pattern,
// a pattern without * matches only its own path, and nothing takes long, because the edge
// classifies every request with these patterns.
func TestNaughtyMatch(t *testing.T) {
	list := naughty.Strings()
	for _, s := range list {
		for _, p := range []string{s, "/" + s, "/a/" + s + "/b"} {
			if !Match(p, p) {
				t.Errorf("Match(%q, itself) is false", p)
			}
			if !Match("/**", "/"+strings.TrimPrefix(p, "/")) {
				t.Errorf("/** does not match %q", p)
			}
		}
		if !strings.Contains(s, "*") {
			for _, other := range []string{"/", "/a", "/" + s + "x", "/x" + s} {
				if Match("/"+s, other) && strings.TrimSuffix("/"+s, "/") != strings.TrimSuffix(other, "/") {
					t.Errorf("literal pattern %q matched %q", "/"+s, other)
				}
			}
		}
	}
	for _, pattern := range []string{"/*", "/**", "/a/*/b", "/a/**/b", "/*.pdf", "/**/x"} {
		for _, s := range list {
			if strings.Contains(s, "/") {
				continue
			}
			if Match(pattern, "/"+s) && pattern == "/a/*/b" {
				t.Errorf("%s matched the one-segment path %q", pattern, "/"+s)
			}
			if pattern == "/*" && s != "" && !Match(pattern, "/"+s) {
				t.Errorf("/* does not match the one-segment path %q", "/"+s)
			}
		}
	}
}

// A manifest may hold up to 200 characters of ** segments and a request path may be long; the
// match stays proportional to their product instead of growing with every **.
func TestNaughtyMatchIsBounded(t *testing.T) {
	stars := "/" + strings.TrimSuffix(strings.Repeat("**/", 66), "/") + "/x"
	path := "/" + strings.TrimSuffix(strings.Repeat("a/", 2000), "/")
	start := time.Now()
	if Match(stars, path) {
		t.Fatal("a pattern ending in /x matched a path without x")
	}
	if !Match(stars, path+"/x") {
		t.Fatal("a pattern of ** segments did not match a path ending in x")
	}
	long := naughty.Strings()[len(naughty.Strings())-1]
	Match("/*a*b*c*d*e*f*", "/"+long)
	Match(stars, "/"+long)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("matching took %s", d)
	}
}
