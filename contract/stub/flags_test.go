package stub

import (
	"slices"
	"testing"

	"github.com/whisk-run/contract/manifest"
)

func TestOnFlags(t *testing.T) {
	m, err := manifest.Parse([]byte("whisk: 1\nname: flag-app\nflags:\n  zeta: {kind: permission}\n  alpha: {kind: permission}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := onFlags(m, []string{"zeta", "undeclared", "alpha", "zeta"}); !slices.Equal(got, []string{"alpha", "zeta"}) {
		t.Fatalf("onFlags = %v", got)
	}
	if got := onFlags(m, nil); got == nil || len(got) != 0 {
		t.Fatalf("onFlags(nil) = %#v, want empty", got)
	}
}

func TestEdgeSendsFlags(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	e := newEdge(s)
	_, out := get(t, e, "GET", "/whoami", map[string]string{"X-Whisk-Flags": "forged"}, "")
	if out["X-Whisk-Flags"] != "" {
		t.Fatalf("a forged X-Whisk-Flags reached the app: %v", out)
	}
	s.flags = []string{"alpha", "zeta"}
	_, out = get(t, e, "GET", "/whoami", nil, "")
	if out["X-Whisk-Flags"] != "alpha,zeta" {
		t.Fatalf("X-Whisk-Flags = %q", out["X-Whisk-Flags"])
	}
}
