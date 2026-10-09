package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/apitypes"
	rules "github.com/whisk-run/contract/doctor"
)

// A minimal Hono app that passes every rule, used as the base for the fixtures below.
var passing = map[string]string{
	"whisk.yaml": `whisk: 1
name: fixture
routes:
  public: ["/", "/health"]
health:
  path: /health
database: app
secrets: [API_KEY, STRIPE_WEBHOOK_SECRET]
functions:
  - name: nightly
    cron: "0 6 * * *"
    graph: workflows/nightly.graph.yaml
webhooks:
  - name: stripe
    preset: stripe
    secret: STRIPE_WEBHOOK_SECRET
    handler: /hooks/stripe
`,
	"workflows/nightly.graph.yaml": "steps:\n  - id: count\n    next: record\n  - id: record\n",
	"package.json":                 `{"name":"fixture","dependencies":{"hono":"^4"}}`,
	"package-lock.json":            `{"name":"fixture","lockfileVersion":3}`,
	"src/index.ts": `import { Hono } from "hono";
const app = new Hono();
app.get("/", (c) => c.text("hi"));
app.get("/health", (c) => c.text("ok"));
app.post("/hooks/stripe", deliveries.handle(recordEvent, () => {}));
const key = process.env.API_KEY;
export const port = Number(process.env.PORT ?? 8080);
export default { port, fetch: app.fetch, hostname: "0.0.0.0" };
`,
	"src/functions.ts": `export const nightly = inngest.createFunction({ id: "nightly" }, { cron: "0 6 * * *" }, async ({ step }) => {
  const n = await step.run("count", () => 1);
  return step.run("record", () => n);
});
`,
	"Dockerfile": "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nUSER node\nEXPOSE 8080\nCMD [\"node\",\"dist/index.js\"]\n",
	".gitignore": ".env\n.env.*\n!.env.example\n",
}

