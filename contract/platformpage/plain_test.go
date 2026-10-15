package platformpage

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// visible is what a person reading the page sees: its title and its text, without the script,
// the styles and the markup.
func visible(page string) string {
	page = regexp.MustCompile(`(?s)<(script|style)>.*?</(script|style)>`).ReplaceAllString(page, " ")
	return regexp.MustCompile(`<[^>]*>`).ReplaceAllString(page, " ")
}

// A plain page names the app and nothing of Whisk's: no wordmark, no footer, none of Whisk's
// colours, and a reload flag that names nobody.
func TestRenderPlain(t *testing.T) {
	cases := []struct {
		name string
		page Page
		want []string
	}{
		{
			name: "an error page",
			page: Page{Heading: "This app is temporarily unavailable", App: "Acme Portal", Lines: []string{"Try again in a moment."}, Code: "PLATFORM_UNAVAILABLE", RequestID: "req_1", Retry: &Retry{After: 5}, Plain: true},
			want: []string{`<header><span class="name">Acme Portal</span></header>`, "<title>This app is temporarily unavailable · Acme Portal</title>", "whiskHeal(5);", PlainReloadFlag + "=1", "<code>PLATFORM_UNAVAILABLE</code>"},
		},
		{
			name: "the waking page",
			page: Page{Heading: "Starting Acme Portal…", App: "Acme Portal", Lines: []string{"Opens in a moment."}, Waiting: true, Script: "poll()", Retry: &Retry{Hidden: true}, Plain: true},
			want: []string{`<div class="spin"`, "poll()</script>", "<h1>Starting Acme Portal…</h1>"},
		},
		{
			name: "a page with no app name has no header",
			page: Page{Heading: "Checking your browser", Lines: []string{"It takes a moment."}, Plain: true},
			want: []string{"<body><main>"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Render(c.page)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q", w)
				}
			}
			if text := visible(got); strings.Contains(strings.ToLower(text), "whisk") {
				t.Errorf("a plain page shows Whisk: %s", text)
			}
			for _, branded := range []string{"#e3a92b", "#faf8f3", "#14213d", `class="dot"`, "<footer>", `class="kicker"`, "Georgia", ReloadFlag} {
				if strings.Contains(got, branded) {
					t.Errorf("a plain page carries %q", branded)
				}
			}
		})
	}
}

func TestPlainLines(t *testing.T) {
	cases := []struct {
		message string
		status  int
		want    []string
	}{
		{"The app could not be started because its node did not answer.", 503, []string{"The app could not be started because its node did not answer.", "Try again in a moment."}},
		{"This app is paused while Whisk reviews it.", 503, []string{"Try again in a moment."}},
		{"You are signed in but do not have access to Acme.", 403, []string{"You are signed in but do not have access to Acme.", "If you think you should have access, ask whoever gave you this address."}},
		{"This route requires a signed-in user.", 401, []string{"This route requires a signed-in user.", "Open the app again to sign in."}},
		{"Too many requests.", 429, []string{"Too many requests.", "Wait a moment, then try again."}},
		{"", 400, []string{"Go back and try again."}},
		{"Set always_on in WHISK.yaml.", 504, []string{"Try again in a moment."}},
	}
	for _, c := range cases {
		if got := PlainLines(c.message, c.status); !slices.Equal(got, c.want) {
			t.Errorf("PlainLines(%q, %d) = %q, want %q", c.message, c.status, got, c.want)
		}
	}
}
