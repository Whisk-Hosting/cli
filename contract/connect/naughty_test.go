package connect

import (
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// Naughty strings (HARNESS.md §8) as secret values, paths and recipe text: nothing panics, and
// no rendered header value carries a line break.
func TestNaughty(t *testing.T) {
	c := conn()
	for _, s := range naughty.Strings() {
		placed, err := Apply(c, env(map[string]string{"SIGN": s}, &Request{Method: "GET", Path: s, Query: s, Body: []byte(s)}))
		if err == nil {
			for k, vs := range placed.Headers {
				for _, v := range vs {
					if strings.ContainsAny(v, "\r\n") {
						t.Fatalf("%s carries a line break for %q", k, s)
					}
				}
			}
		}
		_, _ = Match(c.Operations, "GET", s, nil)
		bad := c
		bad.URL = s
		_, _ = Check(bad)
		_ = Usage(bad)
		_ = Describe(bad)
	}
}
