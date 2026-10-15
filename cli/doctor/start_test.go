package doctor

import (
	"strconv"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
)

// The start-up rules (doctor-rules.md W100 to W103) fire on the passing fixture changed in one
// way each, and never on the fixture itself (TestPassingFixtureIsClean).
func TestStartRulesTrigger(t *testing.T) {
	cases := map[string]map[string]string{
		"W101": with(passing, map[string]string{"Dockerfile": "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nUSER node\nEXPOSE 8080\nCMD [\"npx\", \"tsx\", \"src/index.ts\"]\n"}),
		"W102": with(passing, map[string]string{"Dockerfile": "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nUSER node\nEXPOSE 8080\nCMD [\"sh\", \"-c\", \"npx prisma migrate deploy && node dist/index.js\"]\n"}),
		"W103": with(passing, map[string]string{"src/index.ts": "import { google } from \"googleapis\";\n" + passing["src/index.ts"]}),
	}
	for id, files := range cases {
		t.Run(id, func(t *testing.T) {
			rep := runDoctor(t, files, false)
			if !has(rep, id) {
				t.Errorf("%s did not trigger; findings: %+v; skipped: %v", id, rep.Findings, rep.Skipped)
			}
			for _, f := range rep.Findings {
				if f.Rule != id {
					t.Errorf("only %s should fire, got %+v", id, f)
				}
			}
		})
	}
}

// W100 reads the typical start from the platform's validate answer, and says why it was
// skipped when it could not know it.
func TestW100MeasuredStart(t *testing.T) {
	bound := memRepo(passing)
	bound.Binding = &config.Binding{Org: "sober", App: "customer-health"}
	slow := &api.Validation{Start: &api.AppStart{TypicalMS: 7312, Count: 10}}
	cases := []struct {
		name    string
		repo    Repo
		v       *api.Validation
		message string
		skipped string
	}{
		{"slow", bound, slow, "customer-health takes 7.3 s to start (typical of the last 10 starts). Visitors wait that long after it sleeps.", ""},
		{"one start", bound, &api.Validation{Start: &api.AppStart{TypicalMS: 4000, Count: 1}}, "customer-health takes 4.0 s to start (the one start recorded). Visitors wait that long after it sleeps.", ""},
		{"quick", bound, &api.Validation{Start: &api.AppStart{TypicalMS: 1100, Count: 10}}, "", ""},
		{"exactly the limit", bound, &api.Validation{Start: &api.AppStart{TypicalMS: SlowStartMS, Count: 10}}, "", ""},
		{"no starts yet", bound, &api.Validation{}, "", "W100: skipped, the platform has recorded no starts of this app yet"},
		{"zero starts", bound, &api.Validation{Start: &api.AppStart{}}, "", "W100: skipped, the platform has recorded no starts of this app yet"},
		{"not asked", bound, nil, "", "W100: skipped, the platform was not asked, so no start times are known"},
		{"unbound", memRepo(passing), slow, "", "W100: skipped, the directory is not bound to an app, so no start times are known"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := w100(tc.repo, Context{Validation: tc.v})
			got := ""
			if len(o.findings) == 1 {
				got = o.findings[0].Message
				if o.findings[0].Fix == "" || o.findings[0].Level != "warning" {
					t.Errorf("finding without the rule's level and fix: %+v", o.findings[0])
				}
			} else if len(o.findings) > 1 {
				t.Fatalf("one finding at most, got %+v", o.findings)
			}
			if got != tc.message {
				t.Errorf("message = %q, want %q", got, tc.message)
			}
			if strings.Join(o.skipped, "|") != tc.skipped {
				t.Errorf("skipped = %v, want %q", o.skipped, tc.skipped)
			}
		})
	}
	alwaysOn := memRepo(with(passing, map[string]string{"whisk.yaml": strings.Replace(passing["whisk.yaml"], "database: app\n", "database: app\nalways_on: true\n", 1)}))
	alwaysOn.Binding = bound.Binding
	if o := w100(alwaysOn, Context{Validation: slow}); len(o.findings) != 1 || !strings.HasSuffix(o.findings[0].Message, "Every deploy and restart waits that long.") {
		t.Errorf("an always-on app does not sleep: %+v", o)
	}
}

