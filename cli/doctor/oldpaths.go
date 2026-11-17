package doctor

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/redirects"
)

// oldAddress is an address on the app's own site that Whisk's edge answers 404 itself unless the
// app holds files at such paths (CADDY.md §4.4): a WordPress folder, a .php page, a cgi-bin
// script. It is matched where a page or code links to it: after a quote, a bracket, an equals
// sign or a space.
var oldAddress = regexp.MustCompile(`(?:^|["'(=\s])(/(?:[A-Za-z0-9._~%-]+/)*(?:(?:wp-content|wp-includes|wp-admin|cgi-bin)/[^\s"'()<>?#\\]*[^\s"'()<>?#\\./]|[A-Za-z0-9._~%-]+\.php))(\?[^\s"'()<>#\\]*)?`)

// oldLinkFiles are the files a link to a page or a picture is written in.
var oldLinkFiles = map[string]bool{
	".html": true, ".htm": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true,
	".css": true, ".scss": true, ".md": true, ".mdx": true, ".astro": true, ".vue": true,
	".svelte": true, ".json": true, ".py": true, ".go": true,
}

// oldLink is one link to an old address: its path, its query and its line.
type oldLink struct {
	Path, Query string
	Line        int
}

// oldLinks finds the links to old addresses in a file's content, in order. Pure.
func oldLinks(content []byte) []oldLink {
	var out []oldLink
	for i, line := range bytes.Split(content, []byte("\n")) {
		for _, m := range oldAddress.FindAllSubmatch(line, -1) {
			out = append(out, oldLink{Path: string(m[1]), Query: strings.TrimPrefix(string(m[2]), "?"), Line: i + 1})
		}
	}
	return out
}

// servedFrom reports whether a file of the repository answers an address: one whose path ends
// with it, as public/wp-content/uploads/logo.jpg answers /wp-content/uploads/logo.jpg. Pure.
func servedFrom(files []File, address string) bool {
	for _, f := range files {
		if "/"+f.Path == address || strings.HasSuffix("/"+f.Path, address) {
			return true
		}
	}
	return false
}

// w177 reports a page or code linking to an old WordPress or PHP address of the app's own site
// that no file of the repository answers and no redirect sends on: the edge answers it 404
// before the app sees it. One finding per file, at its first such link.
func w177(r Repo, _ Context) outcome {
	set, _ := loadRedirects(r)
	matcher := redirects.Compile(set.Rules)
	var out outcome
	for _, f := range r.Files {
		if f.Content == nil || !oldLinkFiles[strings.ToLower(path.Ext(f.Path))] || stack.IsVendored(f.Path) || stack.IsTest(f.Path) {
			continue
		}
		var missing []oldLink
		for _, l := range oldLinks(f.Content) {
			if servedFrom(r.Files, l.Path) {
				continue
			}
			if _, ok := matcher.Match(l.Path, l.Query, "https://app.example"); ok {
				continue
			}
			missing = append(missing, l)
		}
		if len(missing) == 0 {
			continue
		}
		msg := fmt.Sprintf("%s links to %s, which no file in the repository holds and no redirect sends on, so the edge answers it 404.", f.Path, missing[0].Path)
		if len(missing) > 1 {
			msg = fmt.Sprintf("%s links to %d old WordPress or PHP addresses no file in the repository holds and no redirect sends on, %s first, so the edge answers them 404.", f.Path, len(missing), missing[0].Path)
		}
		out = out.add("W177", f.Path, missing[0].Line, msg)
	}
	return out
}
