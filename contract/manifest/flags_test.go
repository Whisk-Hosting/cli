package manifest

import (
	"strings"
	"testing"
)

func TestFlags(t *testing.T) {
	base := "whisk: 1\nname: flag-app\n"
	cases := []struct {
		name, yaml string
		problem    string // a fragment of the problem, or "" for none
	}{
		{"none", "", ""},
		{"a release flag with its day unquoted", "flags:\n  new-checkout:\n    remove_by: 2027-01-31\n", ""},
		{"a release flag with its day quoted", "flags:\n  new-checkout:\n    kind: release\n    remove_by: \"2027-01-31\"\n", ""},
		{"a kill switch", "flags:\n  payments-off:\n    kind: kill-switch\n", ""},
		{"a permission flag with nothing else", "flags:\n  beta-reports: {kind: permission}\n", ""},
		{"a kill switch with a remove-by day", "flags:\n  payments-off:\n    kind: kill-switch\n    remove_by: 2027-01-31\n", "only a release flag has a remove_by"},
		{"a day not on the calendar", "flags:\n  new-checkout:\n    remove_by: \"2027-02-30\"\n", "not a day of the calendar"},
		{"a day in another shape", "flags:\n  new-checkout:\n    remove_by: \"31/01/2027\"\n", "remove_by"},
		{"an unknown kind", "flags:\n  new-checkout: {kind: forever}\n", "kind"},
		{"a name in capitals", "flags:\n  NewCheckout: {}\n", "NewCheckout"},
		{"an unknown key", "flags:\n  new-checkout: {owner: me}\n", "owner"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Parse([]byte(base + c.yaml))
			if c.problem == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				for name, f := range m.Flags {
					if f.Kind == "" {
						t.Fatalf("flag %s has no kind after defaults", name)
					}
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.problem) {
				t.Fatalf("Parse error = %v, want one naming %q", err, c.problem)
			}
		})
	}
}

func TestFlagDefaults(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: flag-app\nflags:\n  new-checkout:\n    remove_by: 2027-01-31\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Flags["new-checkout"]; got.Kind != FlagRelease || got.RemoveBy != "2027-01-31" {
		t.Fatalf("flag = %+v", got)
	}
	m, err = Parse([]byte("whisk: 1\nname: flag-app\n"))
	if err != nil || m.Flags == nil {
		t.Fatalf("no flags gives %#v, %v; want an empty map", m.Flags, err)
	}
}
