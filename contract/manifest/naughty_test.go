package manifest

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/contract/naughty"
)

// Every naughty string, as a whole file and in every field a person or an agent writes, must
// come back as a manifest that keeps the schema's rules or as Problems with a manifest code
// (docs/HARNESS.md §8).

var (
	reNaughtySlug   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	reNaughtyPath   = regexp.MustCompile(`^/[^?#\s]*$`)
	reNaughtyEnv    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
	manifestCodes   = map[string]bool{"MANIFEST_INVALID": true, "MANIFEST_UNKNOWN_KEY": true, "CONVENTIONS_VERSION_UNSUPPORTED": true}
	naughtyFieldSet = []struct {
		name  string
		embed func(s string) map[string]any
	}{
		{"name", func(s string) map[string]any { return base(map[string]any{"name": s}) }},
		{"routes.public", func(s string) map[string]any {
			return base(map[string]any{"routes": map[string]any{"public": []any{s}}})
		}},
		{"routes.challenge", func(s string) map[string]any {
			return base(map[string]any{"routes": map[string]any{"public": []any{"/**"}, "challenge": []any{s}}})
		}},
		{"routes.csrf_off", func(s string) map[string]any {
			return base(map[string]any{"routes": map[string]any{"csrf_off": []any{s}}})
		}},
		{"routes.headers key", func(s string) map[string]any {
			return base(map[string]any{"routes": map[string]any{"headers": map[string]any{s: "x"}}})
		}},
		{"routes.headers value", func(s string) map[string]any {
			return base(map[string]any{"routes": map[string]any{"headers": map[string]any{"X-Note": s}}})
		}},
		{"health.path", func(s string) map[string]any { return base(map[string]any{"health": map[string]any{"path": s}}) }},
		{"database", func(s string) map[string]any { return base(map[string]any{"database": s}) }},
		{"migrate", func(s string) map[string]any { return base(map[string]any{"migrate": s}) }},
		{"build.dockerfile", func(s string) map[string]any {
			return base(map[string]any{"build": map[string]any{"dockerfile": s}})
		}},
		{"secrets", func(s string) map[string]any { return base(map[string]any{"secrets": []any{s}}) }},
		{"env key", func(s string) map[string]any { return base(map[string]any{"env": map[string]any{s: "v"}}) }},
		{"env value", func(s string) map[string]any { return base(map[string]any{"env": map[string]any{"LOG_LEVEL": s}}) }},
		{"queue.endpoint", func(s string) map[string]any {
			return base(map[string]any{"queue": map[string]any{"endpoint": s}})
		}},
		{"functions.name", func(s string) map[string]any {
			return base(map[string]any{"functions": []any{map[string]any{"name": s, "cron": "0 6 * * *", "graph": "w/a.graph.yaml"}}})
		}},
		{"functions.cron", func(s string) map[string]any {
			return base(map[string]any{"functions": []any{map[string]any{"name": "nightly", "cron": s, "graph": "w/a.graph.yaml"}}})
		}},
		{"functions.cron minute", func(s string) map[string]any {
			return base(map[string]any{"functions": []any{map[string]any{"name": "nightly", "cron": s + " * * * *", "graph": "w/a.graph.yaml"}}})
		}},
		{"functions.tz", func(s string) map[string]any {
			return base(map[string]any{"functions": []any{map[string]any{"name": "nightly", "cron": "0 6 * * *", "tz": s, "graph": "w/a.graph.yaml"}}})
		}},
		{"functions.event", func(s string) map[string]any {
			return base(map[string]any{"functions": []any{map[string]any{"name": "on-po", "event": s, "graph": "w/a.graph.yaml"}}})
		}},
		{"functions.graph", func(s string) map[string]any {
			return base(map[string]any{"functions": []any{map[string]any{"name": "on-po", "event": "po.created", "graph": s}}})
		}},
		{"webhooks.name", func(s string) map[string]any { return hook(map[string]any{"name": s}) }},
		{"webhooks.preset", func(s string) map[string]any { return hook(map[string]any{"preset": s}) }},
		{"webhooks.secret", func(s string) map[string]any { return hook(map[string]any{"secret": s}) }},
		{"webhooks.handler", func(s string) map[string]any { return hook(map[string]any{"handler": s}) }},
		{"webhooks.ip_allowlist", func(s string) map[string]any { return hook(map[string]any{"ip_allowlist": []any{s}}) }},
		{"webhooks.hmac.header", func(s string) map[string]any {
			return hook(map[string]any{"preset": "hmac", "hmac": map[string]any{"header": s, "algorithm": "sha256", "encoding": "hex", "payload": "{body}"}})
		}},
		{"webhooks.hmac.payload", func(s string) map[string]any {
			return hook(map[string]any{"preset": "hmac", "hmac": map[string]any{"header": "X-Sig", "algorithm": "sha256", "encoding": "hex", "payload": s}})
		}},
		{"webhooks.hmac.signature_pattern", func(s string) map[string]any {
			return hook(map[string]any{"preset": "hmac", "hmac": map[string]any{"header": "X-Sig", "algorithm": "sha256", "encoding": "hex", "payload": "{body}", "signature_pattern": s}})
		}},
		{"static.dir", func(s string) map[string]any {
			return base(map[string]any{"static": []any{map[string]any{"dir": s, "path": "/assets"}}})
		}},
		{"calls", func(s string) map[string]any { return base(map[string]any{"calls": []any{s}}) }},
		{"customer_identity", func(s string) map[string]any { return base(map[string]any{"customer_identity": s}) }},
		{"unknown key", func(s string) map[string]any { return base(map[string]any{s: true}) }},
	}
)

