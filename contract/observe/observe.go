// Package observe says whether an app's code reports its own errors and sends its own traces
// (SKILL.md §3 "Errors and traces"): the marks doctor's W140 and W141 look for, and the platform
// looks for in a deployed commit before it reports the exceptions it finds in the app's logs
// (CONTROL-PLANE.md §6.17 "Errors from the logs"). Each pattern is both RE2 and POSIX ERE, so
// `git grep -E` reads it as Go does.
package observe

import (
	"path"
	"strings"
)

// ErrorsPattern marks an app that reports its errors itself: the name SENTRY_DSN, or a Sentry
// SDK started in TypeScript, JavaScript, Python or Go.
const ErrorsPattern = `SENTRY_DSN|(Sentry|sentry_sdk|sentry)\.(init|Init)\(`

// TracesPattern marks an app that sends traces itself: the exporter's variable, an OpenTelemetry
// SDK or tracer provider, @vercel/otel, or an auto-instrumenting start command.
const TracesPattern = `OTEL_EXPORTER_OTLP_(TRACES_)?ENDPOINT|NodeSDK|TracerProvider|registerOTel|opentelemetry-instrument|@opentelemetry/auto-instrumentations-node/register`

// codeExts are the files whose code is read: TypeScript, JavaScript, Python and Go.
var codeExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts", ".py", ".go"}

// startFiles are the files that hold a start command as they are.
var startFiles = []string{"Dockerfile", "Procfile", "package.json"}

// skipDirs are folders that hold no code of the app's own, or only its tests.
var skipDirs = []string{"node_modules", ".venv", "venv", "vendor", "dist", "build", ".next", "__pycache__", ".whisk", ".git", "site-packages", "test", "tests", "__tests__", "spec", "fuzz"}

// Read reports whether a file is one the marks are looked for in: the app's own code, outside
// tests and vendored folders, or a file that holds its start command. Pure.
func Read(p string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		for _, d := range skipDirs {
			if seg == d {
				return false
			}
		}
	}
	base := path.Base(p)
	for _, f := range startFiles {
		if base == f {
			return true
		}
	}
	if strings.HasSuffix(base, ".Dockerfile") {
		return true
	}
	if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return false
	}
	ext := strings.ToLower(path.Ext(base))
	for _, e := range codeExts {
		if ext == e {
			return true
		}
	}
	return false
}

// Pathspecs are Read as git pathspecs, for `git grep` over a commit.
func Pathspecs() []string {
	out := []string{}
	for _, e := range codeExts {
		out = append(out, "*"+e)
	}
	for _, f := range startFiles {
		out = append(out, f, "*/"+f)
	}
	out = append(out, "*.Dockerfile")
	for _, d := range skipDirs {
		out = append(out, ":(exclude,glob)**/"+d+"/**")
	}
	return append(out, ":(exclude)*_test.go", ":(exclude)*.test.*", ":(exclude)*.spec.*", ":(exclude,glob)**/test_*")
}
