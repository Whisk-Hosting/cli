package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// One triggering fixture and one near miss per rule in security.go, on the passing fixture.
func TestSecurityRules(t *testing.T) {
	manifest := passing["whisk.yaml"]
	customers := map[string]string{"whisk.yaml": manifest + "customer_identity: app\n"}
	cases := []struct {
		name  string
		rule  string
		files map[string]string
		want  bool
	}{
		{"untagged template into query", "W096", map[string]string{"src/orders.ts": "await pool.query(`select * from orders where id = ${id}`);\n"}, true},
		{"unsafe with interpolation", "W096", map[string]string{"src/orders.ts": "await sql.unsafe(`delete from orders where id = ${id}`);\n"}, true},
		{"concatenated into query", "W096", map[string]string{"src/orders.ts": "await pool.query(\"select * from orders where name = '\" + name + \"'\");\n"}, true},
		{"python f-string", "W096", map[string]string{"app/orders.py": "conn.execute(text(f\"select * from orders where id = {order_id}\"))\n"}, true},
		{"python percent", "W096", map[string]string{"app/orders.py": "cur.execute(\"select * from orders where name = '%s'\" % name)\n"}, true},
		{"go sprintf", "W096", map[string]string{"orders.go": "package main\nvar q = fmt.Sprintf(\"select * from orders where name = '%s'\", name)\n"}, true},
		{"tagged template is parameterised", "W096", map[string]string{"src/orders.ts": "await sql`select * from orders where id = ${id}`;\n"}, false},
		{"query with parameters", "W096", map[string]string{"src/orders.ts": "await pool.query(\"select * from orders where id = $1\", [id]);\n", "app/orders.py": "cur.execute(\"select * from orders where id = %s\", (order_id,))\n"}, false},
		{"fetch with a template url", "W096", map[string]string{"src/api.ts": "await http.get(`/api/orders/${id}`);\n"}, false},
		{"english text joined", "W096", map[string]string{"app/copy.py": "msg = \"Update your details, \" + name\n"}, false},

		{"react raw html", "W097", map[string]string{"src/page.tsx": "<div dangerouslySetInnerHTML={{ __html: body }} />\n"}, true},
		{"innerHTML from a variable", "W097", map[string]string{"public/app.js": "el.innerHTML = note.body;\n"}, true},
		{"innerHTML from a template", "W097", map[string]string{"public/app.js": "el.innerHTML = `<b>${name}</b>`;\n"}, true},
		{"jinja safe", "W097", map[string]string{"templates/note.html": "<p>{{ note.body | safe }}</p>\n"}, true},
		{"go template.HTML", "W097", map[string]string{"page.go": "package main\nvar h = template.HTML(body)\n"}, true},
		{"innerHTML cleared", "W097", map[string]string{"public/app.js": "el.innerHTML = \"\";\nel.textContent = note.body;\n"}, false},
		{"escaped text", "W097", map[string]string{"templates/note.html": "<p>{{ note.body }}</p>\n"}, false},

		{"list not limited to the customer", "W098", with(customers, map[string]string{
			"migrations/001.sql": "create table orders (id serial primary key, customer_id text not null, total int);\n",
			"src/orders.ts":      "const rows = await sql.unsafe(\"select * from orders order by id desc\");\n"}), true},
		{"drizzle schema, raw list", "W098", with(customers, map[string]string{
			"src/schema.ts": "export const orders = pgTable(\"orders\", {\n  id: serial(\"id\").primaryKey(),\n  ownerId: text(\"owner_id\").notNull(),\n});\n",
			"src/report.ts": "const rows = await pool.query(\"select count(*) from orders\");\n"}), true},
		{"list limited to the customer", "W098", with(customers, map[string]string{
			"migrations/001.sql": "create table orders (id serial primary key, customer_id text not null);\n",
			"src/orders.ts":      "const rows = await pool.query(\"select * from orders where customer_id = $1\", [id.userId]);\n"}), false},
		{"one row by id, checked by the app", "W098", with(customers, map[string]string{
			"migrations/001.sql": "create table orders (id serial primary key, customer_id text not null);\n",
			"src/orders.ts":      "const [o] = await pool.query(\"select * from orders where id = $1\", [orderId]);\n"}), false},
		{"no customer sign-in", "W098", map[string]string{
			"migrations/001.sql": "create table orders (id serial primary key, customer_id text not null);\n",
			"src/orders.ts":      "const rows = await pool.query(\"select * from orders\");\n"}, false},

		{"public post without a challenge", "W099", map[string]string{"src/index.ts": passing["src/index.ts"] + "app.post(\"/\", (c) => c.text(\"saved\"));\n"}, true},
		{"public delete", "W099", map[string]string{"whisk.yaml": manifestWithPublic(manifest, `["/", "/health", "/api/**"]`), "src/api.ts": "app.delete(\"/api/notes/:id\", del);\n"}, true},
		{"admin made public", "W099", map[string]string{"whisk.yaml": manifestWithPublic(manifest, `["/", "/health", "/admin/*"]`)}, true},
		{"public form without a challenge", "W099", map[string]string{"whisk.yaml": manifestWithPublic(manifest, `["/", "/health", "/contact"]`), "src/contact.ts": "app.post(\"/contact\", save);\n"}, true},
		{"public form under a challenge", "W099", map[string]string{"whisk.yaml": withChallenge(manifestWithPublic(manifest, `["/", "/health", "/contact"]`), `["/contact"]`), "src/contact.ts": "app.post(\"/contact\", save);\n"}, false},
		{"private post", "W099", map[string]string{"src/notes.ts": "app.post(\"/notes\", save);\napp.delete(\"/notes/:id\", del);\n"}, false},
		{"webhook handler", "W099", map[string]string{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, c.rule); got != c.want {
				t.Errorf("%s = %v, want %v; findings %+v", c.rule, got, c.want, rep.Findings)
			}
		})
	}
}

