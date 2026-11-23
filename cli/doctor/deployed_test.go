package doctor

import (
	"slices"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/stack"
)

// What the deployed app runs: folders the build never ships, and folders for working on the app
// that nothing the deploy runs names or imports, are left out of the code the rules read.
func TestUndeployedFolders(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"a dev harness nothing names", map[string]string{"dev/stack.mjs": "const host = '127.0.0.1:5432';\n"}, []string{"dev"}},
		{"code set aside", map[string]string{"inactive/bills/index.ts": "process.env.BILLS_KEY;\n"}, []string{"inactive"}},
		{"a script the start command runs", map[string]string{
			"scripts/boot.ts": "export const x = 1;\n",
			"package.json":    `{"name":"fixture","scripts":{"start":"node scripts/boot.js && node dist/index.js"}}`,
		}, nil},
		{"a script npm start reaches through npm run", map[string]string{
			"scripts/seed.ts": "export const x = 1;\n",
			"package.json":    `{"name":"fixture","scripts":{"start":"npm run seed && node dist/index.js","seed":"node scripts/seed.js"}}`,
		}, nil},
		{"a dev script only npm run dev names", map[string]string{
			"dev/stack.mjs": "export const x = 1;\n",
			"package.json":  `{"name":"fixture","scripts":{"dev":"node dev/stack.mjs"}}`,
		}, []string{"dev"}},
		{"a tools folder the app imports", map[string]string{
			"tools/money.ts": "export const cents = 100;\n",
			"src/index.ts":   passing["src/index.ts"] + "import { cents } from \"../tools/money\";\n",
		}, nil},
		{"a folder the app loads by path", map[string]string{
			"examples/one.ts": "export default 1;\n",
			"src/load.ts":     "const mods = import.meta.glob(\"examples/*.ts\");\n",
		}, nil},
		{"a folder the Dockerfile does not copy", map[string]string{
			"Dockerfile":    "FROM node:22-slim\nWORKDIR /app\nCOPY package*.json ./\nCOPY src ./src\nUSER node\nCMD [\"node\",\"dist/index.js\"]\n",
			"notes/x.ts":    "process.env.NOTES;\n",
			"src/second.ts": "export const y = 2;\n",
		}, []string{"notes", "workflows"}},
		{"a folder .dockerignore leaves out", map[string]string{
			".dockerignore": "node_modules\nlab/\n",
			"lab/try.ts":    "process.env.LAB;\n",
		}, []string{"lab"}},
		{"an ordinary folder is the app", map[string]string{"server/db.ts": "export const x = 1;\n"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := repoOf(t, with(passing, c.files))
			if !slices.Equal(r.Undeployed, c.want) {
				t.Errorf("undeployed = %v, want %v", r.Undeployed, c.want)
			}
		})
	}
}