func TestTypeScriptAtStart(t *testing.T) {
	cases := map[string]string{
		`CMD ["npx", "tsx", "src/index.ts"]`:                     "tsx",
		`CMD ["tsx", "src/index.ts"]`:                            "tsx",
		`CMD npx tsx src/index.ts`:                               "tsx",
		`tsx src/index.ts`:                                       "tsx",
		`node --import tsx src/index.ts`:                         "tsx",
		`node --import=tsx src/index.ts`:                         "tsx",
		`node -r ts-node/register src/index.ts`:                  "ts-node",
		`node --loader ts-node/esm src/index.ts`:                 "ts-node",
		`ts-node src/index.ts`:                                   "ts-node",
		`ts-node-dev --respawn src/index.ts`:                     "ts-node-dev",
		`npm run migrate && tsx src/server.ts`:                   "tsx",
		`babel-node src/index.js`:                                "babel-node",
		`CMD ["node", "dist/index.js"]`:                          "",
		`node dist/index.js`:                                     "",
		`node src/app.tsx`:                                       "",
		`bun src/index.ts`:                                       "",
		`deno run -A src/main.ts`:                                "",
		`node --experimental-strip-types src/index.ts`:           "",
		`node dist/tsx-helpers.js`:                               "",
		`node packages/tsx/dist/cli.js`:                          "",
		`ENTRYPOINT ["/app/server"]`:                             "",
		`sh -c "exec uvicorn app.main:app --port ${PORT:-8080}"`: "",
	}
	for cmd, want := range cases {
		got := ""
		if m := tsAtStart.FindStringSubmatch(cmd); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%q: found %q, want %q", cmd, got, want)
		}
	}
}

func TestMigrateCommands(t *testing.T) {
	cases := map[string]bool{
		`npx prisma migrate deploy && node dist/index.js`:                       true,
		`prisma db push && node server.js`:                                      true,
		`drizzle-kit migrate && node dist/index.js`:                             true,
		`npx knex migrate:latest && node index.js`:                              true,
		`npx sequelize-cli db:migrate && node app.js`:                           true,
		`typeorm-ts-node-commonjs migration:run -d src/data-source.ts`:          true,
		`alembic upgrade head && uvicorn app.main:app`:                          true,
		`alembic -c alembic.ini upgrade head`:                                   true,
		`python -m alembic upgrade head`:                                        true,
		`python manage.py migrate && gunicorn site.wsgi`:                        true,
		`goose -dir migrations postgres "$DATABASE_URL" up && ./server`:         true,
		`migrate -path db/migrations -database "$DATABASE_URL" up`:              true,
		`npm run migrate && node dist/index.js`:                                 true,
		`yarn db:migrate && yarn start:prod`:                                    true,
		`pnpm run migrate:deploy && node dist/main.js`:                          true,
		`node dist/migrate.js && node dist/index.js`:                            true,
		`python migrate.py && python main.py`:                                   true,
		`CMD ["node", "dist/index.js"]`:                                         false,
		`node dist/index.js`:                                                    false,
		`ENTRYPOINT ["/app/server"]`:                                            false,
		`./server migrate`:                                                      false,
		`prisma generate && node dist/index.js`:                                 false,
		`drizzle-kit generate`:                                                  false,
		`alembic history`:                                                       false,
		`uvicorn app.main:app --host 0.0.0.0 --port ${PORT:-8080}`:              false,
		`node dist/index.js --no-migrate`:                                       false,
		`node dist/migrations-report.js`:                                        true,
		`gunicorn app:app --bind 0.0.0.0:$PORT`:                                 false,
		`npm ci && npm run build`:                                               false,
		`goose create add_users sql`:                                            false,
		`echo "goose, migrate and up are just words"`:                           false,
		`sh -c "exec uvicorn app.main:app --host 0.0.0.0 --port ${PORT:-8080}"`: false,
	}
	for cmd, want := range cases {
		got := false
		for _, re := range migrateCommands {
			if re.MatchString(cmd) {
				got = true
			}
		}
		if got != want {
			t.Errorf("%q: migrates = %v, want %v", cmd, got, want)
		}
	}
}

