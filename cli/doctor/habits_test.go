package doctor

import (
	"strings"
	"testing"
)

// One triggering fixture and one near miss per rule in habits.go, on the passing fixture.
func TestHabitRules(t *testing.T) {
	manifest := passing["whisk.yaml"]
	cases := []struct {
		name  string
		rule  string
		files map[string]string
		want  bool
	}{
		{"static folder missing", "W004", map[string]string{"whisk.yaml": manifest + "static:\n  - { dir: dist/assets, path: /assets }\n"}, true},
		{"static folder ignored", "W004", map[string]string{"whisk.yaml": manifest + "static:\n  - { dir: dist/assets, path: /assets }\n", "dist/assets/app.js": "x", ".gitignore": passing[".gitignore"] + "dist/\n"}, true},
		{"static folder present", "W004", map[string]string{"whisk.yaml": manifest + "static:\n  - { dir: public/assets, path: /assets }\n", "public/assets/app.js": "x"}, false},

		{"timeout over the wake", "W005", map[string]string{"whisk.yaml": strings.Replace(manifest, "path: /health\n", "path: /health\n  timeout: 120\n", 1)}, true},
		{"timeout over the wake, always on", "W005", map[string]string{"whisk.yaml": strings.Replace(manifest, "path: /health\n", "path: /health\n  timeout: 120\n", 1) + "always_on: true\n"}, false},
		{"timeout within the wake", "W005", map[string]string{"whisk.yaml": strings.Replace(manifest, "path: /health\n", "path: /health\n  timeout: 45\n", 1)}, false},

		{"setInterval", "W023", map[string]string{"src/poll.ts": "setInterval(() => sync(), 60_000);\n"}, true},
		{"node-cron", "W023", map[string]string{"src/poll.ts": `import cron from "node-cron";` + "\n"}, true},
		{"apscheduler", "W023", map[string]string{"app/jobs.py": "from apscheduler.schedulers.background import BackgroundScheduler\n"}, true},
		{"robfig cron", "W023", map[string]string{"jobs.go": "package main\nimport \"github.com/robfig/cron/v3\"\n"}, true},
		{"setInterval in browser code", "W023", map[string]string{"public/clock.js": "setInterval(tick, 1000);\n"}, false},

		{"LISTEN", "W024", map[string]string{"src/listen.ts": "await client.query(\"LISTEN orders\");\n"}, true},
		{"advisory lock", "W024", map[string]string{"src/lock.ts": "await sql`select pg_advisory_lock(42)`;\n"}, true},
		{"SET search_path", "W024", map[string]string{"src/db2.ts": "await pool.query('SET search_path TO app');\n"}, true},
		{"transaction lock", "W024", map[string]string{"src/lock.ts": "await sql`select pg_advisory_xact_lock(42)`;\n"}, false},
		{"SET LOCAL", "W024", map[string]string{"src/db2.ts": "await tx.query('SET LOCAL statement_timeout = 5000');\n"}, false},
		{"in a migration", "W024", map[string]string{"src/migrate.ts": "await pool.query('SET search_path TO app');\n"}, false},

		{"function id differs", "W043", map[string]string{"whisk.yaml": strings.Replace(manifest, "name: nightly", "name: nightly-report", 1)}, true},
		{"function id matches", "W043", map[string]string{}, false},

		{"build secret not mounted", "W064", map[string]string{"whisk.yaml": manifest + "build:\n  secrets: [NPM_TOKEN]\n"}, true},
		{"build secret mounted", "W064", map[string]string{"whisk.yaml": manifest + "build:\n  secrets: [NPM_TOKEN]\n",
			"Dockerfile": "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nRUN --mount=type=secret,id=NPM_TOKEN,env=NPM_TOKEN npm ci\nUSER node\nEXPOSE 8080\nCMD [\"node\",\"dist/index.js\"]\n"}, false},

		{"cookie domain string", "W091", map[string]string{"src/c.ts": "c.header(\"Set-Cookie\", \"sid=1; Domain=example.com; Path=/\");\n"}, true},
		{"cookie domain option", "W091", map[string]string{"src/c.ts": "setCookie(c, \"sid\", \"1\", { domain: \".whisk.page\" });\n"}, true},
		{"go cookie domain", "W091", map[string]string{"c.go": "package main\nvar k = http.Cookie{Name: \"sid\", Domain: \"x\"}\n"}, true},
		{"cookie without domain", "W091", map[string]string{"src/c.ts": "setCookie(c, \"sid\", \"1\", { path: \"/\" });\n"}, false},

		{"authorization header read", "W092", map[string]string{"src/auth.ts": "const token = c.req.header(\"Authorization\");\n"}, true},
		{"jwt verify", "W092", map[string]string{"src/auth.ts": "const user = jwt.verify(token, key);\n"}, true},
		{"go authorization read", "W092", map[string]string{"auth.go": "package main\nfunc f(r *http.Request) string { return r.Header.Get(\"Authorization\") }\n"}, true},
		{"outgoing authorization", "W092", map[string]string{"src/call.ts": "await fetch(u, { headers: { authorization: `Bearer ${t}` } });\n", "app/call.py": "headers[\"Authorization\"] = \"Bearer \" + t\n"}, false},

		{"auth0", "W093", map[string]string{"src/auth.ts": `import { auth } from "express-openid-connect";` + "\n" + `import { ManagementClient } from "auth0";` + "\n"}, true},
		{"nodemailer", "W093", map[string]string{"src/mail.ts": `const nodemailer = require("nodemailer");` + "\n"}, true},
		{"smtplib", "W093", map[string]string{"app/mail.py": "import smtplib\n"}, true},
		{"redis client is fine", "W093", map[string]string{"src/kv.ts": `import { createClient } from "redis";` + "\n"}, false},

		{"customers never told apart", "W094", map[string]string{"whisk.yaml": manifest + "customer_identity: app\n"}, true},
		{"only the helper module scopes", "W094", map[string]string{"whisk.yaml": manifest + "customer_identity: org\n", "src/whisk.ts": "export const canSee = (id, o) => inScope(scopeFor(id), o);\n"}, true},
		{"helpers used", "W094", map[string]string{"whisk.yaml": manifest + "customer_identity: app\n", "src/orders.ts": "const scope = scopeFor(who(c));\n"}, false},
		{"python helpers used", "W094", map[string]string{"whisk.yaml": manifest + "customer_identity: app\n", "app/orders.py": "if not can_see(who, order.owner_id):\n    raise HTTPException(404)\n"}, false},
		{"go helpers used", "W094", map[string]string{"whisk.yaml": manifest + "customer_identity: app\n", "orders.go": "package main\nfunc ok(id Identity, o string) bool { return id.CanSee(o) }\n"}, false},
		{"audience compared", "W094", map[string]string{"whisk.yaml": manifest + "customer_identity: app\n", "src/orders.ts": "if (id.audience === \"customer\") q = q.where(eq(orders.ownerId, id.userId));\n"}, false},
		{"no customer sign-in", "W094", map[string]string{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, c.rule); got != c.want {
				t.Errorf("%s = %v, want %v; findings %+v; skipped %v", c.rule, got, c.want, rep.Findings, rep.Skipped)
			}
		})
	}
}

