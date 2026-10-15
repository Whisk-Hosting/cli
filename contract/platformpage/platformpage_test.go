package platformpage

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	cases := []struct {
		name string
		page Page
		want []string
		not  []string
	}{
		{
			name: "an app's page names the app and escapes everything",
			page: Page{Heading: "Sign in to continue", App: "Tom & Jerry's <CRM>", Lines: []string{"Line one.", "", "Line two."}, Code: "AUTH_REQUIRED", RequestID: "req_1"},
			want: []string{
				"<title>Sign in to continue · Tom &amp; Jerry&#39;s &lt;CRM&gt;</title>",
				`<p class="kicker">Tom &amp; Jerry&#39;s &lt;CRM&gt;</p>`,
				"<h1>Sign in to continue</h1>", "<p>Line one.</p><p>Line two.</p>",
				"<code>AUTH_REQUIRED</code> · request <code>req_1</code>",
				"is hosted on Whisk.", `class="dot"`, "#e3a92b", "#faf8f3",
			},
			not: []string{"<p></p>", "<script>", "spin\"", "<CRM>"},
		},
		{
			name: "a platform page has no app line and no small print when there is nothing to quote",
			page: Page{Heading: "Nothing here", Lines: []string{"Nothing is served at this hostname."}},
			want: []string{"<title>Nothing here</title>", "Whisk hosts business apps built with AI."},
			not:  []string{`class="kicker"`, `class="small"`},
		},
		{
			name: "the waking page spins and polls",
			page: Page{Heading: "Starting CRM…", App: "CRM", Waiting: true, Script: "poll()", NoScript: "Refresh in a few seconds."},
			want: []string{`<div class="spin"`, "<script>poll()</script>", "<noscript><p>Refresh in a few seconds.</p></noscript>", "prefers-reduced-motion"},
		},
		{
			name: "an error page reloads itself once after a while and offers a Retry button",
			page: Page{Heading: "Temporarily unavailable", Lines: []string{"Try again."}, Retry: &Retry{After: 5}},
			want: []string{`<p class="retry" id="whisk-retry"><a href="">Retry</a></p>`, "whiskHeal(5);</script>", "sessionStorage", "__whisk_reload=1"},
			not:  []string{" hidden>", "whiskHeal(Math"},
		},
		{
			name: "a page whose server fills in the wait clamps it",
			page: Page{Heading: "Too many requests", Retry: &Retry{AfterJS: "'{http.response.header.Retry-After}'", Sentence: "Wait & see."}},
			want: []string{"whiskHeal(Math.min(Math.max(parseInt('{http.response.header.Retry-After}',10)||5,1),120));", "Wait &amp; see.<br><a href=\"\">Retry</a>"},
		},
		{
			name: "the waking page keeps its Retry line hidden until its poll gives up",
			page: Page{Heading: "Starting CRM…", Waiting: true, Script: "poll()", Retry: &Retry{Hidden: true, Sentence: "CRM did not start."}},
			want: []string{`id="whisk-retry" hidden>CRM did not start.`, "function whiskHeal(after)", "poll()</script>"},
			not:  []string{"whiskHeal(0);", "whiskHeal(5)"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Render(c.page)
			if !strings.HasPrefix(got, "<!doctype html>") {
				t.Fatalf("not a document: %.40q", got)
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q", n)
				}
			}
		})
	}
}

// The offer line leads with its link, escapes every part, and is left out unless its address
// is https.
func TestRenderOffer(t *testing.T) {
	o := &Offer{Link: "Upgrade to Starter", URL: "https://whisk.run/o/acme/billing?a=1&b=2", Rest: "to keep your apps awake."}
	out := Render(Page{Heading: "Starting CRM…", Offer: o})
	want := `<p class="offer"><a href="https://whisk.run/o/acme/billing?a=1&amp;b=2">Upgrade to Starter</a> to keep your apps awake.</p>`
	if !strings.Contains(out, want) {
		t.Errorf("offer line missing: %s", out)
	}
	for _, bad := range []string{"javascript:alert(1)", "http://whisk.run/billing", ""} {
		if out := Render(Page{Heading: "h", Offer: &Offer{Link: "x", URL: bad}}); strings.Contains(out, `class="offer"`) {
			t.Errorf("offer with %q was drawn", bad)
		}
	}
	if out := Render(Page{Heading: "h"}); strings.Contains(out, `class="offer"`) {
		t.Error("a page without an offer drew one")
	}
}
