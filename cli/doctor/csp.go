package doctor

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// An app's own Content-Security-Policy, in whisk.yaml's routes.headers or in its code, can block
// the analytics it loads: the browser refuses the script or its pings and the tool records no
// visits, with nothing in the app's logs to say so. W178 reads both and names the sources each
// tool needs.

// tracker is an analytics tool: how its use shows in code, and the sources its script and pings
// need, each with a host it is reached at, for testing the policy.
type tracker struct {
	Name  string
	Signs []string
	Needs []cspNeed
}

// cspNeed is one source a tool needs in a directive: Host is a host it reaches there (Scheme,
// https when empty), and Source is what to add to let it and its siblings through.
type cspNeed struct {
	Directive, Scheme, Host, Source string
}

// trackers are the analytics tools W178 knows, with the sources their own guides list.
var trackers = []tracker{
	{"Google Analytics", []string{"googletagmanager.com", "google-analytics.com", "@next/third-parties/google", "react-ga4"}, []cspNeed{
		{"script-src", "", "www.googletagmanager.com", "https://www.googletagmanager.com"},
		{"connect-src", "", "www.google-analytics.com", "https://*.google-analytics.com"},
		{"connect-src", "", "region1.google-analytics.com", "https://*.google-analytics.com"},
		{"connect-src", "", "region1.analytics.google.com", "https://*.analytics.google.com"},
		{"connect-src", "", "www.googletagmanager.com", "https://www.googletagmanager.com"},
		{"img-src", "", "www.google-analytics.com", "https://*.google-analytics.com"},
		{"img-src", "", "www.googletagmanager.com", "https://www.googletagmanager.com"},
	}},
	{"PostHog", []string{"posthog.com"}, []cspNeed{
		{"script-src", "", "us-assets.i.posthog.com", "https://*.posthog.com"},
		{"connect-src", "", "us.i.posthog.com", "https://*.posthog.com"},
	}},
	{"Plausible", []string{"plausible.io"}, []cspNeed{
		{"script-src", "", "plausible.io", "https://plausible.io"},
		{"connect-src", "", "plausible.io", "https://plausible.io"},
	}},
	{"Fathom", []string{"usefathom.com", "fathom-client"}, []cspNeed{
		{"script-src", "", "cdn.usefathom.com", "https://cdn.usefathom.com"},
		{"connect-src", "", "cdn.usefathom.com", "https://cdn.usefathom.com"},
	}},
	{"Microsoft Clarity", []string{"clarity.ms"}, []cspNeed{
		{"script-src", "", "www.clarity.ms", "https://*.clarity.ms"},
		{"connect-src", "", "a.clarity.ms", "https://*.clarity.ms"},
		{"img-src", "", "c.clarity.ms", "https://*.clarity.ms"},
	}},
	{"Hotjar", []string{"hotjar.com", "@hotjar/browser"}, []cspNeed{
		{"script-src", "", "static.hotjar.com", "https://*.hotjar.com"},
		{"connect-src", "", "in.hotjar.com", "https://*.hotjar.com"},
		{"connect-src", "", "content.hotjar.io", "https://*.hotjar.io"},
		{"connect-src", "wss", "ws.hotjar.com", "wss://*.hotjar.com"},
	}},
}

// cspFallback is the directive a browser reads when a policy has none for d (CSP Level 3).
var cspFallback = map[string][]string{
	"script-src":  {"script-src-elem", "script-src", "default-src"},
	"connect-src": {"connect-src", "default-src"},
	"img-src":     {"img-src", "default-src"},
}

// policy is a Content-Security-Policy as directive name to sources, names lower case.
type policy map[string][]string

// parsePolicy reads a policy's text. A directive named twice keeps its first, as a browser does.
// Pure.
func parsePolicy(text string) policy {
	p := policy{}
	for _, part := range strings.Split(text, ";") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		name := strings.ToLower(f[0])
		if _, seen := p[name]; !seen {
			p[name] = f[1:]
		}
	}
	return p
}

// allows reports whether the policy lets directive d reach scheme://host. Pure.
func (p policy) allows(d, scheme, host string) bool {
	for _, name := range cspFallback[d] {
		if sources, ok := p[name]; ok {
			for _, s := range sources {
				if sourceAllows(s, scheme, host) {
					return true
				}
			}
			return false
		}
	}
	return true
}

