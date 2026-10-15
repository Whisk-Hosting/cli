package routes

import (
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// FuzzMatch checks the route glob against a plain reference (docs/HARNESS.md §8.3): a pattern
// without * matches exactly its own path, "/**" matches everything, a trailing slash never
// changes the answer, and a pattern always matches the path it spells when * stands for itself.
func FuzzMatch(f *testing.F) {
	for _, p := range []string{"/", "/**", "/api/*", "/a/**/b", "/*.css", "/reports/*/pdf"} {
		f.Add(p, "/api/x")
	}
	for _, s := range naughty.Strings() {
		f.Add(s, s)
		f.Add("/**", "/"+s)
		f.Add("/"+s+"/*", "/"+s+"/x")
	}
	f.Fuzz(func(t *testing.T, pattern, path string) {
		got := Match(pattern, path)
		if got != Match(pattern, path+"/") && !strings.HasSuffix(path, "/") {
			t.Fatalf("trailing slash changed %q against %q", pattern, path)
		}
		if !Match("/**", path) {
			t.Fatalf("/** does not match %q", path)
		}
		if !strings.Contains(pattern, "*") {
			if want := strings.TrimSuffix(pattern, "/") == strings.TrimSuffix(path, "/"); got != want && strings.HasPrefix(pattern, "/") == strings.HasPrefix(path, "/") {
				t.Fatalf("Match(%q, %q) = %v, exact comparison says %v", pattern, path, got, want)
			}
		}
		if !strings.Contains(path, "*") && !Match(path, path) {
			t.Fatalf("%q does not match itself", path)
		}
		if MatchAny([]string{pattern, "/**"}, path) != true {
			t.Fatalf("MatchAny missed /**")
		}
		// Every segment as * matches any path with the same number of non-empty segments.
		segs := segments(path)
		star := make([]string, len(segs))
		empty := false
		for i, s := range segs {
			star[i] = "*"
			empty = empty || s == ""
		}
		if !empty && !Match("/"+strings.Join(star, "/"), path) {
			t.Fatalf("%d stars do not match %q", len(segs), path)
		}
	})
}