func with(base map[string]string, changes map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range changes {
		if v == "" {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func writeAll(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runDoctor(t *testing.T, files map[string]string, fix bool) Report {
	t.Helper()
	dir := writeAll(t, files)
	rep, err := Run(context.Background(), Options{Dir: dir, Fix: fix})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func has(rep Report, id string) bool {
	for _, f := range rep.Findings {
		if f.Rule == id {
			return true
		}
	}
	return false
}

func TestPassingFixtureIsClean(t *testing.T) {
	rep := runDoctor(t, passing, false)
	if len(rep.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v", rep.Findings)
	}
}

// One triggering fixture per rule. W003, W070 and W080 need a binding, a git history or the
// platform and are covered below.
func TestEachRuleTriggers(t *testing.T) {
	cases := map[string]map[string]string{
		"W001": with(passing, map[string]string{"whisk.yaml": ""}),
		"W002": with(passing, map[string]string{"whisk.yaml": "whisk: 1\nname: fixture\nhealth:\n  path: nope\n"}),
		"W010": with(passing, map[string]string{"src/config.ts": `export const key = "sk_live_` + strings.Repeat("a1b2c3d4", 4) + `";` + "\n"}),
		"W011": with(passing, map[string]string{".env": "API_KEY=value\n", ".gitignore": ""}),
		"W020": with(passing, map[string]string{"src/index.ts": strings.ReplaceAll(passing["src/index.ts"], "process.env.PORT ?? 8080", "8080")}),
		"W021": with(passing, map[string]string{"src/store.ts": `import fs from "node:fs";
fs.writeFileSync("./data/x.json", "{}");
`}),
		"W022": with(passing, map[string]string{"whisk.yaml": strings.ReplaceAll(passing["whisk.yaml"], "path: /health", "path: /healthz")}),
		"W030": with(passing, map[string]string{"src/x.ts": `export const t = process.env.XERO_TENANT_ID;` + "\n"}),
		"W031": with(passing, map[string]string{"src/index.ts": strings.ReplaceAll(passing["src/index.ts"], "process.env.API_KEY", `"none"`)}),
		"W040": with(passing, map[string]string{"workflows/nightly.graph.yaml": ""}),
		"W041": with(passing, map[string]string{"workflows/nightly.graph.yaml": "steps:\n  - id: count\n    next: publish\n  - id: publish\n"}),
		"W042": with(passing, map[string]string{"src/functions.ts": strings.ReplaceAll(passing["src/functions.ts"], `"record"`, `"upload"`)}),
		"W050": with(passing, map[string]string{"src/index.ts": strings.ReplaceAll(passing["src/index.ts"], `app.post("/hooks/stripe"`, `app.post("/hooks/other"`)}),
		"W051": with(passing, map[string]string{"whisk.yaml": strings.ReplaceAll(passing["whisk.yaml"], "secrets: [API_KEY, STRIPE_WEBHOOK_SECRET]", "secrets: [API_KEY]")}),
		"W052": with(passing, map[string]string{"whisk.yaml": strings.ReplaceAll(passing["whisk.yaml"], `public: ["/", "/health"]`, `public: ["/", "/health", "/hooks/stripe"]`)}),
		"W053": with(passing, map[string]string{"src/index.ts": strings.ReplaceAll(passing["src/index.ts"], `deliveries.handle(recordEvent, () => {})`, `(c) => c.text("ok")`)}),
		"W060": with(passing, map[string]string{"Dockerfile": "FROM node:22-slim\nCMD [\"node\",\"x\"]\n"}),
		"W061": with(passing, map[string]string{"package-lock.json": ""}),
		"W062": with(passing, map[string]string{
			"Dockerfile": "FROM node:24-trixie-slim AS build\nFROM gcr.io/distroless/nodejs24-debian13:nonroot\nUSER nonroot\nEXPOSE 8080\nCMD [\"dist/index.js\"]\n",
			"whisk.yaml": passing["whisk.yaml"] + "migrate: \"npm run migrate\"\n",
		}),
		"W090": with(passing, map[string]string{"public/login.html": `<form method="post">\n  <input type=password name="pw">\n</form>\n`}),
	}
	for id, files := range cases {
		t.Run(id, func(t *testing.T) {
			rep := runDoctor(t, files, false)
			if !has(rep, id) {
				t.Errorf("%s did not trigger; findings: %+v; skipped: %v", id, rep.Findings, rep.Skipped)
			}
			for _, f := range rep.Findings {
				if r, ok := rules.Lookup(f.Rule); !ok || f.Level != r.Level || f.Fix == "" {
					t.Errorf("finding %+v does not carry the catalogue's level and fix", f)
				}
			}
		})
	}
}

func TestW062Commands(t *testing.T) {
	cases := []struct {
		image, migrate string
		want           bool
	}{
		{"gcr.io/distroless/nodejs24-debian13:nonroot", "node dist/migrate.js", false},
		{"gcr.io/distroless/nodejs24-debian13:nonroot", "npm run migrate", true},
		{"gcr.io/distroless/nodejs24-debian13:nonroot", "node a.js && node b.js", true},
		{"gcr.io/distroless/nodejs24-debian13:debug", "npm run migrate", false},
		{"--platform=linux/amd64 gcr.io/distroless/static-debian12", "sh -c ./migrate", true},
		{"scratch", "/app/server migrate", false},
		{"node:24-trixie-slim", "npm run migrate", false},
	}
	for _, c := range cases {
		files := with(passing, map[string]string{
			"Dockerfile": "FROM " + c.image + "\nUSER nonroot\nEXPOSE 8080\nCMD [\"x\"]\n",
			"whisk.yaml": passing["whisk.yaml"] + "migrate: \"" + c.migrate + "\"\n",
		})
		if got := has(runDoctor(t, files, false), "W062"); got != c.want {
			t.Errorf("FROM %s, migrate %q: W062 = %v, want %v", c.image, c.migrate, got, c.want)
		}
	}
}

func TestW063PackageFindings(t *testing.T) {
	lock := `{"lockfileVersion":3,"packages":{"":{},"node_modules/lodash":{"version":"4.17.15"},"node_modules/a/node_modules/axios":{"version":"1.11.0"}}}`
	r, err := Load(context.Background(), writeAll(t, with(passing, map[string]string{"package-lock.json": lock})))
	if err != nil {
		t.Fatal(err)
	}
	f := func(pkg, version, fixed, sev, where, path string) api.PackageFinding {
		return api.PackageFinding{ID: "CVE-" + pkg + "-" + fixed, Package: pkg, Version: version, Fixed: fixed, Severity: apitypes.PackageSeverity(sev), Where: apitypes.PackageWhere(where), Path: path}
	}
	scan := &api.PackageScan{Findings: []api.PackageFinding{
		f("lodash", "4.17.15", "4.17.19", "high", "source", "package-lock.json"),
		f("lodash", "4.17.15", "4.17.21", "critical", "source", "package-lock.json"),
		f("axios", "1.11.0", "1.12.0", "high", "source", "package-lock.json"),
		f("ws", "8.18.3", "8.21.0", "high", "source", "package-lock.json"),
		f("zlib1g", "1.2.13", "1.2.13-2", "critical", "image", ""),
		f("perl-base", "5.36.0", "", "critical", "image", ""),
		f("brace-expansion", "2.0.2", "2.1.6", "medium", "image", ""),
	}}
	// The check of the local lockfiles: lodash is fixed locally, minimist is new, and the live
	// scan's lockfile findings give way to it.
	check := &api.PackageCheck{Findings: []api.PackageFinding{
		f("minimist", "1.2.0", "1.2.6", "critical", "source", "package-lock.json"),
		f("qs", "6.5.2", "", "high", "source", "package-lock.json"),
	}}
	cases := []struct {
		name  string
		pkgs  *api.Packages
		check *api.PackageCheck
		want  []string
	}{
		{"not asked", nil, nil, nil},
		{"other plan", &api.Packages{Included: false}, nil, nil},
		{"no scan yet", &api.Packages{Included: true}, nil, nil},
		{"no scan yet, lockfiles checked", &api.Packages{Included: true}, check, []string{
			"package-lock.json: minimist 1.2.0 in the lockfile has 1 known critical vulnerability (CVE-minimist-1.2.6); 1.2.6 fixes it.",
		}},
		{"lockfiles checked before the deploy, image from the live scan", &api.Packages{Included: true, Scan: scan}, check, []string{
			"Dockerfile: zlib1g 1.2.13 in the live image has 1 known critical vulnerability (CVE-zlib1g-1.2.13-2); 1.2.13-2 fixes it.",
			"package-lock.json: minimist 1.2.0 in the lockfile has 1 known critical vulnerability (CVE-minimist-1.2.6); 1.2.6 fixes it.",
		}},
		{"lockfiles checked, nothing to fix", &api.Packages{Included: true}, &api.PackageCheck{}, nil},
		{"serious with a fix, still in the lockfile or image", &api.Packages{Included: true, Scan: scan}, nil, []string{
			"Dockerfile: zlib1g 1.2.13 in the live image has 1 known critical vulnerability (CVE-zlib1g-1.2.13-2); 1.2.13-2 fixes it.",
			"package-lock.json: lodash 4.17.15 in the lockfile has 2 known critical vulnerabilities (CVE-lodash-4.17.19); 4.17.21 fixes them.",
			"package-lock.json: axios 1.11.0 in the lockfile has 1 known high vulnerability (CVE-axios-1.12.0); 1.12.0 fixes it.",
		}},
	}
	for _, c := range cases {
		var got []string
		for _, x := range Check(r, Context{Packages: c.pkgs, LockCheck: c.check, RepoLimit: FreeRepoLimit}).Findings {
			if x.Rule == "W063" {
				got = append(got, x.File+": "+x.Message)
			}
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// Run sends the local lockfiles for a check only when the plan includes scanning, leaves out
// installed dependencies and other files, and reports a failed check as a skip.
func TestRunChecksLockfiles(t *testing.T) {
	dir := writeAll(t, with(passing, map[string]string{
		"package-lock.json":                `{"lockfileVersion":3}`,
		"worker/uv.lock":                   "version = 1\n",
		"node_modules/x/package-lock.json": "{}",
		"src/package-lock.json.bak":        "{}",
		"vendor/github.com/a/b/go.mod":     "module b\n",
	}))
	var sent []string
	checker := func(err error) func(context.Context, []api.Lockfile) (api.PackageCheck, error) {
		return func(_ context.Context, files []api.Lockfile) (api.PackageCheck, error) {
			sent = sent[:0]
			for _, f := range files {
				sent = append(sent, f.Path)
			}
			return api.PackageCheck{}, err
		}
	}
	packages := func(included bool) func(context.Context) (api.Packages, error) {
		return func(context.Context) (api.Packages, error) { return api.Packages{Included: included}, nil }
	}

	if _, err := Run(context.Background(), Options{Dir: dir, Packages: packages(false), CheckLockfiles: checker(nil)}); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent lockfiles on a plan without scanning: %v", sent)
	}
	if _, err := Run(context.Background(), Options{Dir: dir, Packages: packages(true), CheckLockfiles: checker(nil)}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(sent)
	if strings.Join(sent, ",") != "package-lock.json,worker/uv.lock" {
		t.Fatalf("sent %v", sent)
	}
	rep, err := Run(context.Background(), Options{Dir: dir, Packages: packages(true), CheckLockfiles: checker(errors.New("PLATFORM_UNAVAILABLE"))})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rep.Skipped, "\n"), "the local lockfiles could not be checked") {
		t.Fatalf("skipped: %v", rep.Skipped)
	}
}

func TestVersionLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"4.17.19", "4.17.21", true}, {"4.17.21", "4.17.19", false}, {"1.9.0", "1.10.0", true}, {"2.1", "2.1.1", true}, {"v1.2.0", "1.3.0", true}} {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestW003NameMismatch(t *testing.T) {
	files := with(passing, map[string]string{".whisk/app.json": `{"org":"acme","app":"other","api":"https://api.whisk.run"}`})
	if rep := runDoctor(t, files, false); !has(rep, "W003") {
		t.Errorf("W003 did not trigger: %+v", rep.Findings)
	}
	files[".whisk/app.json"] = `{"org":"acme","app":"fixture","api":"https://api.whisk.run"}`
	if rep := runDoctor(t, files, false); has(rep, "W003") {
		t.Errorf("W003 fired on a matching binding")
	}
}

func TestW020Loopback(t *testing.T) {
	files := with(passing, map[string]string{"src/index.ts": strings.ReplaceAll(passing["src/index.ts"], `"0.0.0.0"`, `"127.0.0.1"`)})
	rep := runDoctor(t, files, false)
	if !has(rep, "W020") {
		t.Errorf("loopback listen not reported: %+v", rep.Findings)
	}
}

func TestPythonAndGoIdioms(t *testing.T) {
	py := map[string]string{
		"whisk.yaml":     "whisk: 1\nname: pyapp\nroutes:\n  public: [\"/health\"]\nsecrets: [TOKEN]\n",
		"pyproject.toml": "[project]\nname = \"pyapp\"\ndependencies = [\"fastapi\"]\n",
		"uv.lock":        "version = 1\n",
		"app/main.py":    "from fastapi import FastAPI\nimport os\napp = FastAPI()\n@app.get(\"/health\")\ndef health():\n    return {\"token\": os.environ.get(\"TOKEN\"), \"port\": os.getenv(\"PORT\")}\n",
		"Dockerfile":     "FROM python:3.13-slim\nRUN useradd app\nUSER app\nCMD [\"sh\",\"-c\",\"uvicorn app.main:app --port ${PORT:-8080}\"]\n",
	}
	if rep := runDoctor(t, py, false); len(rep.Findings) != 0 {
		t.Errorf("python fixture: %+v", rep.Findings)
	}
	gofiles := map[string]string{
		"whisk.yaml": "whisk: 1\nname: goapp\nroutes:\n  public: [\"/health\"]\nsecrets: [TOKEN]\n",
		"go.mod":     "module goapp\n\ngo 1.23\n\nrequire github.com/go-chi/chi/v5 v5.0.0\n",
		"go.sum":     "x\n",
		"main.go":    "package main\nimport (\"net/http\"; \"os\")\nfunc main() {\n\tmux := http.NewServeMux()\n\tmux.HandleFunc(\"GET /health\", func(w http.ResponseWriter, r *http.Request) {})\n\t_ = os.Getenv(\"TOKEN\")\n\thttp.ListenAndServe(\"0.0.0.0:\"+os.Getenv(\"PORT\"), mux)\n}\n",
	}
	if rep := runDoctor(t, gofiles, false); len(rep.Findings) != 0 {
		t.Errorf("go fixture: %+v", rep.Findings)
	}
}

func TestFixes(t *testing.T) {
	files := with(passing, map[string]string{
		"whisk.yaml": strings.ReplaceAll(strings.ReplaceAll(passing["whisk.yaml"], "secrets: [API_KEY, STRIPE_WEBHOOK_SECRET]", "secrets: [API_KEY] # keep"), `public: ["/", "/health"]`, `public: ["/", "/health", "/hooks/stripe"]`),
		".env":       "API_KEY=x\n",
		".gitignore": "node_modules\n",
	})
	files["workflows/nightly.graph.yaml"] = "\n"
	dir := writeAll(t, files)
	rep, err := Run(context.Background(), Options{Dir: dir, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Fixed) == 0 {
		t.Fatalf("nothing fixed; findings %+v", rep.Findings)
	}
	m, _ := os.ReadFile(filepath.Join(dir, "whisk.yaml"))
	if !strings.Contains(string(m), "STRIPE_WEBHOOK_SECRET") || strings.Contains(string(m), "/hooks/stripe\"]") || !strings.Contains(string(m), "# keep") {
		t.Errorf("manifest after fix:\n%s", m)
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(gi), ".env.*") || !strings.HasPrefix(string(gi), "node_modules\n") {
		t.Errorf(".gitignore after fix:\n%s", gi)
	}
	g, _ := os.ReadFile(filepath.Join(dir, "workflows/nightly.graph.yaml"))
	if !strings.Contains(string(g), "id: count") || !strings.Contains(string(g), "id: record") {
		t.Errorf("graph after fix:\n%s", g)
	}
	for _, f := range rep.Findings {
		if f.Rule == "W051" || f.Rule == "W052" || f.Rule == "W040" {
			t.Errorf("%s still reported after fix: %s", f.Rule, f.Message)
		}
	}
	if !strings.Contains(strings.Join(rep.Fixed, "\n"), "git rm --cached .env") {
		t.Errorf("the git rm for the tracked .env should be reported, got %v", rep.Fixed)
	}
}

// The three templates pass doctor with zero warnings (CONTRACT.md §11).
func TestTemplatesAreClean(t *testing.T) {
	for _, name := range []string{"typescript", "python", "go"} {
		dir := filepath.Join("..", "..", "templates", name)
		if _, err := os.Stat(dir); err != nil {
			t.Skipf("%s not present", dir)
		}
		rep, err := Run(context.Background(), Options{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			// The TypeScript template runs from node_modules until it bundles its build
			// (pull request #189 does); W104 is its one expected finding until then.
			if name == "typescript" && f.Rule == "W104" {
				continue
			}
			t.Errorf("%s: %s %s %s:%d %s", name, f.Rule, f.Level, f.File, f.Line, f.Message)
		}
	}
}

// Platform names are never reported as undeclared, including a WHISK_ name newer than this
// CLI's copy of environment.md (doctor-rules.md W030).
func TestW030IgnoresPlatformNames(t *testing.T) {
	rep := runDoctor(t, with(passing, map[string]string{"src/x.ts": "export const k = process.env.WHISK_DELIVERY_KEY;\nexport const n = process.env.WHISK_KEY_FROM_A_NEWER_PLATFORM;\nexport const i = process.env.INNGEST_DEV;\n"}), false)
	if has(rep, "W030") {
		t.Errorf("W030 reported a platform name: %+v", rep.Findings)
	}
}

// A secret read through a table of names is read: its name is a string literal, not a
// process.env.NAME (doctor-rules.md W031).
func TestW031NameTable(t *testing.T) {
	code := strings.ReplaceAll(passing["src/index.ts"], "process.env.API_KEY", `env[KEYS.api]`)
	code = "const KEYS = { api: 'API_KEY' };\nconst env = process.env;\n" + code
	rep := runDoctor(t, with(passing, map[string]string{"src/index.ts": code}), false)
	if has(rep, "W031") {
		t.Errorf("W031 reported a secret read through a table of names: %+v", rep.Findings)
	}
}

// A plain node:http server that compares the path itself is read as routes, so W022 and W050
// run on it; with no routes doctor can read at all, the skip says what went unchecked and why
// (doctor-rules.md W022, W050).
func TestPlainNodeServerRoutes(t *testing.T) {
	plain := `import http from "node:http";
const key = process.env.API_KEY;
http.createServer((req, res) => {
  const { pathname } = new URL(req.url, "http://x");
  switch (pathname) {
    case "/health": return res.end("ok");
    case "/hooks/stripe": return deliveries.handle(recordEvent)(req, res);
  }
  if (req.url === "/") return res.end("hi");
}).listen(Number(process.env.PORT ?? 8080), "0.0.0.0");
`
	rep := runDoctor(t, with(passing, map[string]string{"package.json": `{"name":"fixture"}`, "src/index.ts": plain}), false)
	for _, id := range []string{"W022", "W050"} {
		if has(rep, id) {
			t.Errorf("%s reported on a plain server that has the route: %+v", id, rep.Findings)
		}
		for _, sk := range rep.Skipped {
			if strings.HasPrefix(sk, id) {
				t.Errorf("%s skipped on a plain server: %s", id, sk)
			}
		}
	}

	unreadable := `import http from "node:http";
const key = process.env.API_KEY;
const routes = { "/": hi, "/health": ok, "/hooks/stripe": deliveries.handle(recordEvent) };
http.createServer((req, res) => routes[req.url](req, res)).listen(Number(process.env.PORT ?? 8080), "0.0.0.0");
`
	rep = runDoctor(t, with(passing, map[string]string{"package.json": `{"name":"fixture"}`, "src/index.ts": unreadable}), false)
	var w022, w050 string
	for _, sk := range rep.Skipped {
		switch {
		case strings.HasPrefix(sk, "W022"):
			w022 = sk
		case strings.HasPrefix(sk, "W050"):
			w050 = sk
		}
	}
	if !strings.Contains(w022, "could not confirm a GET route for the health path /health") || !strings.Contains(w022, "req.url ===") {
		t.Errorf("W022 skip = %q", w022)
	}
	if !strings.Contains(w050, "a POST route for webhook stripe at /hooks/stripe") {
		t.Errorf("W050 skip = %q", w050)
	}
}

func TestW090Promoted(t *testing.T) {
	r := Repo{Files: []File{{Path: "public/login.html", Content: []byte("<input type=password>\n")}}}
	cases := []struct {
		name     string
		v        *api.Validation
		findings int
		skipped  string
	}{
		{"not bound or not asked", nil, 1, ""},
		{"bound, not promoted", &api.Validation{}, 1, ""},
		{"bound, promoted", &api.Validation{Promoted: true}, 0, "W090: skipped, the app is a Promoted app, which may keep its own sign-in"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := w090(r, Context{Validation: tc.v})
			if len(o.findings) != tc.findings {
				t.Errorf("findings = %+v, want %d", o.findings, tc.findings)
			}
			if got := strings.Join(o.skipped, ";"); got != tc.skipped {
				t.Errorf("skipped = %q, want %q", got, tc.skipped)
			}
		})
	}
}

func TestPasswordFields(t *testing.T) {
	cases := map[string]bool{
		`<input type="password" name="pw">`:              true,
		`<input type='password'>`:                        true,
		`<Input type={"password"} />`:                    true,
		"field({ type: `password` })":                    true,
		`el.type = "password"`:                           true,
		`password = PasswordField("Password")`:           true,
		`widget=forms.PasswordInput()`:                   true,
		`<input type="text" name="password">`:            false,
		`body := url.Values{"grant_type": {"password"}}`: false,
		`const passwordReset = false`:                    false,
	}
	for src, want := range cases {
		r := Repo{Files: []File{{Path: "src/page.tsx", Content: []byte(src + "\n")}}}
		if got := len(passwordFields(r)) > 0; got != want {
			t.Errorf("%q: found = %v, want %v", src, got, want)
		}
	}
	r := Repo{Files: []File{
		{Path: "templates/login.jinja", Content: []byte("<p>\n<input type=password>\n<input type=password>\n")},
		{Path: "tests/login.html", Content: []byte(`<input type="password">`)},
		{Path: "node_modules/x/index.js", Content: []byte(`el.type = "password"`)},
	}}
	got := passwordFields(r)
	if len(got) != 1 || got[0].File != "templates/login.jinja" || got[0].Line != 2 {
		t.Errorf("passwordFields = %+v", got)
	}
}

// The inbox handler is a delivery route like a webhook's: doctor finds it in code and checks it
// verifies the delivery (doctor-rules.md W050, W053).
func TestInboxHandlerRoute(t *testing.T) {
	yaml := passing["whisk.yaml"] + "inbox:\n  handler: /inbound/email\n"
	cases := []struct {
		name string
		code string
		want []string
		none []string
	}{
		{"routed and verified", `app.post("/inbound/email", deliveries.handle(recordMail, () => {}));` + "\n", nil, []string{"W050", "W053"}},
		{"no route", "", []string{"W050"}, nil},
		{"routed but not verified", `app.post("/inbound/email", (c) => c.text("ok"));` + "\n", []string{"W053"}, []string{"W050"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := map[string]string{"whisk.yaml": yaml}
			if c.code != "" {
				files["src/mail.ts"] = "import { app } from \"./index\";\n" + c.code
			}
			rep := runDoctor(t, with(passing, files), false)
			for _, id := range c.want {
				if !has(rep, id) {
					t.Errorf("%s not reported: %+v", id, rep.Findings)
				}
			}
			for _, id := range c.none {
				if has(rep, id) {
					t.Errorf("%s reported: %+v", id, rep.Findings)
				}
			}
		})
	}
}