// sourceAllows is CSP Level 3 source matching for another site's host: *, a scheme, or a host
// with an optional scheme, a leading *. wildcard, a port and a path. https upgrades http and
// covers wss. Keywords such as 'self' never match another site. Pure.
func sourceAllows(src, scheme, host string) bool {
	s := strings.ToLower(strings.TrimSpace(src))
	schemeOK := func(want string) bool {
		switch want {
		case scheme:
			return true
		case "http":
			return scheme == "https"
		case "https":
			return scheme == "wss"
		case "ws":
			return scheme == "wss"
		}
		return false
	}
	switch {
	case s == "" || strings.HasPrefix(s, "'"):
		return false
	case s == "*":
		return scheme == "https" || scheme == "http" || scheme == "wss" || scheme == "ws"
	case strings.HasSuffix(s, ":") && !strings.Contains(s, "/"):
		return schemeOK(strings.TrimSuffix(s, ":"))
	}
	if i := strings.Index(s, "://"); i >= 0 {
		if !schemeOK(s[:i]) {
			return false
		}
		s = s[i+3:]
	} else if scheme != "https" && scheme != "wss" {
		return false
	}
	if i := strings.IndexAny(s, "/"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	if rest, ok := strings.CutPrefix(s, "*."); ok {
		return strings.HasSuffix(host, "."+rest)
	}
	return s == host
}

// cspText is a quoted string in code that holds a policy directive.
var cspText = regexp.MustCompile("[\"'`]([^\"'`]*(?:default-src|script-src|connect-src|img-src)[^\"'`]*)[\"'`]")

// policyIn is the policy a file sets, its text joined from every string in it that holds a
// directive, and the line of the first; "" when the file names no Content-Security-Policy or
// only one that reports. Pure.
func policyIn(content []byte) (string, int) {
	if !bytes.Contains(bytes.ToLower(content), []byte("content-security-policy")) {
		return "", 0
	}
	lower := bytes.ToLower(content)
	if bytes.Count(lower, []byte("content-security-policy-report-only")) == bytes.Count(lower, []byte("content-security-policy")) {
		return "", 0
	}
	var parts []string
	first := 0
	for i, line := range bytes.Split(content, []byte("\n")) {
		for _, m := range cspText.FindAllSubmatch(line, -1) {
			parts = append(parts, string(m[1]))
			if first == 0 {
				first = i + 1
			}
		}
	}
	return strings.Join(parts, ";"), first
}

// cspFiles are the files a policy or an analytics tool is written in.
var cspFiles = map[string]bool{
	".html": true, ".htm": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true,
	".cjs": true, ".astro": true, ".vue": true, ".svelte": true, ".py": true, ".go": true, ".json": true,
}

// blocked is what a policy lacks for the trackers in use: per tool, the directives and the
// sources to add, in the order trackers lists them. Pure.
func blocked(p policy, used []tracker) []string {
	var out []string
	for _, t := range used {
		add := map[string][]string{}
		var order []string
		for _, n := range t.Needs {
			scheme := n.Scheme
			if scheme == "" {
				scheme = "https"
			}
			if p.allows(n.Directive, scheme, n.Host) {
				continue
			}
			if _, ok := add[n.Directive]; !ok {
				order = append(order, n.Directive)
			}
			if !slices.Contains(add[n.Directive], n.Source) {
				add[n.Directive] = append(add[n.Directive], n.Source)
			}
		}
		if len(order) == 0 {
			continue
		}
		var parts []string
		for _, d := range order {
			parts = append(parts, strings.Join(add[d], " ")+" to "+d)
		}
		out = append(out, t.Name+" needs "+strings.Join(parts, ", "))
	}
	return out
}

// trackersIn is the analytics tools a repository's code loads. Pure.
func trackersIn(files []File) []tracker {
	var used []tracker
	for _, t := range trackers {
		for _, f := range files {
			if f.Content == nil || !cspFiles[strings.ToLower(path.Ext(f.Path))] || stack.IsVendored(f.Path) || stack.IsTest(f.Path) {
				continue
			}
			if containsAny(f.Content, t.Signs) {
				used = append(used, t)
				break
			}
		}
	}
	return used
}

func containsAny(b []byte, signs []string) bool {
	for _, s := range signs {
		if bytes.Contains(b, []byte(s)) {
			return true
		}
	}
	return false
}

// w178 reports a Content-Security-Policy, in whisk.yaml or in code, that blocks an analytics tool
// the app loads, naming the sources to add.
func w178(r Repo, _ Context) outcome {
	used := trackersIn(r.Files)
	if len(used) == 0 {
		return outcome{}
	}
	var out outcome
	report := func(file string, line int, text string) {
		if lacks := blocked(parsePolicy(text), used); len(lacks) > 0 {
			out = out.add("W178", file, line, fmt.Sprintf("The Content-Security-Policy in %s blocks the app's analytics, which then records no visits: %s.", file, strings.Join(lacks, "; ")))
		}
	}
	if r.HasManifest {
		keys := make([]string, 0, len(r.Manifest.Routes.Headers))
		for k := range r.Manifest.Routes.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if strings.EqualFold(k, "Content-Security-Policy") {
				report(manifestFile, yamlLine(r.ManifestNode, "/routes/headers"), r.Manifest.Routes.Headers[k])
			}
		}
	}
	for _, f := range r.Files {
		if f.Content == nil || !cspFiles[strings.ToLower(path.Ext(f.Path))] || stack.IsVendored(f.Path) || stack.IsTest(f.Path) {
			continue
		}
		if text, line := policyIn(f.Content); text != "" {
			report(f.Path, line, text)
		}
	}
	return out
}