func TestStaticProblem(t *testing.T) {
	files := []File{{Path: "public/a.css", Tracked: true}, {Path: "dist/b.js", Tracked: false}}
	cases := []struct {
		dir         string
		git, onDisk bool
		want        string
	}{
		{"public", true, true, ""},
		{"dist", true, true, "holds no file git tracks"},
		{"dist", false, true, ""},
		{"assets", true, false, "does not exist"},
		{"assets", false, true, "holds no file that would be committed"},
	}
	for _, c := range cases {
		if got := staticProblem(files, c.dir, c.git, c.onDisk); !strings.HasPrefix(got, c.want) || (c.want == "" && got != "") {
			t.Errorf("%s git=%v disk=%v: %q, want %q", c.dir, c.git, c.onDisk, got, c.want)
		}
	}
}

func TestMountsSecret(t *testing.T) {
	cases := map[string]bool{
		"RUN --mount=type=secret,id=NPM_TOKEN npm ci":               true,
		"RUN --mount=id=NPM_TOKEN,type=secret npm ci":               true,
		"RUN --mount=type=secret,env=NPM_TOKEN npm ci":              true,
		"RUN --mount=type=secret,id=NPM_TOKEN_OLD npm ci":           false,
		"RUN --mount=type=cache,id=NPM_TOKEN,target=/root/.npm npm": false,
		"RUN npm ci": false,
	}
	for src, want := range cases {
		if got := mountsSecret(src, "NPM_TOKEN"); got != want {
			t.Errorf("%s: %v, want %v", src, got, want)
		}
	}
}
