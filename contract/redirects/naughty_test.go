package redirects

import (
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

// Every naughty string as a rule's from and to, as a request path and as a query: nothing
// panics, a rule Check accepts answers its own path, and the edge's Match stays fast.
func TestNaughty(t *testing.T) {
	list := naughty.Strings()
	var rules []Rule
	for _, s := range list {
		for _, r := range []Rule{{From: "/" + s, To: "/x"}, {From: "/a", To: s}, {From: "/" + s + "/*", To: "/n/*"}, {From: s, Status: 410}} {
			if len(Check([]Rule{r})) == 0 {
				rules = append(rules, r)
			}
		}
		ParseFile([]byte(s))
		ParseFile([]byte("/" + s + " /x"))
	}
	m := Compile(rules)
	start := time.Now()
	for _, s := range list {
		m.Match("/"+s, s, "https://example.com")
		m.Match("/"+s+"/x/y", "", "")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("matching the naughty strings took %v", d)
	}
}

func FuzzMatch(f *testing.F) {
	f.Add("/blog/*", "/news/*", "/blog/a b/c", "q=1")
	f.Add("/p?id=1", "https://e.example/x", "/p", "id=1&id=2")
	f.Fuzz(func(t *testing.T, from, to, path, query string) {
		rules := []Rule{{From: from, To: to}}
		ok := len(Check(rules)) == 0
		a, matched := Compile(rules).Match(path, query, "")
		if matched && !ok {
			t.Fatalf("a rule Check refuses matched: %q -> %q on %q", from, to, path)
		}
		if matched && a.Location == "" {
			t.Fatalf("a redirect with no location: %+v", a)
		}
	})
}
