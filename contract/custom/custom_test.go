package custom

import (
	"strings"
	"testing"
)

func TestRuns(t *testing.T) {
	for p, want := range map[string]bool{
		"custom/migrations/001_notes.sql": true,
		"custom/views/deal_summary.sql":   true,
		"custom/migrations/README.md":     false,
		"custom/migrations/old/001.sql":   false, // a folder inside is left alone
		"custom/001.sql":                  false,
		"migrations/001.sql":              false,
		"src/custom/views/a.sql":          false,
		"custom/views":                    false,
	} {
		if got := Runs(p); got != want {
			t.Errorf("Runs(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestFileName(t *testing.T) {
	for name, want := range map[string]bool{
		"001_notes.sql":                   true,
		"deal-summary.v2.sql":             true,
		"A.sql":                           true,
		".sql":                            false,
		".hidden.sql":                     false,
		"-flag.sql":                       false,
		"notes.SQL":                       false,
		"notes.sql.bak":                   false,
		"sub/notes.sql":                   false,
		"../notes.sql":                    false,
		"no tes.sql":                      false,
		"notés.sql":                       false,
		strings.Repeat("a", 125) + ".sql": false,
		strings.Repeat("a", 124) + ".sql": true,
	} {
		if got := FileName(name); got != want {
			t.Errorf("FileName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	f := func(name, sql string) File { return File{Name: name, SQL: sql} }
	many := make([]File, 9)
	for i := range many {
		many[i] = f(string(rune('a'+i))+".sql", strings.Repeat("x", 250<<10))
	}
	for _, tc := range []struct {
		name              string
		migrations, views []File
		want              string
	}{
		{"none", nil, nil, ""},
		{"plain", []File{f("001.sql", "select 1")}, []File{f("v.sql", "select 1")}, ""},
		{"the same name in both folders", []File{f("a.sql", "")}, []File{f("a.sql", "")}, ""},
		{"a name Whisk does not run", []File{f("../a.sql", "")}, nil, `custom/migrations/"../a.sql" is not a name Whisk runs: use letters, digits, _, - and . and end it .sql`},
		{"twice", nil, []File{f("v.sql", ""), f("v.sql", "")}, "custom/views/v.sql is there twice"},
		{"too big", []File{f("a.sql", strings.Repeat("x", FileMax+1))}, nil, "custom/migrations/a.sql is over 256 KiB"},
		{"not UTF-8", nil, []File{f("v.sql", "select '\xff'")}, "custom/views/v.sql is not UTF-8 text"},
		{"a NUL", nil, []File{f("v.sql", "select 1\x00")}, "custom/views/v.sql is not UTF-8 text"},
		{"too much together", many, nil, "the files of custom/migrations and custom/views are over 2 MiB together"},
	} {
		if got := Check(tc.migrations, tc.views); got != tc.want {
			t.Errorf("%s: Check = %q, want %q", tc.name, got, tc.want)
		}
	}
}