func TestMigrationsAtStartup(t *testing.T) {
	dockerfile := func(cmd string) string {
		return "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nUSER node\nEXPOSE 8080\n" + cmd + "\n"
	}
	cases := []struct {
		name    string
		changes map[string]string
		file    string
		line    int
		message string
	}{
		{"in the CMD", map[string]string{"Dockerfile": dockerfile(`CMD ["sh", "-c", "npx prisma migrate deploy && node dist/index.js"]`)},
			"Dockerfile", 6, "The start command runs migrations (prisma migrate deploy) every time the app starts; whisk.yaml migrate runs them once per deploy instead."},
		{"through npm start", map[string]string{
			"Dockerfile":   dockerfile(`CMD ["npm", "start"]`),
			"package.json": "{\n  \"name\": \"fixture\",\n  \"scripts\": {\n    \"build\": \"tsc\",\n    \"start\": \"npm run migrate && node dist/index.js\",\n    \"migrate\": \"drizzle-kit migrate\"\n  },\n  \"dependencies\": {\"hono\": \"^4\"}\n}\n"},
			"package.json", 5, "The start command runs migrations (npm run migrate) every time the app starts; whisk.yaml migrate runs them once per deploy instead."},
		{"in prestart, without a Dockerfile", map[string]string{
			"Dockerfile":   "",
			"package.json": "{\n  \"name\": \"fixture\",\n  \"scripts\": {\n    \"prestart\": \"prisma migrate deploy\",\n    \"start\": \"node dist/index.js\"\n  },\n  \"dependencies\": {\"hono\": \"^4\"}\n}\n"},
			"package.json", 4, "The start command runs migrations (prisma migrate deploy) every time the app starts; whisk.yaml migrate runs them once per deploy instead."},
		{"in a shell script the CMD runs", map[string]string{
			"Dockerfile":        dockerfile(`CMD ["/app/scripts/start.sh"]`),
			"scripts/start.sh":  "#!/bin/sh\nset -e\n# migrate first\nnpx knex migrate:latest\nexec node dist/index.js\n",
			"src/index.ts":      passing["src/index.ts"],
			"src/functions.ts":  passing["src/functions.ts"],
			"package-lock.json": passing["package-lock.json"]},
			"scripts/start.sh", 4, "The start command runs migrations (knex migrate:latest) every time the app starts; whisk.yaml migrate runs them once per deploy instead."},
		{"in the entry code, with migrate already set", map[string]string{
			"whisk.yaml":   strings.Replace(passing["whisk.yaml"], "database: app\n", "database: app\nmigrate: node dist/migrate.js\n", 1),
			"src/index.ts": "import { migrate } from \"drizzle-orm/postgres-js/migrator\";\n// await migrate(db, { migrationsFolder: \"old\" });\n/* a\nblock */\nawait migrate(db, { migrationsFolder: \"drizzle\" });\n" + passing["src/index.ts"]},
			"src/index.ts", 5, "src/index.ts runs migrations when the app starts; whisk.yaml migrate already runs them once per deploy."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := w102(memRepo(with(passing, tc.changes)), Context{})
			if len(o.findings) != 1 {
				t.Fatalf("want one finding, got %+v (skipped %v)", o.findings, o.skipped)
			}
			f := o.findings[0]
			if f.File != tc.file || f.Line != tc.line || f.Message != tc.message {
				t.Errorf("finding = %s:%d %q\nwant      %s:%d %q", f.File, f.Line, f.Message, tc.file, tc.line, tc.message)
			}
		})
	}

	// A migrate file only the manifest runs is not the entry, and a dev script is not the start.
	clean := with(passing, map[string]string{
		"whisk.yaml":     strings.Replace(passing["whisk.yaml"], "database: app\n", "database: app\nmigrate: node dist/migrate.js\n", 1),
		"src/migrate.ts": "import { migrate } from \"drizzle-orm/postgres-js/migrator\";\nawait migrate(db, { migrationsFolder: \"drizzle\" });\n",
		"package.json":   `{"name":"fixture","scripts":{"start":"node dist/index.js","dev":"npm run migrate && tsx src/index.ts","migrate":"node dist/migrate.js"},"dependencies":{"hono":"^4"}}`,
	})
	if o := w102(memRepo(clean), Context{}); len(o.findings) != 0 {
		t.Errorf("a separate migrate file reported: %+v", o.findings)
	}
	if o := w101(memRepo(clean), Context{}); len(o.findings) != 0 {
		t.Errorf("a dev script reported: %+v", o.findings)
	}
}

