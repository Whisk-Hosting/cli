package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixturesValid(t *testing.T) {
	files, _ := filepath.Glob("../fixtures/manifests/valid/*.yaml")
	if len(files) == 0 {
		t.Fatal("no valid manifest fixtures")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(src); err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
		}
	}
}

// Each invalid fixture starts with "# expect: CODE substring". CODE is the Problems code and
// the substring must appear in the error, so a fixture cannot fail for the wrong reason.
func TestFixturesInvalid(t *testing.T) {
	files, _ := filepath.Glob("../fixtures/manifests/invalid/*.yaml")
	if len(files) == 0 {
		t.Fatal("no invalid manifest fixtures")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		first, _, _ := strings.Cut(string(src), "\n")
		code, want, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(first, "# expect:")), " ")
		if !strings.HasPrefix(first, "# expect:") || code == "" || want == "" {
			t.Fatalf("%s: first line must be '# expect: CODE substring'", filepath.Base(f))
		}
		_, err = Parse(src)
		if err == nil {
			t.Errorf("%s: expected %s containing %q, got no error", filepath.Base(f), code, want)
			continue
		}
		ps, ok := err.(Problems)
		if !ok {
			t.Errorf("%s: error is not Problems: %v", filepath.Base(f), err)
			continue
		}
		if ps.Code() != code {
			t.Errorf("%s: code %s, want %s (%v)", filepath.Base(f), ps.Code(), code, err)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %q does not contain %q", filepath.Base(f), err.Error(), want)
		}
	}
}

func TestTemplatesPass(t *testing.T) {
	// The three starters and the three canaries.
	files, _ := filepath.Glob("../../templates/*/whisk.yaml")
	canaries, _ := filepath.Glob("../../templates/canary/*/whisk.yaml")
	files = append(files, canaries...)
	if len(files) != 6 {
		t.Skipf("expected 6 template manifests, found %d", len(files))
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Parse(src)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		for _, fn := range m.Functions {
			if _, err := os.Stat(filepath.Join(filepath.Dir(f), fn.Graph)); err != nil {
				t.Errorf("%s: function %s graph %s: %v", f, fn.Name, fn.Graph, err)
			}
		}
	}
}

func TestDefaults(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: job-tracker\nfunctions:\n  - name: nightly\n    cron: \"0 6 * * *\"\n    graph: workflows/nightly.graph.yaml\n  - name: zero\n    event: x.y\n    graph: workflows/zero.graph.yaml\n    retries: 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Health.Path != "/health" || m.Health.Timeout != 90 || m.Database != "app" || m.Queue.Endpoint != "/.whisk/inngest" {
		t.Fatalf("defaults not applied: %+v", m)
	}
	if m.Functions[0].Retries != 3 || m.Functions[1].Retries != 0 {
		t.Fatalf("retries default wrong: %+v", m.Functions)
	}
	if m.CustomerIdentity != "none" || m.Previews.Database != "empty" || m.Previews.TTLDays != 3 {
		t.Fatalf("defaults not applied: %+v", m)
	}
	if m.Routes.Public == nil || m.Secrets == nil || m.Env == nil || m.Webhooks == nil {
		t.Fatal("empty collections must be non-nil so JSON renders [] and {}")
	}
}

func TestClassification(t *testing.T) {
	m, err := Parse([]byte(`
whisk: 1
name: app
routes:
  public: ["/", "/public/**", "/health", "/contact"]
  challenge: ["/contact"]
  csrf_off: ["/embed/*"]
secrets: [STRIPE_WEBHOOK_SECRET]
webhooks:
  - { name: stripe, preset: stripe, secret: STRIPE_WEBHOOK_SECRET, handler: /hooks/stripe }
`))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][3]bool{ // public, service, challenge
		"/":               {true, false, false},
		"/public/a/b":     {true, false, false},
		"/notes":          {false, false, false},
		"/hooks/stripe":   {false, true, false},
		"/hooks/stripe/":  {false, true, false},
		"/hooks/stripe/x": {false, false, false},
		"/.whisk/inngest": {false, true, false},
		"/contact":        {true, false, true},
	}
	for path, want := range cases {
		got := [3]bool{m.IsPublic(path), m.IsService(path), m.NeedsChallenge(path)}
		if got != want {
			t.Errorf("%s: got %v want %v", path, got, want)
		}
	}
	if !m.CSRFExempt("/embed/x") || m.CSRFExempt("/notes") {
		t.Fatal("csrf_off classification wrong")
	}
}

func TestUnknownKeySuggestion(t *testing.T) {
	_, err := Parse([]byte("whisk: 1\nname: app\nfunction: []\nroutes:\n  publc: []\n"))
	ps, ok := err.(Problems)
	if !ok {
		t.Fatalf("expected Problems, got %v", err)
	}
	if ps.Code() != "MANIFEST_UNKNOWN_KEY" {
		t.Fatalf("code = %s: %v", ps.Code(), err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "did you mean functions") || !strings.Contains(msg, "did you mean public") {
		t.Fatalf("suggestions missing: %s", msg)
	}
}

func TestCheckCron(t *testing.T) {
	good := []string{"0 6 * * *", "*/15 * * * *", "0 0 1 1 0", "30 2,14 * * 1-5", "0 9-17/2 * * *"}
	bad := []string{"* * * *", "60 * * * *", "0 24 * * *", "a b c d e", "0 6 * * 8", "5-3 * * * *", "*/0 * * * *"}
	for _, g := range good {
		if err := CheckCron(g); err != nil {
			t.Errorf("%q should be valid: %v", g, err)
		}
	}
	for _, b := range bad {
		if err := CheckCron(b); err == nil {
			t.Errorf("%q should be invalid", b)
		}
	}
}
