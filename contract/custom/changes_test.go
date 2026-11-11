package custom

import (
	"reflect"
	"strings"
	"testing"
)

func TestRecordName(t *testing.T) {
	for p, want := range map[string]string{
		"custom/changes/tell-the-warehouse.yaml":              "tell-the-warehouse",
		"custom/changes/2026-regions.yaml":                    "2026-regions",
		"custom/changes/Regions.yaml":                         "",
		"custom/changes/regions.yml":                          "",
		"custom/changes/-regions.yaml":                        "",
		"custom/changes/old/regions.yaml":                     "",
		"custom/changes/README.md":                            "",
		"custom/regions.yaml":                                 "",
		"custom/changes/.yaml":                                "",
		"custom/changes/" + strings.Repeat("a", 64) + ".yaml": "",
		"custom/changes/" + strings.Repeat("a", 63) + ".yaml": strings.Repeat("a", 63),
	} {
		got, ok := RecordName(p)
		if ok != (want != "") || ok && got != want {
			t.Errorf("RecordName(%q) = %q, %v; want %q", p, got, ok, want)
		}
	}
	if RecordPath("regions") != "custom/changes/regions.yaml" {
		t.Errorf("RecordPath = %q", RecordPath("regions"))
	}
}

func TestParseChange(t *testing.T) {
	whole := Change{Name: "c", Asked: "When a deal is won, tell the warehouse.",
		Touches: []string{"custom/server/warehouse.ts", "custom/web/stock/"}, Checks: []string{"custom/server/test/warehouse.test.ts"}}
	record := "asked: When a deal is won, tell the warehouse.\ntouches:\n  - custom/server/warehouse.ts\n  - custom/web/stock/\nchecks:\n  - custom/server/test/warehouse.test.ts\n"
	for _, tc := range []struct {
		name, src string
		want      *Change
		problems  []Problem
	}{
		{"a whole record", record, &whole, nil},
		{"asked over lines", strings.Replace(record, "asked: When a deal is won, tell the warehouse.", "asked: >\n  When a deal is won,\n  tell the warehouse.", 1), &whole, nil},
		{"empty", "", nil, []Problem{{0, "is empty: write what was asked in asked, the files in touches and the tests in checks"}}},
		{"not YAML", "asked: [\n", nil, []Problem{{1, "does not parse as YAML: did not find expected node content"}}},
		{"a list", "- a\n", nil, []Problem{{1, "is not a mapping of asked, touches and checks"}}},
		{"nothing asked", strings.Replace(record, "When a deal is won, tell the warehouse.", "", 1), nil,
			[]Problem{{1, "says nothing in asked: write what the business asked for, in its own plain words"}}},
		{"asked as a list", strings.Replace(record, "asked: When a deal is won, tell the warehouse.", "asked: [a]", 1), nil,
			[]Problem{{1, "asked is not text: write what the business asked for, in its own plain words"}}},
		{"asked at length", strings.Replace(record, "When a deal is won, tell the warehouse.", strings.Repeat("a", AskedMax+1), 1), nil,
			[]Problem{{1, "asked is over 1000 characters: say what was asked in a few sentences"}}},
		{"no asked", strings.SplitN(record, "\n", 2)[1], nil,
			[]Problem{{0, "says nothing in asked: write what the business asked for, in its own plain words"}}},
		{"no touches, no checks", "asked: a\n", nil, []Problem{
			{0, "names nothing in touches: name each file the change adds, changes or deletes"},
			{0, "names nothing in checks: name the tests that prove the change works"}}},
		{"empty lists", "asked: a\ntouches: []\nchecks:\n", nil, []Problem{
			{2, "names nothing in touches: name each file the change adds, changes or deletes"},
			{3, "names nothing in checks: name the tests that prove the change works"}}},
		{"touches as one path", strings.Replace(record, "touches:\n  - custom/server/warehouse.ts\n  - custom/web/stock/", "touches: custom/server/warehouse.ts", 1), nil,
			[]Problem{{2, "touches is not a list: write each path on a line of its own starting -"}}},
		{"a key it does not take", record + "by: Jane\n", nil,
			[]Problem{{7, "has by, which a record does not take: it takes asked, touches and checks"}}},
		{"a key twice", record + "asked: again\n", nil, []Problem{{7, "asked is there twice"}}},
		{"paths that are not paths", "asked: a\ntouches:\n  - /etc/passwd\n  - ../up.ts\n  - src/*.ts\n  - src\\a.ts\n  - ./a.ts\n  - custom/changes/b.yaml\n  - [x]\nchecks:\n  - custom/server/test/\n", nil, []Problem{
			{3, `touches has "/etc/passwd", which is not a path from the code's root, such as custom/server/warehouse.ts`},
			{4, `touches has "../up.ts", which is not a path from the code's root, such as custom/server/warehouse.ts`},
			{5, `touches has "src/*.ts", which is a pattern: name each file, or a folder ending / for everything in it`},
			{6, `touches has "src\\a.ts", which is not a path from the code's root: write it with / between folders`},
			{7, `touches has "./a.ts", which is not a path from the code's root, such as custom/server/warehouse.ts`},
			{8, `touches has "custom/changes/b.yaml", which is where the records are kept, which are never a change`},
			{9, "touches has an entry that is not a path"},
			{11, `checks has "custom/server/test/", which is a folder: name each test file`}}},
		{"too many paths", "asked: a\ntouches:\n" + strings.Repeat("  - a.ts\n", ListMax+1) + "checks: [a.test.ts]\n", nil,
			[]Problem{{2, "touches names over 200 paths: name a folder ending / for everything in it"}}},
		{"not UTF-8", "asked: \xff\n", nil, []Problem{{0, "is not UTF-8 text"}}},
		{"too long", "asked: " + strings.Repeat("a", 64<<10) + "\n", nil,
			[]Problem{{0, "is over 64 KiB, and a record says what was asked in a few sentences"}}},
	} {
		got, problems := ParseChange("c", []byte(tc.src))
		if !reflect.DeepEqual(problems, tc.problems) {
			t.Errorf("%s: problems = %+v, want %+v", tc.name, problems, tc.problems)
			continue
		}
		if tc.want != nil && !reflect.DeepEqual(got, *tc.want) {
			t.Errorf("%s: change = %+v, want %+v", tc.name, got, *tc.want)
		}
	}
}

