package doctor

import (
	"strings"
	"testing"
)

func TestW150(t *testing.T) {
	cases := []struct {
		name  string
		flags string
		want  []string // the flags named, in order
	}{
		{"no flags", "", nil},
		{"a release flag with a day to come", "flags:\n  new-checkout: {remove_by: 2027-01-31}\n", nil},
		{"a release flag with no day", "flags:\n  new-checkout: {}\n", []string{"new-checkout"}},
		{"a release flag past its day", "flags:\n  old-menu: {remove_by: 2026-10-01}\n", []string{"old-menu"}},
		{"a kill switch and a permission flag stay", "flags:\n  payments-off: {kind: kill-switch}\n  beta: {kind: permission}\n", nil},
		{"both, by name", "flags:\n  zeta: {}\n  alpha: {remove_by: 2026-01-01}\n", []string{"alpha", "zeta"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := repoOf(t, with(passing, map[string]string{"whisk.yaml": passing["whisk.yaml"] + c.flags}))
			got := w150On(r, "2026-11-02").findings
			if len(got) != len(c.want) {
				t.Fatalf("findings %+v, want %v", got, c.want)
			}
			for i, f := range got {
				if f.Rule != "W150" || !strings.Contains(f.Message, c.want[i]) || f.Line == 0 {
					t.Fatalf("finding %+v, want W150 naming %s with a line", f, c.want[i])
				}
			}
		})
	}
}

func repoOf(t *testing.T, files map[string]string) Repo {
	t.Helper()
	r, err := Load(t.Context(), writeAll(t, files))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestW150RunsInDoctor(t *testing.T) {
	rep := runDoctor(t, with(passing, map[string]string{"whisk.yaml": passing["whisk.yaml"] + "flags:\n  new-checkout: {}\n"}), false)
	if !has(rep, "W150") {
		t.Fatalf("W150 did not trigger; findings %+v", rep.Findings)
	}
}