func TestPythonStartRules(t *testing.T) {
	py := map[string]string{
		"whisk.yaml":      "whisk: 1\nname: pyapp\nroutes:\n  public: [\"/health\"]\nmigrate: alembic upgrade head\n",
		"pyproject.toml":  "[project]\nname = \"pyapp\"\n",
		"uv.lock":         "version = 1\n",
		"app/__init__.py": "",
		"app/main.py": `"""The app."""
import os
from contextlib import asynccontextmanager

import pandas as pd
from fastapi import FastAPI
from alembic import command

@asynccontextmanager
async def lifespan(app):
    command.upgrade(config, "head")
    yield

def report():
    import torch
    return torch.zeros(1)
`,
		"Dockerfile": "FROM python:3.13-slim\nUSER app\nCMD [\"sh\", \"-c\", \"exec uvicorn app.main:app --host 0.0.0.0 --port ${PORT:-8080}\"]\n",
	}
	r := memRepo(py)
	if got := entryFiles(r, startCommands(r)); len(got) != 1 || got[0].Path != "app/main.py" {
		t.Fatalf("entry files = %v", got)
	}
	heavy := w103(r, Context{})
	if len(heavy.findings) != 1 || heavy.findings[0].Line != 5 || !strings.Contains(heavy.findings[0].Message, "loads pandas") {
		t.Errorf("W103 = %+v", heavy.findings)
	}
	mig := w102(r, Context{})
	if len(mig.findings) != 1 || mig.findings[0].File != "app/main.py" || mig.findings[0].Line != 11 {
		t.Errorf("W102 = %+v", mig.findings)
	}
	if o := w101(r, Context{}); len(o.findings) != 0 {
		t.Errorf("W101 = %+v", o.findings)
	}
}

