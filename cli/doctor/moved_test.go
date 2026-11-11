package doctor

import (
	"reflect"
	"testing"

	"github.com/whisk-run/contract/manifest"
)

const phonesNote = "moved:\n  - from: companies.phone_number\n    to: companies.phone_jsonb\nvalues: every number kept, as a \"Work\" entry\nfix: read phone_jsonb where you read phone_number\n"

// In a product's source, each name a migration removes or renames has a note in moved/.
func TestMovedRule(t *testing.T) {
	source := manifest.Manifest{Managed: &manifest.Managed{Product: "crm", Name: "CRM"}}
	migration := File{Path: "migrations/0021_data_model.sql", Content: []byte("drop view companies_summary;\nalter table companies rename column revenue to revenue_text;\n" +
		"alter table companies add column revenue bigint;\nalter table companies drop column revenue_text, drop column phone_number;\ncreate view companies_summary as select 1;\n")}
	note := func(p, body string) File { return File{Path: p, Content: []byte(body)} }
	type found struct {
		file    string
		line    int
		message string
	}
	for _, tc := range []struct {
		name string
		repo Repo
		want []found
	}{
		{"an app that is no product's source", Repo{Files: []File{migration}}, nil},
		{"a fork's checkout", Repo{Manifest: source, Fork: &Fork{Base: "abc"}, Files: []File{migration}}, nil},
		{"a source with a note for each name", Repo{Manifest: source, Files: []File{migration, note("moved/company-phones-as-list.yaml", phonesNote),
			note("moved/company-revenue-as-a-number.yaml", "moved:\n  - from: companies.revenue\n    to: companies.revenue\nvalues: read as a whole number\nfix: read revenue as a number\n")}}, nil},
		{"a name no note names", Repo{Manifest: source, Files: []File{migration, note("moved/company-phones-as-list.yaml", phonesNote)}},
			[]found{{"migrations/0021_data_model.sql", 2, "migrations/0021_data_model.sql renames companies.revenue to companies.revenue_text, and no note in moved/ says where it went."}}},
		{"a note that cannot be read, and a file that is no note", Repo{Manifest: source, Files: []File{note("moved/phones.yaml", "moved: []\nfix: x\n"), note("moved/README.md", "notes\n"),
			{Path: "migrations/0002.down.sql", Content: []byte("drop table deals;\n")}, {Path: "custom/migrations/0001_ours.sql", Content: []byte("drop table custom.ours;\n")}}},
			[]found{{"moved/README.md", 0, "moved/README.md is not a note: moved/ holds one file for each note, named in lower-case letters, digits and dashes and ending .yaml."},
				{"moved/phones.yaml", 1, "moved/phones.yaml names nothing in moved: name each name that moved, with from and, unless it went away, to."}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []found
			for _, f := range w174(tc.repo, Context{}).findings {
				got = append(got, found{f.File, f.Line, f.Message})
				if f.Rule != "W174" || f.Fix == "" || f.Level != "error" {
					t.Errorf("finding %+v", f)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("findings\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}
