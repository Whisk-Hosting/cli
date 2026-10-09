package redirects

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var site = []Rule{
	{From: "/about-us", To: "/about"},
	{From: "/contact.html", To: "/contact", Status: 308},
	{From: "/blog/*", To: "/news/*"},
	{From: "/blog/2019/*", To: "https://archive.example.com/*"},
	{From: "/shop/*", To: "https://shop.example.com/"},
	{From: "/product.php?id=12", To: "/products/widget"},
	{From: "/product.php?id=12&lang=fr", To: "/fr/products/widget"},
	{From: "/product.php", To: "/products"},
	{From: "/old-promo", Status: 410},
	{From: "/temp", To: "/sale", Status: 302, Query: QueryDrop},
	{From: "/caf%C3%A9", To: "/cafe"},
	{From: "/search", To: "/find?from=old", Query: QueryKeep},
}

func TestMatch(t *testing.T) {
	m := Compile(site)
	cases := []struct {
		path, query, base string
		status            int
		loc               string
	}{
		{"/about-us", "", "", 301, "/about"},
		{"/about-us/", "", "", 301, "/about"},
		{"/about-us", "utm_source=x", "", 301, "/about?utm_source=x"},
		{"/contact.html", "", "", 308, "/contact"},
		{"/blog", "", "", 301, "/news/"},
		{"/blog/", "", "", 301, "/news/"},
		{"/blog/a-post", "", "", 301, "/news/a-post"},
		{"/blog/a/b/", "p=2", "", 301, "/news/a/b/?p=2"},
		{"/blog/a b", "", "", 301, "/news/a%20b"},
		{"/blog/2019/x", "", "", 301, "https://archive.example.com/x"},
		{"/shop/cart", "", "", 301, "https://shop.example.com/"},
		{"/product.php", "id=12", "", 301, "/products/widget"},
		{"/product.php", "lang=fr&id=12&x=1", "", 301, "/fr/products/widget"},
		{"/product.php", "id=13", "", 301, "/products?id=13"},
		{"/old-promo", "", "", 410, ""},
		{"/temp", "a=1", "", 302, "/sale"},
		{"/café", "", "", 301, "/cafe"},
		{"/search", "q=x", "", 301, "/find?from=old&q=x"},
		{"/about-us", "", "https://new.example.com", 301, "https://new.example.com/about"},
		{"/shop/x", "", "https://new.example.com", 301, "https://shop.example.com/"},
	}
	for _, c := range cases {
		a, ok := m.Match(c.path, c.query, c.base)
		if !ok || a.Status != c.status || a.Location != c.loc {
			t.Errorf("Match(%q, %q, %q) = %+v %v, want %d %q", c.path, c.query, c.base, a, ok, c.status, c.loc)
		}
	}
	for _, p := range []string{"/", "/about", "/blogger", "/about-us/team", "/Blog/x", "/news/x"} {
		if a, ok := m.Match(p, "", ""); ok {
			t.Errorf("Match(%q) = %+v, want no match", p, a)
		}
	}
}

func TestMatchRootPrefix(t *testing.T) {
	m := Compile([]Rule{{From: "/*", To: "https://new.example.com/*"}, {From: "/keep", To: "/kept"}})
	if a, _ := m.Match("/x/y", "a=1", ""); a.Location != "https://new.example.com/x/y?a=1" {
		t.Errorf("root prefix = %q", a.Location)
	}
	if a, _ := m.Match("/", "", ""); a.Location != "https://new.example.com/" {
		t.Errorf("root = %q", a.Location)
	}
	if a, _ := m.Match("/keep", "", ""); a.Location != "/kept" {
		t.Errorf("an exact rule loses to the root prefix: %q", a.Location)
	}
}