func TestHeavyImports(t *testing.T) {
	cases := []struct {
		path, src string
		name      string
		line      int
	}{
		{"src/index.ts", "import { Hono } from \"hono\";\nimport { google } from \"googleapis\";\n", "googleapis", 2},
		{"src/index.ts", "import AWS from 'aws-sdk';\n", "aws-sdk", 1},
		{"src/index.ts", "import {\n  a,\n  b,\n} from \"@tensorflow/tfjs-node\";\n", "@tensorflow/tfjs-node", 1},
		{"src/index.ts", "import \"puppeteer\";\n", "puppeteer", 1},
		{"src/index.js", "const admin = require(\"firebase-admin\");\n", "firebase-admin", 1},
		{"src/index.ts", "import { pipeline } from \"@huggingface/transformers\";\nimport { chromium } from \"playwright\";\n", "@huggingface/transformers", 1},
		{"src/index.ts", "import x from \"hono\"\nimport { chromium } from \"playwright\"\n", "playwright", 2},
		{"src/index.ts", "import type { drive_v3 } from \"googleapis\";\n", "", 0},
		{"src/index.ts", "import { S3Client } from \"@aws-sdk/client-s3\";\n", "", 0},
		{"src/index.ts", "import S3 from \"aws-sdk/clients/s3\";\n", "", 0},
		{"src/index.ts", "// import { google } from \"googleapis\";\n", "", 0},
		{"src/index.ts", "/*\nimport { google } from \"googleapis\";\n*/\n", "", 0},
		{"src/index.ts", "export async function sheets() {\n  const { google } = await import(\"googleapis\");\n  return google;\n}\n", "", 0},
		{"src/index.ts", "function f() {\n  const p = require(\"puppeteer\");\n}\n", "", 0},
		{"src/index.ts", "import { googleapisHelper } from \"./googleapis\";\n", "", 0},
		{"src/index.ts", "const s = \"import x from 'googleapis'\";\n", "", 0},
		{"app/main.py", "import os\nimport pandas as pd\n", "pandas", 2},
		{"app/main.py", "import os, torch\n", "torch", 1},
		{"app/main.py", "import matplotlib.pyplot as plt\n", "matplotlib", 1},
		{"app/main.py", "from sklearn.linear_model import LinearRegression\n", "sklearn", 1},
		{"app/main.py", "from transformers import pipeline\n", "transformers", 1},
		{"app/main.py", "def f():\n    import pandas as pd\n    return pd\n", "", 0},
		{"app/main.py", "# import pandas\n", "", 0},
		{"app/main.py", "import pandas_helpers\nfrom torchvision_lite import x\n", "", 0},
		{"app/main.py", "from .pandas import frame\n", "", 0},
		{"app/main.py", "if TYPE_CHECKING:\n    import pandas\n", "", 0},
	}
	for _, tc := range cases {
		h, ok := heavyImport(File{Path: tc.path, Content: []byte(tc.src)})
		if ok != (tc.name != "") || h.Name != tc.name || (ok && h.Line != tc.line) {
			t.Errorf("%s %q: got %+v %v, want %q at line %d", tc.path, tc.src, h, ok, tc.name, tc.line)
		}
	}
}

func TestEntryFiles(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"built JavaScript read as its source", map[string]string{"Dockerfile": "CMD [\"node\", \"dist/server.js\"]\n", "src/server.ts": "x", "src/routes.ts": "x"}, "src/server.ts"},
		{"an absolute path in the image", map[string]string{"Dockerfile": "CMD [\"node\", \"/app/build/main.js\"]\n", "src/main.ts": "x"}, "src/main.ts"},
		{"package.json main", map[string]string{"package.json": `{"main":"lib/app.js","scripts":{"start":"node ."}}`, "lib/app.js": "x"}, "lib/app.js"},
		{"python -m", map[string]string{"Dockerfile": "CMD [\"python\", \"-m\", \"service\"]\n", "service/__main__.py": "x"}, "service/__main__.py"},
		{"gunicorn module", map[string]string{"Dockerfile": "CMD gunicorn web.wsgi:application --bind 0.0.0.0:$PORT\n", "web/wsgi.py": "x"}, "web/wsgi.py"},
		{"a test file is never an entry", map[string]string{"Dockerfile": "CMD [\"node\", \"index.test.js\"]\n", "index.test.js": "x"}, ""},
		{"a migrate script is not the entry", map[string]string{"Dockerfile": "CMD sh -c \"node dist/migrate.js && node dist/app.js\"\n", "src/migrate.ts": "x", "src/app.ts": "x"}, "src/app.ts"},
		{"conventional names", map[string]string{"main.py": "x", "lib/util.py": "x"}, "main.py"},
		{"nothing", map[string]string{"lib/util.ts": "x"}, ""},
	}
	for _, tc := range cases {
		r := memRepo(tc.files)
		var got []string
		for _, f := range entryFiles(r, startCommands(r)) {
			got = append(got, f.Path)
		}
		if strings.Join(got, ",") != tc.want {
			t.Errorf("%s: entry files = %v, want %q", tc.name, got, tc.want)
		}
	}
}

