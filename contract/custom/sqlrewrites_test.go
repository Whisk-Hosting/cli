package custom

import (
	"reflect"
	"testing"
)

func TestSQLRewrites(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []SQLMove
	}{
		{"a type change", "ALTER TABLE orders ALTER COLUMN total TYPE numeric(12,2);", []SQLMove{{Name: "orders.total", Line: 1}}},
		{"set data type, no column word", "alter table if exists only public.orders\n  alter total set data type bigint;", []SQLMove{{Name: "orders.total", Line: 2}}},
		{"two in one statement", "ALTER TABLE t ALTER a TYPE text, ALTER COLUMN b TYPE int, ADD COLUMN c int;", []SQLMove{{Name: "t.a", Line: 1}, {Name: "t.b", Line: 1}}},
		{"a default is no rewrite", "ALTER TABLE t ALTER COLUMN a SET DEFAULT 0; ALTER TABLE t ALTER COLUMN a DROP NOT NULL;", nil},
		{"a table made in the same file", "CREATE TABLE t (a int); ALTER TABLE t ALTER COLUMN a TYPE bigint;", nil},
		{"only the up half", "-- +goose Up\nSELECT 1;\n-- +goose Down\nALTER TABLE t ALTER COLUMN a TYPE int;", nil},
		{"in a string or comment", "SELECT 'ALTER TABLE t ALTER COLUMN a TYPE int'; -- ALTER TABLE t ALTER a TYPE int", nil},
	}
	for _, tc := range cases {
		if got := SQLRewrites(tc.sql); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: SQLRewrites = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
