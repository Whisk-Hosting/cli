package doctor

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
)

// W160 and W161 read the business's categories and what an owner set from the platform's
// validate answer, and say why they were skipped when they could not know it.
func TestListingRules(t *testing.T) {
	yaml := func(extra string) map[string]string {
		return with(passing, map[string]string{"whisk.yaml": strings.Replace(passing["whisk.yaml"], "name: fixture\n", "name: fixture\n"+extra, 1)})
	}
	bind := func(files map[string]string) Repo {
		r := memRepo(files)
		r.Binding = &config.Binding{Org: "acme", App: "fixture"}
		return r
	}
	listed := func(categories []string, shown *api.ListingValues, owned ...string) *api.Validation {
		return &api.Validation{Listing: &api.ValidationListing{Categories: categories, Shown: shown, Owned: owned}}
	}
	full := "title: Job board\ndescription: Every open job.\ncategory: Jobs\n"
	cases := []struct {
		name     string
		rule     func(Repo, Context) outcome
		repo     Repo
		v        *api.Validation
		messages []string
		lines    []int
		skipped  string
	}{
		{"W160 nothing written, the business has categories", w160, bind(passing), listed([]string{"Sales", "Jobs"}, nil),
			[]string{`whisk.yaml has no title, description and category, which the business's home lists the app by. The business's categories are "Sales" and "Jobs"; use one of them unless none fits.`}, []int{2}, ""},
		{"W160 no category, and none yet", w160, bind(yaml("title: Job board\ndescription: Every open job.\n")), listed([]string{}, nil),
			[]string{"whisk.yaml has no category, which the business's home lists the app by. The business has no categories yet, so name one for what the app is for, such as Sales."}, []int{2}, ""},
		{"W160 only the description missing", w160, bind(yaml("title: Job board\ncategory: Jobs\n")), listed([]string{"Jobs"}, nil),
			[]string{"whisk.yaml has no description, which the business's home lists the app by."}, []int{2}, ""},
		{"W160 all written", w160, bind(yaml(full)), listed([]string{"Jobs"}, nil), nil, nil, ""},
		{"W160 not asked", w160, bind(passing), nil, nil, nil, "W160: skipped, the platform was not asked, so the business's categories are unknown"},
		{"W160 unbound", w160, memRepo(passing), nil, nil, nil, "W160: skipped, the directory is not bound to an app, so the business's categories are unknown"},
		{"W160 an older platform", w160, bind(passing), &api.Validation{}, nil, nil, "W160: skipped, the platform does not say the business's categories yet"},
		{"W161 an owner moved the app", w161, bind(yaml(full)), listed([]string{"Every day"}, &api.ListingValues{Title: "Job board", Description: "Every open job.", Category: "Every day"}, "category"),
			[]string{`An owner set the category on the business's home to "Every day", which stays; set category: "Every day" in whisk.yaml to match.`}, []int{5}, ""},
		{"W161 whisk.yaml already matches", w161, bind(yaml("title: Job board\ndescription: Every open job.\ncategory: Every day\n")), listed(nil, &api.ListingValues{Title: "Job board", Description: "Every open job.", Category: "Every day"}, "category"), nil, nil, ""},
		{"W161 no owner changed it", w161, bind(yaml(full)), listed(nil, nil), nil, nil, ""},
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
