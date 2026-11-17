package doctor

import (
	"strings"
	"testing"
)

func TestSourceAllows(t *testing.T) {
	cases := []struct {
		src, scheme, host string
		want              bool
	}{
		{"*", "https", "www.googletagmanager.com", true},
		{"https:", "https", "www.googletagmanager.com", true},
		{"https:", "wss", "ws.hotjar.com", true},
		{"http:", "https", "x.com", true},
		{"'self'", "https", "www.google-analytics.com", false},
		{"'unsafe-inline'", "https", "www.google-analytics.com", false},
		{"https://www.googletagmanager.com", "https", "www.googletagmanager.com", true},
		{"www.googletagmanager.com", "https", "www.googletagmanager.com", true},
		{"https://www.googletagmanager.com/gtag/js", "https", "www.googletagmanager.com", true},
		{"https://*.google-analytics.com", "https", "region1.google-analytics.com", true},
		{"*.google-analytics.com", "https", "google-analytics.com", false},
		{"https://www.google-analytics.com", "https", "region1.google-analytics.com", false},
		{"https://*.hotjar.com", "wss", "ws.hotjar.com", true},
		{"https://a.com:443", "https", "a.com", true},
		{"data:", "https", "a.com", false},
	}
	for _, c := range cases {
		if got := sourceAllows(c.src, c.scheme, c.host); got != c.want {
			t.Errorf("sourceAllows(%q, %s://%s) = %v, want %v", c.src, c.scheme, c.host, got, c.want)
		}
	}
}

func TestBlocked(t *testing.T) {
	ga := trackers[0]
	cases := []struct {
		name, policy string
		want         []string // each must appear; none when empty
	}{
		{"no policy blocks nothing", "", nil},
		{"self only", "default-src 'self'", []string{"https://www.googletagmanager.com to script-src", "https://*.google-analytics.com https://*.analytics.google.com https://www.googletagmanager.com to connect-src"}},
		{"the script let in, the pings not", "default-src 'self'; script-src 'self' https://www.googletagmanager.com; img-src *", []string{"to connect-src"}},
		{"everything it needs", "default-src 'self'; script-src 'self' https://www.googletagmanager.com; connect-src 'self' https://*.google-analytics.com https://*.analytics.google.com https://www.googletagmanager.com; img-src 'self' https://*.google-analytics.com https://www.googletagmanager.com", nil},
		{"no connect-src falls back to a wide default", "default-src https:; script-src 'self' https://www.googletagmanager.com", nil},
	}
	for _, c := range cases {
		got := strings.Join(blocked(parsePolicy(c.policy), []tracker{ga}), "|")
		if len(c.want) == 0 && got != "" {
			t.Errorf("%s: blocked = %q, want nothing", c.name, got)
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: blocked = %q, want it to name %q", c.name, got, w)
			}
		}
	}
}

// W178: a policy that blocks the analytics the app loads names what to add.
func TestW178PolicyBlocksAnalytics(t *testing.T) {
	manifest := passing["whisk.yaml"]
	gtag := `<script async src="https://www.googletagmanager.com/gtag/js?id=G-XXXX"></script>`
	nextConfig := "const csp = [\n  \"default-src 'self'\",\n  \"script-src 'self' 'unsafe-inline'\",\n].join('; ');\nmodule.exports = { async headers() { return [{ source: '/(.*)', headers: [{ key: 'Content-Security-Policy', value: csp }] }] } };\n"
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"whisk.yaml's policy blocks Google Analytics", map[string]string{
			"public/index.html": gtag,
			"whisk.yaml":        replaceOnce(manifest, "routes:\n", "routes:\n  headers:\n    Content-Security-Policy: \"default-src 'self'\"\n"),
		}, true},
		{"next.config's policy blocks it", map[string]string{"public/index.html": gtag, "next.config.js": nextConfig}, true},
		{"a meta tag's policy blocks PostHog", map[string]string{
			"index.html": `<meta http-equiv="Content-Security-Policy" content="default-src 'self'"><script>posthog.init('phc_x', {api_host: 'https://us.i.posthog.com'})</script>`,
		}, true},
		{"a policy that lets it through", map[string]string{
			"public/index.html": gtag,
			"whisk.yaml":        replaceOnce(manifest, "routes:\n", "routes:\n  headers:\n    Content-Security-Policy: \"default-src 'self'; script-src 'self' https://www.googletagmanager.com; connect-src 'self' https://*.google-analytics.com https://*.analytics.google.com https://www.googletagmanager.com; img-src 'self' https://*.google-analytics.com https://www.googletagmanager.com\"\n"),
		}, false},
		{"a report-only policy", map[string]string{
			"index.html": `<meta http-equiv="Content-Security-Policy-Report-Only" content="default-src 'self'">` + gtag,
		}, false},
		{"no analytics", map[string]string{"next.config.js": nextConfig}, false},
		{"analytics and no policy", map[string]string{"public/index.html": gtag}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W178"); got != c.want {
				t.Errorf("W178 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}
