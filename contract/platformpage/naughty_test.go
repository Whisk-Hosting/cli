package platformpage

import (
	"html"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// Every field a platform page shows can carry text from a request (an app name, a path, a
// request id). Rendered with naughty text, the page has exactly the markup it has with plain
// text, so no input opens a tag (an empty field leaves its element out), and the heading reads
// as the escaped text.
func TestNaughtyRender(t *testing.T) {
	page := func(s string) Page {
		return Page{Heading: s, App: s, Lines: []string{s, "then " + s}, Code: s, RequestID: s, Waiting: true, NoScript: s}
	}
	baseline := strings.Count(Render(page("plain")), "<")
	for _, s := range naughty.Strings() {
		out := Render(page(s))
		if s != "" && strings.Count(out, "<") != baseline {
			t.Errorf("%q changed the markup: %d tags, want %d", s, strings.Count(out, "<"), baseline)
		}
		if strings.Contains(strings.ToLower(out), "<script") {
			t.Errorf("%q put a script tag on a page that has none", s)
		}
		if s != "" && !strings.Contains(out, "<h1>"+html.EscapeString(s)+"</h1>") {
			t.Errorf("%q is not the escaped heading", s)
		}
		if strings.Count(out, "<!doctype html>") != 1 || !strings.HasSuffix(out, "</body></html>\n") {
			t.Errorf("%q broke the document shape", s)
		}
	}
}