func TestUncovered(t *testing.T) {
	warehouse := Change{Name: "tell-the-warehouse", Touches: []string{"custom/server/warehouse.ts", "custom/server/test/"}}
	regions := Change{Name: "regions", Touches: []string{"src/companies/CompanyList.tsx", "custom/server/test/regions.test.ts"}}
	for _, tc := range []struct {
		name               string
		changed            []string
		changes            []Change
		uncovered          []string
		unchanged, regions []string
	}{
		{"nothing changed", nil, []Change{warehouse}, nil, warehouse.Touches, nil},
		{"each file in a record, a folder covering what is in it",
			[]string{"custom/server/warehouse.ts", "custom/server/test/warehouse.test.ts", "src/companies/CompanyList.tsx", "custom/server/test/regions.test.ts"},
			[]Change{warehouse, regions}, nil, nil, nil},
		{"a file in no record, in order and once",
			[]string{"package.json", "custom/server/warehouse.ts", "custom/web/stock.tsx", "package.json"},
			[]Change{warehouse}, []string{"custom/web/stock.tsx", "package.json"}, []string{"custom/server/test/"}, nil},
		{"the records and .whisk are never a change",
			[]string{"custom/changes/regions.yaml", ".whisk/app.json", ".whisk/dev/compose.yaml"}, nil, nil, warehouse.Touches, nil},
		{"a folder's name alone is not inside it",
			[]string{"custom/server/test", "custom/server/tests/a.test.ts"}, []Change{warehouse},
			[]string{"custom/server/test", "custom/server/tests/a.test.ts"}, []string{"custom/server/warehouse.ts", "custom/server/test/"}, nil},
		{"a file the release has that the copy deleted, named by its record",
			[]string{"src/companies/CompanyList.tsx"}, []Change{regions}, nil, warehouse.Touches, []string{"custom/server/test/regions.test.ts"}},
	} {
		if got := Uncovered(tc.changed, tc.changes); !reflect.DeepEqual(got, tc.uncovered) {
			t.Errorf("%s: Uncovered = %q, want %q", tc.name, got, tc.uncovered)
		}
		if got := Unchanged(tc.changed, warehouse); !reflect.DeepEqual(got, tc.unchanged) {
			t.Errorf("%s: Unchanged(warehouse) = %q, want %q", tc.name, got, tc.unchanged)
		}
		if tc.regions != nil {
			if got := Unchanged(tc.changed, regions); !reflect.DeepEqual(got, tc.regions) {
				t.Errorf("%s: Unchanged(regions) = %q, want %q", tc.name, got, tc.regions)
			}
		}
	}
}

func TestRecords(t *testing.T) {
	good := []byte("asked: a\ntouches: [a.ts]\nchecks: [a.test.ts]\n")
	changes, problems := Records(map[string][]byte{
		"custom/changes/b.yaml":       good,
		"custom/changes/a.yaml":       good,
		"custom/changes/broken.yaml":  []byte("asked: a\n"),
		"custom/changes/notes.md":     []byte("# notes"),
		"custom/changes/old/c.yaml":   good,
		"custom/changes/Regions.yaml": good,
	})
	if names := []string{changes[0].Name, changes[1].Name}; len(changes) != 2 || !reflect.DeepEqual(names, []string{"a", "b"}) {
		t.Errorf("changes = %+v", changes)
	}
	notRecord := []Problem{{0, "is not a record: custom/changes holds one file a change, named for it in lower-case letters, digits and dashes and ending .yaml"}}
	want := map[string][]Problem{
		"custom/changes/broken.yaml": {
			{0, "names nothing in touches: name each file the change adds, changes or deletes"},
			{0, "names nothing in checks: name the tests that prove the change works"}},
		"custom/changes/notes.md":     notRecord,
		"custom/changes/old/c.yaml":   notRecord,
		"custom/changes/Regions.yaml": notRecord,
	}
	if !reflect.DeepEqual(problems, want) {
		t.Errorf("problems = %+v, want %+v", problems, want)
	}
}

func TestWhiskSQL(t *testing.T) {
	for p, want := range map[string]bool{
		"migrations/0033_regions.sql":       true,
		"db/schema.SQL":                     true,
		"custom/migrations/001_regions.sql": false,
		"custom/views/deal_summary.sql":     false,
		"migrations/README.md":              false,
		"src/custom/migrations/a.sql":       true,
	} {
		if got := WhiskSQL(p); got != want {
			t.Errorf("WhiskSQL(%q) = %v, want %v", p, got, want)
		}
	}
}