func manifestWithPublic(manifest, list string) string {
	return replaceOnce(manifest, `public: ["/", "/health"]`, "public: "+list)
}

func withChallenge(manifest, list string) string {
	return replaceOnce(manifest, "routes:\n", "routes:\n  challenge: "+list+"\n")
}

func replaceOnce(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	panic("fixture text not found: " + old)
}

func TestUnscopedRead(t *testing.T) {
	cases := map[string]bool{
		"select * from orders":                                  true,
		"select count(*) from public.orders o":                  true,
		"select * from orders where customer_id = $1":           false,
		"select * from orders where id = $1":                    false,
		"select * from orders where orders.id = $1":             false,
		"update orders set total = 0":                           true,
		"select * from order_lines":                             false,
		"select o.* from things t join orders o on o.x = t.x":   true,
		"select * from orders where status = 'open' and id > 1": true,
	}
	for stmt, want := range cases {
		if got := unscopedRead(stmt, "orders", "customer_id"); got != want {
			t.Errorf("%q: %v, want %v", stmt, got, want)
		}
	}
}

func TestRouteGlob(t *testing.T) {
	cases := map[string]string{"/notes/:id": "/notes/x", "/notes/{id}": "/notes/x", "/a/<int:id>/b": "/a/x/b", "/plain": "/plain"}
	for in, want := range cases {
		if got := routeGlob(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

// The templates are what agents copy, so none of the security rules may fire on them.
func TestTemplatesPassSecurityRules(t *testing.T) {
	root := filepath.Join("..", "..", "templates")
	dirs := []string{"typescript", "python", "go", "canary/typescript", "canary/python", "canary/go"}
	for _, d := range dirs {
		dir := filepath.Join(root, d)
		if _, err := os.Stat(filepath.Join(dir, "whisk.yaml")); err != nil {
			t.Skipf("templates not beside the CLI: %v", err)
		}
		rep, err := Run(context.Background(), Options{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			switch f.Rule {
			case "W094", "W096", "W097", "W098", "W099":
				t.Errorf("%s: %s %s:%d %s", d, f.Rule, f.File, f.Line, f.Message)
			}
		}
	}
}
