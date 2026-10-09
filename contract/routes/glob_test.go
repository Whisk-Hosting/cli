package routes

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/", "/", true},
		{"/", "/x", false},
		{"/health", "/health", true},
		{"/health", "/health/", true},
		{"/health", "/healthz", false},
		{"/public/*", "/public/a", true},
		{"/public/*", "/public/a/b", false},
		{"/public/*", "/public", false},
		{"/public/**", "/public", true},
		{"/public/**", "/public/a/b/c", true},
		{"/public/**", "/private/a", false},
		{"/assets/*.css", "/assets/site.css", true},
		{"/assets/*.css", "/assets/site.js", false},
		{"/assets/*.css", "/assets/x/site.css", false},
		{"/**/edit", "/a/b/edit", true},
		{"/**/edit", "/edit", true},
		{"/**", "/anything/at/all", true},
		{"/**", "/", true},
		{"/api/*/items/**", "/api/v1/items", true},
		{"/api/*/items/**", "/api/v1/items/1/tags", true},
		{"/api/*/items/**", "/api/items/1", false},
		{"/a*b", "/ab", true},
		{"/a*b", "/axyzb", true},
		{"/a*b", "/axyz", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.path); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	public := []string{"/", "/public/**", "/health"}
	if !MatchAny(public, "/health") || !MatchAny(public, "/public/x/y") || MatchAny(public, "/notes") {
		t.Fatal("MatchAny classification wrong")
	}
	if MatchAny(nil, "/") {
		t.Fatal("empty list must match nothing")
	}
}
