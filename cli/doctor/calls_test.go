package doctor

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
)

// W120 and W121 read call consent from the platform's validate answer, and say why they were
// skipped when they could not know it.
func TestCallConsentRules(t *testing.T) {
	yaml := func(extra string) map[string]string {
		return with(passing, map[string]string{"whisk.yaml": strings.Replace(passing["whisk.yaml"], "database: app\n", "database: app\n"+extra, 1)})
	}
	bind := func(files map[string]string) Repo {
		r := memRepo(files)
		r.Binding = &config.Binding{Org: "acme", App: "fixture"}
		return r
	}
	consent := func(notAccepted, unlisted []string) *api.Validation {
		v := &api.Validation{}
		v.Calls.NotAccepted, v.Calls.Unlisted = notAccepted, unlisted
		return v
	}
	cases := []struct {
		name     string
		rule     func(Repo, Context) outcome
		repo     Repo
		v        *api.Validation
		messages []string
		lines    []int
		skipped  string
	}{
		{"W120 two callers on the bridge", w120, bind(passing), consent(nil, []string{"shop", "website"}),
			[]string{"shop and website call fixture only because the call was made before apps had to accept their callers; add accepted_callers: [shop, website] to keep it."}, []int{2}, ""},
		{"W120 one caller", w120, bind(passing), consent(nil, []string{"website"}),
			[]string{"website calls fixture only because the call was made before apps had to accept their callers; add accepted_callers: [website] to keep it."}, []int{2}, ""},
		{"W120 declared, even empty", w120, bind(yaml("accepted_callers: []\n")), consent(nil, []string{"website"}), nil, nil, ""},
		{"W120 nobody on the bridge", w120, bind(passing), consent(nil, []string{}), nil, nil, ""},
		{"W120 not asked", w120, bind(passing), nil, nil, nil, "W120: skipped, the platform was not asked, so the other apps are unknown"},
		{"W120 unbound", w120, memRepo(passing), nil, nil, nil, "W120: skipped, the directory is not bound to an app, so the other apps are unknown"},
		{"W121 one target refuses", w121, bind(yaml("calls: [orders, stock]\n")), consent([]string{"stock"}, nil),
			[]string{"stock does not accept calls from fixture, so the call is refused: the whisk.yaml of stock needs accepted_callers: [fixture]."}, []int{8}, ""},
		{"W121 all accept", w121, bind(yaml("calls: [orders]\n")), consent([]string{}, nil), nil, nil, ""},
		{"W121 unbound", w121, memRepo(passing), nil, nil, nil, "W121: skipped, the directory is not bound to an app, so the other apps are unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.rule(tc.repo, Context{Validation: tc.v})
			if len(o.findings) != len(tc.messages) {
				t.Fatalf("findings = %+v, want %d", o.findings, len(tc.messages))
			}
			for i, f := range o.findings {
				if f.Message != tc.messages[i] || f.Line != tc.lines[i] || f.File != "whisk.yaml" || f.Level != "warning" || f.Fix == "" {
					t.Errorf("finding %d = %+v, want %q at line %d", i, f, tc.messages[i], tc.lines[i])
				}
			}
			if got := strings.Join(o.skipped, "|"); got != tc.skipped {
				t.Errorf("skipped = %q, want %q", got, tc.skipped)
			}
		})
	}
}
