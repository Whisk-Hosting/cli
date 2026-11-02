package doctor

import "testing"

// W140 and W141 warn while nothing in the app starts a Sentry or an OpenTelemetry SDK, read from
// its code, Dockerfile, Procfile and package.json, never from tests or comments
// (doctor-rules.md W140, W141).
func TestErrorsAndTraces(t *testing.T) {
	bare := with(passing, map[string]string{"src/instrument.ts": ""})
	cases := []struct {
		name          string
		files         map[string]string
		errors, trace bool // whether W140 and W141 are reported
	}{
		{"both started", passing, false, false},
		{"neither", bare, true, true},
		{"errors only", with(bare, map[string]string{"src/errors.ts": "if (process.env.SENTRY_DSN) Sentry.init({});\n"}), false, true},
		{"Sentry reads the DSN itself", with(bare, map[string]string{"src/errors.ts": "Sentry.init({ environment: 'production' });\n"}), false, true},
		{"traces only", with(bare, map[string]string{"src/tracing.ts": "const p = new NodeTracerProvider();\n"}), true, false},
		{"@vercel/otel", with(bare, map[string]string{"instrumentation.ts": "export function register() { registerOTel({ serviceName: 'x' }); }\n"}), true, false},
		{"started from the Dockerfile", with(bare, map[string]string{"Dockerfile": "FROM python:3.13-slim\nUSER app\nCMD [\"opentelemetry-instrument\", \"uvicorn\", \"app.main:app\"]\n"}), true, false},
		{"only in a comment", with(bare, map[string]string{"src/later.ts": "// TODO: Sentry.init with SENTRY_DSN and a NodeSDK\n"}), true, true},
		{"only in a test", with(bare, map[string]string{"src/app.test.ts": "process.env.SENTRY_DSN = 'x'; new NodeSDK();\n"}), true, true},
		{"only in a dependency", with(bare, map[string]string{"node_modules/x/index.js": "Sentry.init({}); new NodeSDK();\n"}), true, true},
		{"python", map[string]string{"whisk.yaml": "whisk: 1\nname: py\n", "pyproject.toml": "[project]\nname = \"py\"\n", "app/main.py": "import os, sentry_sdk\nsentry_sdk.init()\nprovider = TracerProvider()\n"}, false, false},
		{"go", map[string]string{"whisk.yaml": "whisk: 1\nname: g\n", "go.mod": "module g\n", "main.go": "package main\nfunc main() { sentry.Init(sentry.ClientOptions{}); _ = sdktrace.NewTracerProvider() }\n"}, false, false},
		{"a static site", map[string]string{"whisk.yaml": "whisk: 1\nname: s\n", "package.json": `{"name":"s"}`, "Dockerfile": "FROM node:24 AS build\nRUN npm run build\nFROM nginx:1.29-alpine\nCOPY --from=build /dist /usr/share/nginx/html\n"}, false, false},
		{"no code doctor reads", map[string]string{"whisk.yaml": "whisk: 1\nname: r\n", "Dockerfile": "FROM ruby:3.4\nCMD [\"ruby\", \"app.rb\"]\n"}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, c.files, false)
			if got := has(rep, "W140"); got != c.errors {
				t.Errorf("W140 reported %v, want %v: %+v", got, c.errors, rep.Findings)
			}
			if got := has(rep, "W141"); got != c.trace {
				t.Errorf("W141 reported %v, want %v: %+v", got, c.trace, rep.Findings)
			}
		})
	}
}

// The findings point at the start command, where a Node app loads its instrument file, and one
// doctor: allow comment there can keep both out of the report.
func TestErrorsAndTracesAtTheStartCommand(t *testing.T) {
	bare := with(passing, map[string]string{"src/instrument.ts": ""})
	rep := runDoctor(t, bare, false)
	for _, f := range rep.Findings {
		if (f.Rule == "W140" || f.Rule == "W141") && (f.File != "Dockerfile" || f.Line != 6) {
			t.Errorf("%s at %s:%d, want Dockerfile:6", f.Rule, f.File, f.Line)
		}
	}
	allowed := with(bare, map[string]string{"Dockerfile": "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nUSER node\nEXPOSE 8080\n# doctor: allow W140 W141 the app is too big for the SDKs\nCMD [\"node\",\"dist/index.js\"]\n"})
	rep = runDoctor(t, allowed, false)
	if has(rep, "W140") || has(rep, "W141") {
		t.Errorf("an allow comment naming both did not keep them out: %+v", rep.Findings)
	}
}

func TestAllowsRule(t *testing.T) {
	cases := []struct {
		line, rule string
		want       bool
	}{
		{"// doctor: allow W091 the edge test", "W091", true},
		{"# doctor: allow W140 W141 memory", "W141", true},
		{"# doctor: allow W140, W141 memory", "W140", true},
		{"# doctor: allow W140 memory W141", "W141", false},
		{"# doctor: allow W1400", "W140", false},
		{"# nothing here", "W140", false},
	}
	for _, c := range cases {
		if got := allowsRule(c.line, c.rule); got != c.want {
			t.Errorf("allowsRule(%q, %s) = %v, want %v", c.line, c.rule, got, c.want)
		}
	}
}
