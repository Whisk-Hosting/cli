package cdnpurge

import (
	"strings"
	"testing"
)

func TestPathProblem(t *testing.T) {
	for p, want := range map[string]string{
		"/":                             "",
		"/entries/42":                   "",
		"/entries/*":                    "",
		"entries":                       "does not start with /",
		"/a b":                          "holds a space",
		"/a?b=1":                        "holds a space",
		"/a#b":                          "holds a space",
		"/a*/b":                         "before its end",
		"/" + strings.Repeat("a", 1024): "longer than 1024",
	} {
		if got := PathProblem(p); (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("%q: got %q, want %q", p, got, want)
		}
	}
}