func TestCheck(t *testing.T) {
	if ps := Check(site); len(ps) != 0 {
		t.Fatalf("Check(site) = %v", ps)
	}
	cases := []struct {
		rule  Rule
		field string
		want  string
	}{
		{Rule{To: "/x"}, "from", "required"},
		{Rule{From: "about", To: "/x"}, "from", "not a path"},
		{Rule{From: "//evil.example", To: "/x"}, "from", "not a path"},
		{Rule{From: "/a b", To: "/x"}, "from", "space"},
		{Rule{From: "/a/*/b", To: "/x"}, "from", "may only end"},
		{Rule{From: "/a*", To: "/x"}, "from", "may only end"},
		{Rule{From: "/%zz", To: "/x"}, "from", "percent"},
		{Rule{From: "/.whisk/media/x", To: "/x"}, "from", "platform"},
		{Rule{From: "/.well-known/acme-challenge/t", To: "/x"}, "from", "platform"},
		{Rule{From: "/a/*?x=1", To: "/x"}, "from", "prefix rule cannot name a query"},
		{Rule{From: "/a?" + strings.Repeat("&p=1", 1) + "&a=1&b=1&c=1&d=1&e=1&f=1&g=1&h=1&i=1&j=1", To: "/x"}, "from", "query parameters"},
		{Rule{From: "/a"}, "to", "required"},
		{Rule{From: "/a", To: "/x/*"}, "to", "must end in /* too"},
		{Rule{From: "/a/*", To: "/x/*/y"}, "to", "may only end"},
		{Rule{From: "/a", To: "javascript:alert(1)"}, "to", "neither"},
		{Rule{From: "/a", To: "//evil.example/x"}, "to", "neither"},
		{Rule{From: "/a", To: "https://user@evil.example/"}, "to", "neither"},
		{Rule{From: "/a", To: "ftp://x.example/"}, "to", "neither"},
		{Rule{From: "/a", To: "/x", Status: 410}, "to", "remove to"},
		{Rule{From: "/a", To: "/x", Status: 303}, "status", "not a redirect status"},
		{Rule{From: "/a", To: "/x", Query: "merge"}, "query", "not keep or drop"},
	}
	for _, c := range cases {
		ps := Check([]Rule{c.rule})
		if len(ps) == 0 || ps[0].Field != c.field || !strings.Contains(ps[0].Message, c.want) {
			t.Errorf("Check(%+v) = %v, want %s: %q", c.rule, ps, c.field, c.want)
		}
	}
}

func TestCheckDuplicatesAndLoops(t *testing.T) {
	dup := Check([]Rule{{From: "/a", To: "/b"}, {From: "/a/", To: "/c"}})
	if len(dup) != 1 || dup[0].Index != 1 || !strings.Contains(dup[0].Message, "already redirected by redirect 0") {
		t.Errorf("duplicate = %v", dup)
	}
	q := Check([]Rule{{From: "/p?a=1&b=2", To: "/b"}, {From: "/p?b=2&a=1", To: "/c"}})
	if len(q) != 1 {
		t.Errorf("the same query in another order = %v", q)
	}
	loops := [][]Rule{
		{{From: "/a", To: "/a/"}},
		{{From: "/a", To: "/b"}, {From: "/b", To: "/a"}},
		{{From: "/a/*", To: "/a/b/*"}},
		{{From: "/blog/*", To: "/news/*"}, {From: "/news/*", To: "/blog/*"}},
		{{From: "/x", To: "/y?z=1"}, {From: "/y?z=1", To: "/x"}},
	}
	for _, rules := range loops {
		ps := Check(rules)
		if len(ps) == 0 || !strings.Contains(ps[0].Message, "loop") {
			t.Errorf("Check(%v) = %v, want a loop", rules, ps)
		}
	}
	if ps := Check([]Rule{{From: "/x", To: "/y?z=1"}, {From: "/y?z=2", To: "/x"}}); len(ps) != 0 {
		t.Errorf("different queries are not a loop: %v", ps)
	}
}

func TestCheckLimit(t *testing.T) {
	rules := make([]Rule, MaxRules+1)
	for i := range rules {
		rules[i] = Rule{From: fmt.Sprintf("/p%d", i), To: "/"}
	}
	if ps := Check(rules); len(ps) != 1 || !strings.Contains(ps[0].Message, "at most 10000") {
		t.Errorf("over the limit = %v", ps)
	}
	if ps := Check(rules[:MaxRules]); len(ps) != 0 {
		t.Errorf("at the limit = %v", ps[:1])
	}
}

