package routes

import "testing"

func TestCoversAll(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{"the catch-all", []string{"/**"}, true},
		{"the catch-all with a trailing slash", []string{"/**/"}, true},
		{"the catch-all beside others", []string{"/", "/health", "/**"}, true},
		{"two ** in a row", []string{"/**/**"}, true},
		{"home, one segment and the rest", []string{"/", "/*", "/*/**"}, true},
		{"home and one segment or more", []string{"/", "/*/**"}, true},
		{"none", nil, false},
		{"empty list", []string{}, false},
		{"home only", []string{"/"}, false},
		{"everything but home", []string{"/*/**"}, false},
		{"one segment only", []string{"/*"}, false},
		{"home and one segment, no deeper", []string{"/", "/*"}, false},
		{"home and two segments, not one", []string{"/", "/*/*/**"}, false},
		{"a folder's catch-all", []string{"/public/**"}, false},
		{"every folder but with a literal", []string{"/**/edit"}, false},
		{"a partial wildcard segment", []string{"/a*/**", "/"}, false},
		{"the starter's routes", []string{"/", "/health", "/whoami"}, false},
		{"a triple star segment then **", []string{"/", "/***", "/***/**"}, true},
	}
	for _, c := range cases {
		if got := CoversAll(c.patterns); got != c.want {
			t.Errorf("%s: CoversAll(%q) = %v, want %v", c.name, c.patterns, got, c.want)
		}
	}
}

// TestCoversAllAgreesWithMatch checks the answer against Match over paths of every shape the
// patterns could tell apart: a pattern set that covers everything matches each of them.
func TestCoversAllAgreesWithMatch(t *testing.T) {
	paths := []string{"/", "/a", "/a/b", "/a/b/c", "/edit", "/public/x", "/x/y/z/w/v"}
	sets := [][]string{{"/**"}, {"/", "/*", "/*/**"}, {"/*/**"}, {"/", "/*"}, {"/**/edit"}}
	for _, s := range sets {
		all := true
		for _, p := range paths {
			all = all && MatchAny(s, p)
		}
		if CoversAll(s) && !all {
			t.Errorf("CoversAll(%q) but a path is not matched", s)
		}
	}
}
