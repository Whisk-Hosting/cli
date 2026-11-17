package doctor

import (
	"reflect"
	"testing"
)

func TestOldLinks(t *testing.T) {
	cases := []struct {
		in   string
		want []oldLink
	}{
		{`<img src="/wp-content/uploads/2020/05/logo.jpg">`, []oldLink{{Path: "/wp-content/uploads/2020/05/logo.jpg", Line: 1}}},
		{"a\n<link href='/blog/wp-includes/css/x.css'>", []oldLink{{Path: "/blog/wp-includes/css/x.css", Line: 2}}},
		{`<a href="/product.php?id=12">`, []oldLink{{Path: "/product.php", Query: "id=12", Line: 1}}},
		{`background: url(/wp-content/themes/a/bg.png);`, []oldLink{{Path: "/wp-content/themes/a/bg.png", Line: 1}}},
		{`<img src="https://old.example.com/wp-content/uploads/a.jpg">`, nil},
		{`<a href="/blog/wp-admin-tips">`, nil},
		{`<a href="/docs/php-guide">`, nil},
		{`See wp-content/uploads in your old site.`, nil},
		{`<a href="/wp-admin/">`, nil},
	}
	for _, c := range cases {
		if got := oldLinks([]byte(c.in)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("oldLinks(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

// W177: a link to an old WordPress or PHP address that no file holds and no redirect sends on is
// answered 404 by the edge.
func TestW177OldAddressWithoutAFile(t *testing.T) {
	manifest := passing["whisk.yaml"]
	page := `<img src="/wp-content/uploads/2020/05/logo.jpg"><a href="/contact.php">Contact</a>`
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"links with no files", map[string]string{"public/index.html": page}, true},
		{"the files are in the repository", map[string]string{
			"public/index.html":                          page,
			"public/wp-content/uploads/2020/05/logo.jpg": "jpg",
			"public/contact.php":                         "<?php ?>",
		}, false},
		{"redirects send them on", map[string]string{
			"public/index.html": page,
			"whisk.yaml":        manifest + "redirects:\n  - from: /wp-content/*\n    to: /images/*\n  - from: /contact.php\n    to: /contact\n",
		}, false},
		{"only one is covered", map[string]string{
			"public/index.html": page,
			"whisk.yaml":        manifest + "redirects:\n  - from: /contact.php\n    to: /contact\n",
		}, true},
		{"a package's own page", map[string]string{"node_modules/x/index.html": page}, false},
		{"no old addresses", map[string]string{"public/index.html": `<img src="/images/logo.jpg">`}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W177"); got != c.want {
				t.Errorf("W177 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}