// The findings that came from code the deployed app never runs are gone, and the same code in
// the app still warns.
func TestUndeployedCodeIsNotRead(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		rule  string
		want  bool
	}{
		{"an undeclared variable set aside", map[string]string{"inactive/bills/index.ts": "const k = process.env.BILLS_KEY;\n"}, "W030", false},
		{"the same variable in the app", map[string]string{"src/bills.ts": "const k = process.env.BILLS_KEY;\n"}, "W030", true},
		{"a loopback listener in a dev harness", map[string]string{"dev/stack.mjs": "server.listen({ host: \"127.0.0.1\", port: 5432 });\n"}, "W020", false},
		{"a loopback listener in the app", map[string]string{"src/listen.ts": "server.listen({ host: \"127.0.0.1\", port: 5432 });\n"}, "W020", true},
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

// A timer in code that runs in the browser refreshes the page someone is looking at; only a
// timer in server code is W023's.
func TestBrowserTimers(t *testing.T) {
	vite := map[string]string{
		"ui/index.html":     "<!doctype html><div id=root></div><script type=\"module\" src=\"/src/main.tsx\"></script>\n",
		"ui/vite.config.ts": "export default {};\n",
	}
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"a refresh in a bundle's folder", with(vite, map[string]string{"ui/src/api.ts": "setInterval(load, 30000);\n"}), false},
		{"a clock in a React component", map[string]string{"web/ago.tsx": "useEffect(() => { const t = setInterval(tick, 1000); return () => clearInterval(t); }, []);\n"}, false},
		{"a root vite config naming its root", map[string]string{"vite.config.ts": "export default { root: \"frontend\" };\n", "frontend/poll.ts": "setInterval(load, 5000);\n"}, false},
		{"a timer in the server", map[string]string{"src/poll.ts": "setInterval(sync, 60000);\n"}, true},
		{"a timer beside a page that is not served by a bundle", map[string]string{"src/sync.ts": "setInterval(sync, 60000);\n", "src/page.html": "<p>hi</p>\n"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W023"); got != c.want {
				t.Errorf("W023 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}

// Schema changes in modules a migration imports run in the migrate step as the owner (W105).
func TestMigrationImports(t *testing.T) {
	restricted := passing["whisk.yaml"] + "database_role: restricted\nmigrate: \"node dist/migrate.js\"\n"
	setup := passing["whisk.yaml"] + "database_role: restricted\nmigrate: \"node dist/setup.js\"\n"
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"a module's schema the migration runs", map[string]string{
			"whisk.yaml":          restricted,
			"src/migrate.ts":      "import { schema } from \"./calls/schema.js\";\nawait sql.unsafe(schema);\n",
			"src/calls/schema.ts": "export const schema = `create table if not exists calls (id text)`;\n",
		}, false},
		{"through an index that re-exports it", map[string]string{
			"whisk.yaml":           restricted,
			"src/migrate.ts":       "import * as orders from \"./orders\";\nawait sql.unsafe(orders.schema);\n",
			"src/orders/index.ts":  "export { schema } from \"./schema\";\n",
			"src/orders/schema.ts": "export const schema = \"CREATE TABLE orders (id int)\";\n",
		}, false},
		{"the file the migrate command names", map[string]string{
			"whisk.yaml":   setup,
			"src/setup.ts": "await sql`create table t (id int)`;\n",
		}, false},
		{"python, from the migration", map[string]string{
			"whisk.yaml":      passing["whisk.yaml"] + "database_role: restricted\nmigrate: \"python migrate.py\"\n",
			"migrate.py":      "from app.models import SCHEMA\ncur.execute(SCHEMA)\n",
			"app/models.py":   "SCHEMA = \"CREATE TABLE things (id int)\"\n",
			"app/__init__.py": "",
		}, false},
		{"a schema change the migration does not import", map[string]string{
			"whisk.yaml":     restricted,
			"src/migrate.ts": "await sql`select 1`;\n",
			"src/boot.ts":    "await sql`create table if not exists sessions (id text)`;\n",
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W105"); got != c.want {
				t.Errorf("W105 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}

// A webhook route registered with a parameter, a built path or under a mounted prefix is found
// (W050), and its file is the one W053 reads.
func TestDynamicWebhookRoutes(t *testing.T) {
	yaml := strings.Replace(passing["whisk.yaml"], "    handler: /hooks/stripe\n", "    handler: /hooks/stripe\n  - name: dialpad\n    preset: stripe\n    secret: STRIPE_WEBHOOK_SECRET\n    handler: /hooks/dialpad\n", 1)
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"a template literal per automation", map[string]string{"src/hub/define.ts": "for (const h of hooks) app.post(`/hooks/${h.name}`, deliveries.handle(h.run));\n"}, false},
		{"an express parameter", map[string]string{"src/hub/define.ts": "app.post(\"/hooks/:name\", deliveries.handle(run));\n"}, false},
		{"a concatenation", map[string]string{"src/hub/define.ts": "app.post(\"/hooks/\" + name, deliveries.handle(run));\n"}, false},
		{"a router mounted at a prefix", map[string]string{"src/hub/define.ts": "const hooks = new Hono();\nhooks.post(\"/:name\", deliveries.handle(run));\napp.route(\"/hooks\", hooks);\n"}, false},
		{"FastAPI with a router prefix", map[string]string{"app/hooks.py": "router = APIRouter(prefix=\"/hooks\")\n@router.post(\"/{name}\")\nasync def hook(name: str): ...\n"}, false},
		{"chi inside a Route", map[string]string{"hooks.go": "package main\nfunc routes(r chi.Router) {\n\tr.Route(\"/hooks\", func(r chi.Router) {\n\t\tr.Post(\"/{name}\", handle)\n\t})\n}\n"}, false},
		{"net/http with a method and a wildcard", map[string]string{"hooks.go": "package main\nfunc routes(mux *http.ServeMux) {\n\tmux.HandleFunc(\"POST /hooks/{name}\", handle)\n}\n"}, false},
		{"no route anywhere for it", map[string]string{}, true},
		{"only a GET at the path", map[string]string{"src/hub/define.ts": "app.get(\"/hooks/:name\", show);\n"}, true},
		{"a bare catch-all does not count", map[string]string{"src/hub/define.ts": "app.post(\"/:anything\", h);\n"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, with(map[string]string{"whisk.yaml": yaml}, c.files)), false)
			if got := has(rep, "W050"); got != c.want {
				t.Errorf("W050 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}

// A graph step named in code by a name built at runtime is found (W041).
func TestBuiltStepNames(t *testing.T) {
	cases := []struct {
		lang  stack.Lang
		built string
		id    string
		want  bool
	}{
		{stack.JS, "send-orders-${batch}", "send-orders", true},
		{stack.JS, "send-orders/${batch}", "send-orders", true},
		{stack.JS, "${kind}-orders", "send-orders", true},
		{stack.JS, "send-orders-${batch}", "send-order", false},
		{stack.JS, "${kind}", "send-orders", false},
		{stack.PY, "send-orders-{batch}", "send-orders", true},
		{stack.GO, "send-orders-%d", "send-orders", true},
		{stack.GO, "send-%s", "send-orders", true},
		{stack.GO, "receive-%s", "send-orders", false},
	}
	for _, c := range cases {
		if got := builtNameCovers(c.lang, c.built, c.id); got != c.want {
			t.Errorf("builtNameCovers(%s, %q, %q) = %v, want %v", c.lang, c.built, c.id, got, c.want)
		}
	}
	files := with(passing, map[string]string{
		"workflows/nightly.graph.yaml": "steps:\n  - id: count\n    next: send-orders\n  - id: send-orders\n",
		"src/functions.ts":             "export const nightly = inngest.createFunction({ id: \"nightly\" }, { cron: \"0 6 * * *\" }, async ({ step }) => {\n  const n = await step.run(\"count\", () => 1);\n  for (const id of batches) await step.run(`send-orders-${id}`, () => n);\n});\n",
	})
	if rep := runDoctor(t, files, false); has(rep, "W041") {
		t.Errorf("W041 for a step built at runtime; findings %+v", rep.Findings)
	}
}
