package doctor

import "testing"

// W095 on the passing fixture, with and without customers, per migration tool.
func TestPolicyRule(t *testing.T) {
	customers := passing["whisk.yaml"] + "customer_identity: app\n"
	force := "alter table orders enable row level security;\nalter table orders force row level security;\n"
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"sql table without a policy", map[string]string{"whisk.yaml": customers, "migrations/00002_orders.sql": "-- +goose Up\ncreate table orders (id bigserial primary key);\n"}, true},
		{"drizzle table, quoted and qualified", map[string]string{"whisk.yaml": customers, "drizzle/0001_orders.sql": `CREATE TABLE IF NOT EXISTS "public"."orders" ("id" serial);` + "\n"}, true},
		{"alembic table without a policy", map[string]string{"whisk.yaml": customers, "alembic/versions/0002_orders.py": "def upgrade():\n    op.create_table(\"orders\", sa.Column(\"id\", sa.Integer))\n"}, true},
		{"policy only enabled, not forced", map[string]string{"whisk.yaml": customers, "migrations/00002_orders.sql": "create table orders (id int);\nalter table orders enable row level security;\n"}, true},
		{"sql table with a forced policy", map[string]string{"whisk.yaml": customers, "migrations/00002_orders.sql": "create table orders (id int);\n" + force}, false},
		{"forced in a later migration", map[string]string{"whisk.yaml": customers, "migrations/00002_orders.sql": "create table orders (id int);\n", "migrations/00003_rls.sql": "ALTER TABLE ONLY \"Orders\" FORCE ROW LEVEL SECURITY;\n"}, false},
		{"alembic forced through op.execute", map[string]string{"whisk.yaml": customers, "alembic/versions/0002_orders.py": "def upgrade():\n    op.create_table(\"orders\", sa.Column(\"id\", sa.Integer))\n    op.execute(\"alter table orders force row level security\")\n"}, false},
		{"allowed with a reason", map[string]string{"whisk.yaml": customers, "migrations/00002_prices.sql": "-- doctor: allow W095 every customer reads the same price list\ncreate table prices (id int);\n"}, false},
		{"no customers", map[string]string{"migrations/00002_orders.sql": "create table orders (id int);\n"}, false},
		{"temporary table", map[string]string{"whisk.yaml": customers, "migrations/00002_tmp.sql": "create temporary table scratch (id int);\n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W095"); got != c.want {
				t.Errorf("W095 = %v, want %v; findings %+v; skipped %v", got, c.want, rep.Findings, rep.Skipped)
			}
		})
	}
}

func TestTableName(t *testing.T) {
	for ref, want := range map[string]string{`orders`: "orders", `"Orders"`: "orders", `public.orders`: "orders", `"public" . "orders"`: "orders", `app_01j.notes`: "notes"} {
		if got := tableName(ref); got != want {
			t.Errorf("%s: %q, want %q", ref, got, want)
		}
	}
}
