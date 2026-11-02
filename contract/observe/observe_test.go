package observe

import (
	"regexp"
	"testing"
)

func TestRead(t *testing.T) {
	cases := map[string]bool{
		"src/index.ts": true, "app/main.py": true, "main.go": true, "server.mjs": true,
		"Dockerfile": true, "api/Dockerfile": true, "web.Dockerfile": true, "Procfile": true, "package.json": true, "apps/api/package.json": true,
		"src/index.test.ts": false, "main_test.go": false, "tests/test_app.py": false, "test/x.ts": false,
		"node_modules/x/index.js": false, "dist/index.js": false, "README.md": false, "styles.css": false,
	}
	for p, want := range cases {
		if got := Read(p); got != want {
			t.Errorf("Read(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestPatterns(t *testing.T) {
	errs, traces := regexp.MustCompile(ErrorsPattern), regexp.MustCompile(TracesPattern)
	for text, want := range map[string]bool{
		`if (process.env.SENTRY_DSN)`: true, `Sentry.init({})`: true, `sentry_sdk.init()`: true, `sentry.Init(sentry.ClientOptions{})`: true,
		`import * as Sentry from "@sentry/node"`: false, `console.error(err)`: false,
	} {
		if got := errs.MatchString(text); got != want {
			t.Errorf("errors %q: %v, want %v", text, got, want)
		}
	}
	for text, want := range map[string]bool{
		`new NodeSDK()`: true, `new NodeTracerProvider()`: true, `TracerProvider()`: true, `sdktrace.NewTracerProvider(`: true,
		`registerOTel({})`: true, `CMD ["opentelemetry-instrument", "uvicorn"]`: true, `process.env.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`: true,
		`OTEL_EXPORTER_OTLP_ENDPOINT`: true, `import { trace } from "@opentelemetry/api"`: false,
	} {
		if got := traces.MatchString(text); got != want {
			t.Errorf("traces %q: %v, want %v", text, got, want)
		}
	}
}
