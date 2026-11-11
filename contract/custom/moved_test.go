package custom

import (
	"reflect"
	"strings"
	"testing"
)

// A note says what moved, where it went, what became of the values and what code that used it
// does instead; anything else is a problem at its line.
func TestParseMoved(t *testing.T) {
	good := "moved:\n  - from: companies.phone_number\n    to: companies.phone_jsonb\nvalues: every number kept, as a \"Work\" entry\nfix: read phone_jsonb where you read phone_number\n"
	m, ps := ParseMoved("company-phones-as-list", []byte(good))
	want := Moved{ID: "company-phones-as-list", Moves: []Move{{"companies.phone_number", "companies.phone_jsonb"}},
		Values: `every number kept, as a "Work" entry`, Fix: "read phone_jsonb where you read phone_number"}
	if len(ps) != 0 || !reflect.DeepEqual(m, want) {
		t.Errorf("good note: %+v %v", m, ps)
	}
	gone := "moved:\n  - from: handle_contact_note_created\nfix: an activity's trigger keeps last_seen\n"
	if m, ps := ParseMoved("gone", []byte(gone)); len(ps) != 0 || m.Moves[0].To != "" || m.Values != "" {
		t.Errorf("a name that went away: %+v %v", m, ps)
	}
	for _, c := range []struct {
		name, src string
		want      []Problem
	}{
		{"empty", "", []Problem{{0, "is empty: name each name that moved in moved, with where it went, and say what code that used it does instead in fix"}}},
		{"not yaml", "moved: [\n", []Problem{{1, "does not parse as YAML: did not find expected node content"}}},
		{"a list", "- a\n", []Problem{{1, "is not a mapping of moved, values and fix"}}},
		{"no fix", "moved:\n  - from: a\n", []Problem{{0, "says nothing in fix: say what code that used what moved does instead"}}},
		{"no moved", "fix: x\n", []Problem{{0, "names nothing in moved: name each name that moved, with from and, unless it went away, to"}}},
		{"empty moved", "moved: []\nfix: x\n", []Problem{{1, "names nothing in moved: name each name that moved, with from and, unless it went away, to"}}},
		{"no from", "moved:\n  - to: b\nfix: x\n", []Problem{{2, "moved has an entry with no from: name what moved"}}},
		{"a word", "moved:\n  - a\nfix: x\n", []Problem{{2, "moved has an entry that is not a mapping of from and to"}}},
		{"two lines", "moved:\n  - from: \"a\\nb\"\nfix: x\n", []Problem{{2, "moved has a from that is more than one line"}}},
		{"other key", "moved:\n  - from: a\n    as: b\nfix: x\nwhy: y\n", []Problem{{3, "moved has as, which an entry does not take: it takes from and to"}, {5, "has why, which a note does not take: it takes moved, values and fix"}}},
		{"twice", "moved:\n  - from: a\nfix: x\nfix: y\n", []Problem{{4, "fix is there twice"}}},
		{"empty fix", "moved:\n  - from: a\nfix:\n", []Problem{{3, "says nothing in fix"}}},
		{"long", "moved:\n  - from: a\nfix: " + strings.Repeat("x", AskedMax+1) + "\n", []Problem{{3, "fix is over 1000 characters: say it in a few sentences"}}},
		{"binary", "moved:\x00", []Problem{{0, "is not UTF-8 text"}}},
	} {
		if _, got := ParseMoved("n", []byte(c.src)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestMovedID(t *testing.T) {
	for p, want := range map[string]string{"moved/company-phones.yaml": "company-phones", "moved/a/b.yaml": "", "moved/A.yaml": "", "custom/changes/a.yaml": "", "moved/a.yml": ""} {
		if id, ok := MovedID(p); ok != (want != "") || ok && id != want {
			t.Errorf("MovedID(%s) = %q, %v", p, id, ok)
		}
	}
	if MovedPath("a") != "moved/a.yaml" || !InMoved("moved/x") || InMoved("custom/moved/x") {
		t.Error("MovedPath or InMoved")
	}
}

// What a migration removes or renames counts as it stood before the file ran; what the file made
// itself, and a view or function made again under its name, does not.
func TestSQLMoves(t *testing.T) {
	for _, c := range []struct {
		name, sql string
		want      []SQLMove
	}{
		{"tables gone", "drop table contact_notes;\nDROP TABLE IF EXISTS public.deal_notes, \"Old\" CASCADE;",
			[]SQLMove{{"contact_notes", "", 1}, {"deal_notes", "", 2}, {"Old", "", 2}}},
		{"a column renamed, made again and dropped",
			"alter table companies rename column revenue to revenue_text;\nalter table companies add column revenue bigint;\nalter table companies drop column revenue_text, drop column phone_number;",
			[]SQLMove{{"companies.revenue", "companies.revenue_text", 1}, {"companies.revenue", "", 3}, {"companies.phone_number", "", 3}}},
		{"a view made again", "drop view companies_summary;\ncreate view companies_summary as select 1;", nil},
		{"a view gone", "drop materialized view if exists totals;", []SQLMove{{"totals", "", 1}}},
		{"a function made again under its name", "alter function merge(bigint, bigint) rename to merge_keeping;\ncreate function merge(a bigint) returns bigint language sql as $$ select a $$;", nil},
		{"a function renamed", "ALTER FUNCTION merge(bigint) RENAME TO combine;", []SQLMove{{"merge", "combine", 1}}},
		{"a function gone", "drop function handle_note_created();", []SQLMove{{"handle_note_created", "", 1}}},
		{"a table made and dropped", "create table scratch (id int);\ninsert into scratch values (1);\ndrop table scratch;", nil},
		{"a column of a table made here", "create table t (a int);\nalter table t drop column a;", nil},
		{"a column added and dropped", "alter table t add column if not exists a int;\nalter table t drop a;", nil},
		{"a table renamed, then its column", "alter table notes rename to activities;\nalter table activities drop column kind;",
			[]SQLMove{{"notes", "activities", 1}, {"notes.kind", "", 2}}},
		{"a table made again", "drop table t;\ncreate table t (id int);", []SQLMove{{"t", "", 1}}},
		{"a type's value", "alter type stage rename value 'won' to 'closed won';", []SQLMove{{"stage.won", "stage.closed won", 1}}},
		{"a view's column", "alter view v rename column a to b;", []SQLMove{{"v.a", "v.b", 1}}},
		{"constraints and defaults", "alter table t drop constraint t_pk, alter column a drop default, rename constraint c to d;\ndrop index t_idx;\ndrop trigger x on t;", nil},
		{"comments and strings", "-- drop table a;\n/* drop table b; /* nested */ drop table c; */\nselect 'drop table d;';\ndo $body$ begin execute 'drop table e'; end $body$;\nselect E'it\\'s; drop table f';", nil},
		{"goose's down", "-- +goose Up\nalter table t add column a int;\nalter table t rename column b to c;\n-- +goose Down\nalter table t rename column c to b;\ndrop table t;",
			[]SQLMove{{"t.b", "t.c", 3}}},
		{"dbmate's down", "-- migrate:up\ndrop table a;\n-- migrate:down\ncreate table a (id int);\ndrop table b;", []SQLMove{{"a", "", 2}}},
	} {
		if got := SQLMoves(c.sql); !(len(got) == 0 && len(c.want) == 0) && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
	if got := SQLMoves("alter table \"Deals\" drop column \"Amount\";"); len(got) != 1 || got[0].Name != "Deals.Amount" || len(Unexplained(got, []Moved{{Moves: []Move{{From: `"Deals"."Amount"`}}}})) != 0 {
		t.Errorf("quoted: %+v", got)
	}
}

// A name a note says moved explains it however it is written; each other is unexplained.
func TestUnexplained(t *testing.T) {
	moves := []SQLMove{{"companies.phone_number", "", 3}, {"contact_notes", "", 9}, {"merge", "combine", 12}}
	notes := []Moved{{ID: "phones", Moves: []Move{{From: "public.Companies.phone_number", To: "companies.phone_jsonb"}}},
		{ID: "merge", Moves: []Move{{From: "merge(bigint, bigint)", To: "combine"}}}}
	if got := Unexplained(moves, notes); !reflect.DeepEqual(got, []SQLMove{{"contact_notes", "", 9}}) {
		t.Errorf("Unexplained = %+v", got)
	}
	if len(moves) != 3 {
		t.Error("Unexplained changed what it was given")
	}
	if !SameName(`"Odd"`, `"Odd"`) || SameName(`"Odd"`, "odd") || !SameName("public.t", "T") {
		t.Error("SameName")
	}
}

// A column is found wherever code names it, as a whole word in any case; a file by its path, with
// or without its extension.
func TestNoteUses(t *testing.T) {
	n := Moved{ID: "phones", Moves: []Move{{From: "companies.phone_number", To: "companies.phone_jsonb"}, {From: "src/deals/won.ts"}}}
	files := map[string][]Line{
		"custom/server/warehouse.ts": FileLines([]byte("const c = await db.one(sql`select phone_number from companies`);\nconst other = phone_numbers;\nconst Phone_Number = 1;\n")),
		"custom/web/deal.tsx":        FileLines([]byte("import { won } from \"../../src/deals/won\";\nexport const nothing = 1;\n")),
	}
	want := []Use{
		{"custom/server/warehouse.ts", 1, "const c = await db.one(sql`select phone_number from companies`);"},
		{"custom/server/warehouse.ts", 3, "const Phone_Number = 1;"},
		{"custom/web/deal.tsx", 1, `import { won } from "../../src/deals/won";`},
	}
	if got := NoteUses(n, files); !reflect.DeepEqual(got, want) {
		t.Errorf("NoteUses = %+v", got)
	}
	long := map[string][]Line{"a.ts": {{7, strings.Repeat("x", 300) + " phone_number"}}}
	if got := NoteUses(n, long); len(got) != 1 || got[0].Line != 7 || len([]rune(got[0].Text)) != useTextMax || !strings.HasSuffix(got[0].Text, "…") {
		t.Errorf("a long line: %+v", got)
	}
	if got := FileLines([]byte("a\nb\n")); !reflect.DeepEqual(got, []Line{{1, "a"}, {2, "b"}}) {
		t.Errorf("FileLines = %+v", got)
	}
}