// The start command follows npm scripts and shell scripts, and stops at a loop.
func TestStartCommandsFollowScripts(t *testing.T) {
	r := memRepo(map[string]string{
		"Dockerfile":   "FROM node:22 AS build\nCMD [\"npm\", \"run\", \"build\"]\nFROM node:22-slim\nENTRYPOINT [\"./entry.sh\"]\nCMD [\"npm\", \"start\"]\n",
		"entry.sh":     "#!/bin/sh\nexec \"$@\"\n",
		"package.json": "{\"scripts\": {\n\"start\": \"npm run serve\",\n\"serve\": \"npm run start\",\n\"build\": \"tsx build.ts\"\n}}\n",
	})
	var got []string
	for _, c := range startCommands(r) {
		got = append(got, c.File+":"+strconv.Itoa(c.Line))
	}
	if want := "Dockerfile:4,entry.sh:2,Dockerfile:5,package.json:2,package.json:3"; strings.Join(got, ",") != want {
		t.Errorf("start commands = %v, want %s", got, want)
	}
}

func TestW104Unbundled(t *testing.T) {
	final := "FROM node:22-slim AS build\nWORKDIR /app\nCOPY . .\nRUN npm ci && npm run build\n\nFROM node:22-slim\nWORKDIR /app\n"
	pkg := func(scripts string) string {
		return `{"name":"fixture","scripts":{` + scripts + `},"dependencies":{"hono":"^4","pino":"^10"}}`
	}
	cases := []struct {
		name  string
		files map[string]string
		line  int // 0: no finding
		file  string
	}{
		{"node_modules copied into the final stage", map[string]string{
			"package.json": pkg(`"build":"tsc"`),
			"Dockerfile":   final + "COPY --from=build /app/node_modules ./node_modules\nCOPY --from=build /app/dist ./dist\nUSER node\nCMD [\"node\", \"dist/index.js\"]\n"},
			11, "Dockerfile"},
		{"installed in the final stage", map[string]string{
			"package.json": pkg(`"build":"tsc"`),
			"Dockerfile":   "FROM node:22-slim\nWORKDIR /app\nCOPY package*.json ./\nRUN npm ci --omit=dev\nCOPY dist ./dist\nCMD node --enable-source-maps dist/server.mjs\n"},
			6, "Dockerfile"},
		{"no Dockerfile, dependencies, node start script", map[string]string{
			"package.json": "{\n\"name\": \"fixture\",\n\"scripts\": {\n\"build\": \"tsc\",\n\"start\": \"node dist/index.js\"\n},\n\"dependencies\": {\"hono\": \"^4\"}\n}\n"},
			5, "package.json"},
		{"npm start in the CMD runs node", map[string]string{
			"package.json": "{\"name\":\"fixture\",\"scripts\":{\n\"start\":\"node dist/index.js\"},\"dependencies\":{\"hono\":\"^4\"}}",
			"Dockerfile":   "FROM node:22-slim\nRUN npm ci\nCMD [\"npm\", \"start\"]\n"},
			2, "package.json"},
		{"bundled with esbuild in a script", map[string]string{
			"package.json": pkg(`"build":"esbuild src/index.ts --bundle --platform=node --format=esm --outfile=dist/index.mjs"`),
			"Dockerfile":   final + "COPY --from=build /app/node_modules ./node_modules\nCMD [\"node\", \"dist/index.mjs\"]\n"}, 0, ""},
		{"bundled with tsup", map[string]string{"package.json": pkg(`"build":"tsup src/index.ts","start":"node dist/index.js"`)}, 0, ""},
		{"bundled with ncc", map[string]string{"package.json": pkg(`"build":"ncc build src/index.ts -o dist","start":"node dist/index.js"`)}, 0, ""},
		{"bundled with bun build", map[string]string{"package.json": pkg(`"build":"bun build src/index.ts --target=node --outdir dist","start":"node dist/index.js"`)}, 0, ""},
		{"bundled with vite build --ssr", map[string]string{"package.json": pkg(`"build":"vite build --ssr src/server.ts","start":"node dist/server.js"`)}, 0, ""},
		{"vite build for the browser only is not a server bundle", map[string]string{"package.json": pkg(`"build":"vite build && tsc -p server","start":"node dist/server.js"`)}, 1, "package.json"},
		{"bundled in a Dockerfile RUN", map[string]string{
			"package.json": pkg(`"build":"tsc"`),
			"Dockerfile":   "FROM node:22-slim\nRUN npm ci && npx esbuild src/index.ts --bundle --platform=node --outfile=dist/index.js\nCMD [\"node\", \"dist/index.js\"]\n"}, 0, ""},
		{"a build script that loads esbuild", map[string]string{"package.json": pkg(`"build":"tsc --noEmit && node build.mjs","start":"node dist/index.js"`), "build.mjs": "import { build } from \"esbuild\";\n"}, 0, ""},
		{"a build script that bundles nothing", map[string]string{"package.json": pkg(`"build":"node build.mjs","start":"node dist/index.js"`), "build.mjs": "import { cp } from \"node:fs/promises\";\n"}, 1, "package.json"},
		{"a bundler config at the root", map[string]string{"package.json": pkg(`"build":"node build.mjs","start":"node dist/index.js"`), "rollup.config.mjs": "export default {}\n"}, 0, ""},
		{"no node_modules in the final image", map[string]string{
			"package.json": pkg(`"build":"tsc"`),
			"Dockerfile":   final + "COPY --from=build /app/dist ./dist\nCMD [\"node\", \"dist/index.js\"]\n"}, 0, ""},
		{"no dependencies, no Dockerfile", map[string]string{"package.json": `{"name":"fixture","scripts":{"start":"node index.js"}}`}, 0, ""},
		{"started with tsx, which W101 reports", map[string]string{"package.json": pkg(`"start":"tsx src/index.ts"`)}, 0, ""},
		{"started with next start", map[string]string{"package.json": pkg(`"build":"next build","start":"next start"`)}, 0, ""},
		{"not a Node app", map[string]string{"Dockerfile": "FROM python:3.13-slim\nRUN pip install -r requirements.txt\nCMD [\"python\", \"main.py\"]\n"}, 0, ""},
		{"started with bun", map[string]string{"package.json": pkg(`"start":"bun src/index.ts"`)}, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := w104(memRepo(tc.files), Context{})
			if tc.line == 0 {
				if len(o.findings) != 0 {
					t.Errorf("unexpected finding %+v", o.findings)
				}
				return
			}
			if len(o.findings) != 1 {
				t.Fatalf("want one finding, got %+v", o.findings)
			}
			f := o.findings[0]
			if f.File != tc.file || f.Line != tc.line {
				t.Errorf("finding at %s:%d, want %s:%d", f.File, f.Line, tc.file, tc.line)
			}
			if f.Message != "The app loads its libraries file by file at start, which is slow inside Whisk's sandbox." {
				t.Errorf("message = %q", f.Message)
			}
		})
	}
	named := memRepo(with(passing, map[string]string{"package.json": pkg(`"start":"node dist/index.js"`), "Dockerfile": ""}))
	if o := w104(named, Context{}); len(o.findings) != 1 || !strings.HasPrefix(o.findings[0].Message, "fixture loads its libraries") {
		t.Errorf("the message names the app from whisk.yaml: %+v", o.findings)
	}
}
