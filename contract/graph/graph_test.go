package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixturesValid(t *testing.T) {
	files, _ := filepath.Glob("../fixtures/graphs/valid/*.graph.yaml")
	if len(files) == 0 {
		t.Fatal("no valid graph fixtures")
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

// Each invalid fixture carries a first-line comment "# expect: <substring>" that the error
// must contain, so a fixture cannot fail for the wrong reason.
func TestFixturesInvalid(t *testing.T) {
	files, _ := filepath.Glob("../fixtures/graphs/invalid/*.graph.yaml")
	if len(files) == 0 {
		t.Fatal("no invalid graph fixtures")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		first, _, _ := strings.Cut(string(src), "\n")
		want := strings.TrimSpace(strings.TrimPrefix(first, "# expect:"))
		if !strings.HasPrefix(first, "# expect:") || want == "" {
			t.Fatalf("%s: first line must be '# expect: <substring>'", filepath.Base(f))
		}
		_, err = Parse(src)
		if err == nil {
			t.Errorf("%s: expected an error containing %q, got none", filepath.Base(f), want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %q does not contain %q", filepath.Base(f), err.Error(), want)
		}
	}
}

func TestStepIDsAndDrift(t *testing.T) {
	g, err := Parse([]byte(`
steps:
  - id: fetch-po
  - id: over-limit?
    branches: { yes: request-approval, no: post-po }
  - id: request-approval
    kind: approval
    next: approved?
  - id: approved?
    branches: { yes: post-po, no: end }
  - id: post-po
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.StepIDs(), ","); got != "fetch-po,request-approval,post-po" {
		t.Fatalf("StepIDs = %s", got)
	}
	unexpected, never := g.Drift([]string{"fetch-po", "request-approval/request", "request-approval", "notify-buyer"})
	if strings.Join(unexpected, ",") != "notify-buyer" || strings.Join(never, ",") != "post-po" {
		t.Fatalf("Drift = %v, %v", unexpected, never)
	}
}

func TestSuccessorsImplicitNext(t *testing.T) {
	g, err := Parse([]byte("steps:\n  - id: a\n  - id: b\n  - id: c\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s := g.Successors(0); len(s) != 1 || s[0] != "b" {
		t.Fatalf("Successors(0) = %v", s)
	}
	if s := g.Successors(2); len(s) != 0 {
		t.Fatalf("last step must have no successors, got %v", s)
	}
}