func base(extra map[string]any) map[string]any {
	m := map[string]any{"whisk": 1, "name": "job-tracker", "secrets": []any{"HOOK_SECRET"}}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func hook(fields map[string]any) map[string]any {
	w := map[string]any{"name": "stripe", "preset": "stripe", "secret": "HOOK_SECRET", "handler": "/hooks/stripe"}
	for k, v := range fields {
		w[k] = v
	}
	if w["preset"] == "hmac" {
		w["secret"] = "HOOK_SECRET"
	}
	return base(map[string]any{"webhooks": []any{w}})
}

// checkParsed asserts what Parse promises: Problems with manifest codes, or a manifest whose
// values keep the schema and the rules.
func checkParsed(t *testing.T, where string, s string, src []byte) {
	t.Helper()
	m, err := Parse(src)
	if err != nil {
		var ps Problems
		if !errors.As(err, &ps) || len(ps) == 0 {
			t.Errorf("%s %q: error is not Problems: %v", where, s, err)
			return
		}
		if !manifestCodes[ps.Code()] {
			t.Errorf("%s %q: code %q", where, s, ps.Code())
		}
		for _, p := range ps {
			if !manifestCodes[p.Code] || p.Message == "" {
				t.Errorf("%s %q: problem %+v", where, s, p)
			}
		}
		return
	}
	if !reNaughtySlug.MatchString(m.Name) || len(m.Name) < 3 || len(m.Name) > 40 {
		t.Errorf("%s %q: accepted name %q", where, s, m.Name)
	}
	paths := append(append(append([]string{m.Health.Path, m.Queue.Endpoint}, m.Routes.Public...), m.Routes.Challenge...), m.Routes.CSRFOff...)
	for _, w := range m.Webhooks {
		paths = append(paths, w.Handler)
	}
	for _, p := range paths {
		if !reNaughtyPath.MatchString(p) || len(p) > 200 {
			t.Errorf("%s %q: accepted path %q", where, s, p)
		}
	}
	names := append(append([]string{}, m.Secrets...), m.Build.Secrets...)
	for k := range m.Env {
		names = append(names, k)
	}
	for _, n := range names {
		if !reNaughtyEnv.MatchString(n) || reservedName(n) != "" {
			t.Errorf("%s %q: accepted variable name %q", where, s, n)
		}
	}
	for _, f := range m.Functions {
		if !reNaughtySlug.MatchString(f.Name) {
			t.Errorf("%s %q: accepted function name %q", where, s, f.Name)
		}
		if f.Cron != "" && CheckCron(f.Cron) != nil {
			t.Errorf("%s %q: accepted cron %q", where, s, f.Cron)
		}
		if f.Cron != "" {
			NextCron(f.Cron, time.UTC, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		}
	}
	for _, w := range m.Webhooks {
		if !reNaughtySlug.MatchString(w.Name) {
			t.Errorf("%s %q: accepted webhook name %q", where, s, w.Name)
		}
		if w.HMAC != nil && w.HMAC.Check() != nil {
			t.Errorf("%s %q: accepted hmac settings that do not check: %v", where, s, w.HMAC.Check())
		}
	}
	for _, c := range m.Calls {
		if !reNaughtySlug.MatchString(c) {
			t.Errorf("%s %q: accepted call %q", where, s, c)
		}
	}
	switch m.CustomerIdentity {
	case CustomerIdentityNone, CustomerIdentityApp, CustomerIdentityOrg:
	default:
		t.Errorf("%s %q: accepted customer_identity %q", where, s, m.CustomerIdentity)
	}
}

func TestNaughtyManifestFields(t *testing.T) {
	for _, field := range naughtyFieldSet {
		for _, s := range naughty.Strings() {
			src, err := yaml.Marshal(field.embed(s))
			if err != nil {
				t.Fatalf("%s %q: %v", field.name, s, err)
			}
			checkParsed(t, field.name, s, src)
		}
	}
}

// The same strings pasted unquoted into the text, where they can break the YAML itself.
func TestNaughtyManifestText(t *testing.T) {
	templates := []string{
		"%s",
		"whisk: 1\nname: %s\n",
		"whisk: %s\nname: job-tracker\n",
		"whisk: 1\nname: job-tracker\nroutes:\n  public: [%s]\n",
		"whisk: 1\nname: job-tracker\nenv:\n  LOG_LEVEL: %s\n",
		"whisk: 1\nname: job-tracker\nfunctions:\n  - name: nightly\n    cron: %s\n    graph: w/a.graph.yaml\n",
	}
	for _, tpl := range templates {
		for _, s := range naughty.Strings() {
			checkParsed(t, tpl, s, []byte(strings.Replace(tpl, "%s", s, 1)))
		}
	}
}

// A cron expression is valid exactly when NextCron can plan it; SpacedCron only rewrites valid
// ones and what it writes is still valid.
func TestNaughtyCron(t *testing.T) {
	after := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	var exprs []string
	for _, s := range naughty.Strings() {
		exprs = append(exprs, s, s+" * * * *", "* "+s+" * * *", "* * "+s+" * *", "* * * "+s+" *", "* * * * "+s)
	}
	// Steps past the field's range, up to the largest number Atoi accepts.
	for _, step := range []string{"64", "9223372036854775807", "9223372036854775806"} {
		exprs = append(exprs, "*/"+step+" * * * *", "5/"+step+" * * * *", "0 3/"+step+" * * *", "0 0 1/"+step+" * *", "0 0 * 2/"+step+" *", "0 0 * * 1/"+step)
	}
	for _, e := range exprs {
		valid := CheckCron(e) == nil
		if _, ok := NextCron(e, time.UTC, after); ok && !valid {
			t.Errorf("NextCron planned %q, which CheckCron refuses", e)
		}
		for _, min := range []int{2, 15, 60} {
			spaced, changed := SpacedCron(e, min)
			if !changed && spaced != e {
				t.Errorf("SpacedCron(%q, %d) changed the text without saying so: %q", e, min, spaced)
			}
			if changed && CheckCron(spaced) != nil {
				t.Errorf("SpacedCron(%q, %d) = %q, which is not valid", e, min, spaced)
			}
		}
	}
}

// A request path is classified the same however odd it is, and a service route is never
// public by accident.
func TestNaughtyClassification(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: job-tracker\nroutes:\n  public: [\"/\", \"/public/**\", \"/files/*.pdf\"]\n  challenge: [\"/public/contact\"]\n  csrf_off: [\"/embed/*\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range naughty.Strings() {
		for _, p := range []string{s, "/" + s, "/public/" + s, "/files/" + s} {
			public := m.IsPublic(p)
			m.IsService(p)
			if m.NeedsChallenge(p) && !public {
				t.Errorf("%q needs a challenge but is not public", p)
			}
			m.CSRFExempt(p)
		}
		if _, ok := m.FunctionByName(s); ok {
			t.Errorf("FunctionByName(%q) found a function in a manifest that declares none", s)
		}
		if _, ok := m.WebhookByName(s); ok {
			t.Errorf("WebhookByName(%q) found a webhook in a manifest that declares none", s)
		}
	}
}
