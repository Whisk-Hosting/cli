package routes

import "testing"

func TestAmbiguous(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/", false},
		{"/private", false},
		{"/public/a/b", false},
		{"/public/a.b/c..d/...", false},
		{"/.well-known/x", false},
		{"/.whisk/login", false},
		{"/files/report%20q3.pdf", false},
		{"/public/..", true},
		{"/public/../private", true},
		{"/public/./x", true},
		{"/public/%2e%2e/private", true},
		{"/public/%2E%2E/private", true},
		{"/public/.%2e/private", true},
		{"/public/%2e/x", true},
		{"/public/..%2fprivate", true},
		{"/public%2F..%2Fprivate", true},
		{"/public/..\\private", true},
		{"/public/%5c../private", true},
		{"/public/%zz", true},
		{"/files/a%2Fb", false},
		{"/files/a%5Cb", false},
		{"/files/a\\b", false},
		{"/public/../private?x=1", true},
		{"/ok?next=/a/../b", false},
		{"/public/%252e%252e/private", false},
	}
	for _, c := range cases {
		if got := Ambiguous(c.path); got != c.want {
			t.Errorf("Ambiguous(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// Every path that classifies differently from its cleaned form is ambiguous: an unambiguous
// path's segments are exactly the segments the app routes on.
func TestAmbiguousCoversDotTraversal(t *testing.T) {
	for _, p := range []string{"/public/../admin", "/public/%2e%2e/admin", "/public/x/../../admin"} {
		if Match("/public/**", p) && !Ambiguous(p) {
			t.Errorf("%q matches /public/** and is not refused", p)
		}
	}
}