func TestChains(t *testing.T) {
	rules := []Rule{
		{From: "/a", To: "/b"},
		{From: "/b", To: "/c"},
		{From: "/c", To: "/d"},
		{From: "/old/*", To: "/blog/*"},
		{From: "/blog/*", To: "/news/*"},
		{From: "/e", To: "https://elsewhere.example/b"},
	}
	got := Chains(rules)
	want := []Chain{
		{Index: 0, Hops: []string{"/a", "/b", "/c", "/d"}},
		{Index: 1, Hops: []string{"/b", "/c", "/d"}},
		{Index: 3, Hops: []string{"/old/*", "/blog/whisk-sample", "/news/whisk-sample"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chains = %+v\nwant %+v", got, want)
	}
	if c := Chains(site); len(c) != 0 {
		t.Errorf("Chains(site) = %+v", c)
	}
}

func TestParseFile(t *testing.T) {
	src := "\ufeff# moved from the old site\n\n/about-us /about\n/contact.html\t/contact 308\n/blog/* /news/*\n/old-promo 410\n/temp /sale 302 query=drop\n/bad\n/x /y 30x\n/x /y 302 query=drop extra\n"
	p, bad := ParseFile([]byte(src))
	want := []Rule{
		{From: "/about-us", To: "/about"},
		{From: "/contact.html", To: "/contact", Status: 308},
		{From: "/blog/*", To: "/news/*"},
		{From: "/old-promo", Status: 410},
		{From: "/temp", To: "/sale", Status: 302, Query: QueryDrop},
	}
	if !reflect.DeepEqual(p.Rules, want) || !reflect.DeepEqual(p.Lines, []int{3, 4, 5, 6, 7}) {
		t.Errorf("ParseFile = %+v", p)
	}
	if len(bad) != 3 || bad[0].Line != 8 || bad[1].Line != 9 || bad[2].Line != 10 {
		t.Errorf("problems = %v", bad)
	}
	again, bad := ParseFile(Format(want))
	if len(bad) != 0 || !reflect.DeepEqual(again.Rules, want) {
		t.Errorf("ParseFile(Format) = %+v %v", again.Rules, bad)
	}
	if _, bad := ParseFile(make([]byte, MaxFileBytes+1)); len(bad) != 1 {
		t.Errorf("an oversized file = %v", bad)
	}
}

// Ten thousand rules compile and answer a request in microseconds: the edge runs Match on
// every request to an app with redirects.
func BenchmarkMatch(b *testing.B) {
	rules := make([]Rule, 0, MaxRules)
	for i := range MaxRules / 2 {
		rules = append(rules, Rule{From: fmt.Sprintf("/page-%d", i), To: fmt.Sprintf("/p/%d", i)})
		rules = append(rules, Rule{From: fmt.Sprintf("/section-%d/*", i), To: fmt.Sprintf("/s/%d/*", i)})
	}
	m := Compile(rules)
	b.ResetTimer()
	for i := range b.N {
		m.Match(fmt.Sprintf("/section-%d/a/b/c/d", i%5000), "x=1", "")
	}
}

func TestForMethod(t *testing.T) {
	for _, c := range []struct {
		status int
		method string
		want   int
	}{{301, "GET", 301}, {301, "HEAD", 301}, {301, "POST", 308}, {302, "PUT", 307}, {308, "POST", 308}, {410, "POST", 410}} {
		if got := ForMethod(c.status, c.method); got != c.want {
			t.Errorf("ForMethod(%d, %s) = %d, want %d", c.status, c.method, got, c.want)
		}
	}
}

func TestPatterns(t *testing.T) {
	m := Compile([]Rule{
		{From: "/blog/:year/:month/:slug", To: "/posts/:slug"},
		{From: "/blog/2020/:month/:slug", To: "/archive/2020/:slug"},
		{From: "/shop/:category/:id", To: "https://shop.example.com/:category/p/:id?ref=old"},
		{From: "/blog/:year/*", To: "/years/:year"},
		{From: "/blog/*", To: "/posts/*"},
		{From: "/blog/2019/05/launch", To: "/news/launch"},
		{From: "/wiki/Special:Search", To: "/search"},
	})
	cases := []struct{ path, query, loc string }{
		{"/blog/2019/05/hello", "", "/posts/hello"},
		{"/blog/2019/05/hello/", "", "/posts/hello"},
		{"/blog/2020/01/hello", "", "/archive/2020/hello"},
		{"/blog/2019/05/launch", "", "/news/launch"},
		{"/blog/2019/05/a b", "x=1", "/posts/a%20b?x=1"},
		{"/shop/chairs/12", "", "https://shop.example.com/chairs/p/12?ref=old"},
		{"/blog/2019/05", "", "/posts/2019/05"},
		{"/blog/x", "", "/posts/x"},
		{"/wiki/Special:Search", "", "/search"},
	}
	for _, c := range cases {
		a, ok := m.Match(c.path, c.query, "")
		if !ok || a.Location != c.loc {
			t.Errorf("Match(%q) = %+v %v, want %q", c.path, a, ok, c.loc)
		}
	}
	bad := map[string]Rule{
		"appears twice":            {From: "/a/:x/:x", To: "/b"},
		"cannot also end in /*":    {From: "/a/:x/*", To: "/b"},
		"cannot also end in /* or": {From: "/a/:x?q=1", To: "/b"},
	}
	for want, r := range bad {
		if ps := Check([]Rule{r}); len(ps) == 0 || !strings.Contains(ps[0].Message, want) {
			t.Errorf("Check(%+v) = %v, want %q", r, ps, want)
		}
	}
	if ps := Check([]Rule{{From: "/a/:x", To: "/b"}, {From: "/a/:y", To: "/c"}}); len(ps) != 1 {
		t.Errorf("two patterns matching the same requests = %v", ps)
	}
	if ps := Check([]Rule{{From: "/a/:x", To: "/b/:x"}, {From: "/b/:y", To: "/a/:y"}}); len(ps) == 0 || !strings.Contains(ps[0].Message, "loop") {
		t.Errorf("a pattern loop = %v", ps)
	}
	many := make([]Rule, MaxPatterns+1)
	for i := range many {
		many[i] = Rule{From: fmt.Sprintf("/p%d/:x", i), To: "/"}
	}
	if ps := Check(many); len(ps) != 1 || !strings.Contains(ps[0].Message, "at most 1000") {
		t.Errorf("too many patterns = %v", ps)
	}
}
