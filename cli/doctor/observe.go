package doctor

import (
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/observe"
)

// Errors and traces (doctor-rules.md W140 and W141, SKILL.md §3 "Errors and traces"). The
// platform hands every container SENTRY_DSN and the OTEL_* variables; these rules warn while
// nothing in the app starts an SDK to use them, so its Errors and Traces pages stay empty.

var (
	// errorsStarted are the marks of an app that reports its errors (W140).
	errorsStarted = regexp.MustCompile(observe.ErrorsPattern)
	// tracesStarted are the marks of an app that sends traces (W141).
	tracesStarted = regexp.MustCompile(observe.TracesPattern)
	// staticServers are the images whose last stage only serves files, so the app runs no code
	// of its own to report from.
	staticServers = regexp.MustCompile(`^(?:docker\.io/)?(?:library/)?(?:nginx|nginxinc/[^:@]+|caddy|httpd|joseluisq/static-web-server|lipanski/docker-static-website|pierrezemb/gostatic)(?:[:@]|$)`)
)

func w140(r Repo, _ Context) outcome {
	return observeRule(r, "W140", errorsStarted, "Nothing in the app starts a Sentry SDK, so only the stacks it prints reach the Errors page, without the request or what the framework catches quietly.")
}

func w141(r Repo, _ Context) outcome {
	return observeRule(r, "W141", tracesStarted, "Nothing in the app starts an OpenTelemetry SDK, so its requests never reach the Traces page or whisk traces.")
}

// observeRule is W140 and W141: one finding when no file the app runs from carries a mark. Pure.
func observeRule(r Repo, id string, marks *regexp.Regexp, message string) outcome {
	if why := observeSkip(r); why != "" {
		return skip(id, why)
	}
	for _, f := range r.Files {
		if text := observedText(f); text != nil && marks.Match(text) {
			return outcome{}
		}
	}
	file, line := observeAt(r)
	return one(id, file, line, message)
}

// observeAt is where W140 and W141 are reported: the app's start command, where a Node app loads
// its instrument file (node --import ./instrument.mjs) and where a doctor: allow comment goes;
// whisk.yaml when doctor finds no start command. Pure.
func observeAt(r Repo) (string, int) {
	for _, c := range startCommands(r) {
		if c.File != "" && c.Line > 0 {
			return c.File, c.Line
		}
	}
	if r.HasManifest {
		return manifestFile, 0
	}
	return "", 0
}

// observeSkip says why the app has nothing of its own to report from, or "".
func observeSkip(r Repo) string {
	if len(r.Stack.Langs) == 0 {
		return "no TypeScript, JavaScript, Python or Go app here"
	}
	if names := dockerfiles(r); len(names) > 0 {
		if image := finalImage(instructions(r.Read(names[0]))); staticServers.MatchString(image) {
			return "the image " + image + " only serves files"
		}
	}
	return ""
}

// observedText is what W140 and W141 read of a file: the app's own code without its comments,
// and the files that hold its start command as they are; nil for anything else.
func observedText(f File) []byte {
	if f.Content == nil || stack.IsTest(f.Path) || stack.IsVendored(f.Path) {
		return nil
	}
	if lang := stack.LangOf(f.Path); lang != "" {
		return stripComments(lang, f.Content)
	}
	switch base := path.Base(f.Path); {
	case base == "Dockerfile", strings.HasSuffix(base, ".Dockerfile"), base == "Procfile", base == "package.json":
		return f.Content
	}
	return nil
}
