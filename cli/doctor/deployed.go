package doctor

import (
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"

	"github.com/whisk-run/cli/internal/stack"
)

// What of the repository the deployed app runs, so the rules that read server code read only
// that (doctor-rules.md, "What doctor reads"). A folder is left out when the build never ships
// it (the Dockerfile copies other folders only, or .dockerignore leaves it out), or when it is a
// folder for working on the app (devFolders) that nothing the deploy runs names or imports.
// Code under a browser bundle's own folder runs in the browser, not on the server.

// devFolders are the top-level folders whose names say they hold code for working on the app
// rather than the app: local harnesses, tools, scripts, examples and code set aside.
var devFolders = map[string]bool{
	"dev": true, "devtools": true, "tools": true, "scripts": true, "bin": true, "examples": true,
	"example": true, "docs": true, "bench": true, "benchmarks": true, "playground": true,
	"sandbox": true, "scratch": true, "experiments": true, "inactive": true, "archive": true,
	"archived": true, "attic": true, "old": true, "unused": true, "deprecated": true,
	"disabled": true, "legacy": true,
}

// deployScripts are the package.json scripts a deploy runs: the build's and the start's, with
// their pre and post hooks, and the install hooks.
var deployScripts = []string{
	"preinstall", "install", "postinstall", "prepare", "prebuild", "build", "postbuild",
	"prestart", "start", "poststart", "migrate",
}

var reNpmRun = regexp.MustCompile(`\b(?:npm\s+run|yarn(?:\s+run)?|pnpm(?:\s+run)?|bun\s+run)\s+([\w:.-]+)`)

// deployTexts is the text of everything that says what a deploy builds and runs: whisk.yaml, the
// Dockerfile, a Procfile and the package.json scripts a deploy runs, following npm run. Pure.
func deployTexts(r Repo) []string {
	out := []string{string(r.ManifestSrc), r.Manifest.Migrate}
	for _, w := range r.Manifest.Workers {
		out = append(out, w.Command)
	}
	for _, name := range dockerfiles(r) {
		out = append(out, string(r.Read(name)))
	}
	out = append(out, string(r.Read("Procfile")))
	var pkg struct {
		Main    string            `json:"main"`
		Scripts map[string]string `json:"scripts"`
	}
	if src := r.Read("package.json"); src != nil && json.Unmarshal(src, &pkg) == nil {
		out = append(out, pkg.Main)
		queue := append([]string(nil), deployScripts...)
		for _, t := range out {
			for _, m := range reNpmRun.FindAllStringSubmatch(t, -1) {
				queue = append(queue, m[1])
			}
		}
		seen := map[string]bool{}
		for len(queue) > 0 {
			name := queue[0]
			queue = queue[1:]
			s, ok := pkg.Scripts[name]
			if seen[name] || !ok {
				continue
			}
			seen[name] = true
			out = append(out, s)
			for _, m := range reNpmRun.FindAllStringSubmatch(s, -1) {
				queue = append(queue, m[1])
			}
		}
	}
	return out
}

// topFolders is each top-level folder that holds a file, sorted. Pure.
func topFolders(files []File) []string {
	set := map[string]bool{}
	for _, f := range files {
		if i := strings.IndexByte(f.Path, '/'); i > 0 {
			set[f.Path[:i]] = true
		}
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// shippedByDockerfile reports whether a Dockerfile's build would carry a top-level folder: its
// COPY and ADD instructions from the build context name it, or the whole context, and
// .dockerignore does not leave it out. A Dockerfile that copies nothing from the context ships
// everything as far as doctor can tell. Pure.
func shippedByDockerfile(dockerfile, dockerignore []byte, dir string) bool {
	if dockerignore != nil {
		ign := gitignore.CompileIgnoreLines(strings.Split(string(dockerignore), "\n")...)
		if ign.MatchesPath(dir+"/") || ign.MatchesPath(dir) {
			return false
		}
	}
	if dockerfile == nil {
		return true
	}
	var sources []string
	for _, in := range instructionLines(dockerfile) {
		if in.Keyword != "COPY" && in.Keyword != "ADD" || slices.ContainsFunc(in.Flags, func(f string) bool { return strings.HasPrefix(f, "--from") }) {
			continue
		}
		words := in.Args
		if !in.Exec && len(words) == 1 {
			words = strings.Fields(words[0])
		}
		if len(words) < 2 {
			continue
		}
		sources = append(sources, words[:len(words)-1]...)
	}
	if len(sources) == 0 {
		return true
	}
	for _, s := range sources {
		s = strings.TrimPrefix(path.Clean(strings.TrimPrefix(s, "/")), "./")
		first, _, _ := strings.Cut(s, "/")
		if s == "." || s == "" || first == dir {
			return true
		}
		if ok, _ := path.Match(first, dir); ok {
			return true
		}
	}
	return false
}

// namedIn reports whether text names a top-level folder as a path: dev/stack.mjs, ./dev or
// "dev" followed by a separator. Pure.
func namedIn(text, dir string) bool {
	re := regexp.MustCompile(`(?:^|[\s"'=:(,\[{])(?:\./)?` + regexp.QuoteMeta(dir) + `(?:/|[\s"',)\]}]|$)`)
	return re.MatchString(text)
}

// reachedFrom reports whether code outside a top-level folder imports a file in it or names a
// path in it as a string (a folder loaded by a glob or a dynamic import). Pure.
func reachedFrom(files []File, set map[string]bool, dir string) bool {
	prefix := dir + "/"
	lit := regexp.MustCompile("[\"'`](?:\\./|\\.\\./)*" + regexp.QuoteMeta(dir) + `/`)
	goImport := regexp.MustCompile(`"[\w.\-/]+/` + regexp.QuoteMeta(dir) + `(?:/[\w.\-/]*)?"`)
	pyModule := regexp.MustCompile(`(?m)^\s*(?:from|import)\s+` + regexp.QuoteMeta(dir) + `\b|["']` + regexp.QuoteMeta(dir) + `\.\w`)
	for _, f := range files {
		lang := stack.LangOf(f.Path)
		if strings.HasPrefix(f.Path, prefix) || f.Content == nil || lang == "" || stack.IsTest(f.Path) || stack.IsVendored(f.Path) {
			continue
		}
		for _, p := range importedPaths(f, set) {
			if strings.HasPrefix(p, prefix) {
				return true
			}
		}
		src := blankComments(lang, f.Content)
		switch {
		case lit.Match(src):
			return true
		case lang == stack.GO && goImport.Match(src):
			return true
		case lang == stack.PY && pyModule.Match(src):
			return true
		}
	}
	return false
}

// undeployedFolders is each top-level folder whose code the deployed app does not run. Pure.
func undeployedFolders(r Repo) []string {
	var dockerfile []byte
	if names := dockerfiles(r); len(names) > 0 {
		dockerfile = r.Read(names[0])
	}
	var dockerignore []byte
	if dockerfile != nil {
		dockerignore = r.Read(".dockerignore")
	}
	texts := deployTexts(r)
	set := fileSet(r.Files)
	var out []string
	for _, d := range topFolders(r.Files) {
		if !shippedByDockerfile(dockerfile, dockerignore, d) {
			out = append(out, d)
			continue
		}
		if !devFolders[strings.ToLower(d)] {
			continue
		}
		named := slices.ContainsFunc(texts, func(t string) bool { return namedIn(t, d) })
		if !named && !reachedFrom(r.Files, set, d) {
			out = append(out, d)
		}
	}
	return out
}

// undeployed reports whether a path is in a folder the deployed app does not run. Pure.
func undeployed(p string, folders []string) bool {
	first, _, ok := strings.Cut(p, "/")
	return ok && slices.Contains(folders, first)
}

// Browser code.

var (
	reViteRoot   = regexp.MustCompile(`\broot\s*:\s*(?:(?:path\.)?(?:resolve|join)\([^)]*?)?["'](?:\./)?([\w\-./]+?)/?["']`)
	reModuleHTML = regexp.MustCompile(`(?i)<script[^>]+(?:type\s*=\s*["']module["']|src\s*=)`)
	reBrowserJS  = regexp.MustCompile(`(?m)^\s*["']use client["']|\bdocument\.\w|\bwindow\.\w|\blocalStorage\.|\buse(?:Effect|LayoutEffect|State)\s*\(`)
)

// bundlerConfig reports whether a file is a browser bundler's configuration. Pure.
func bundlerConfig(p string) bool {
	base := path.Base(p)
	for _, name := range []string{"vite.config.", "webpack.config.", "rollup.config.", "parcel.config.", "svelte.config."} {
		if strings.HasPrefix(base, name) {
			return true
		}
	}
	return false
}

// browserFolders is each folder that holds a browser bundle's source, whose code runs in the
// browser: a folder below the root with a bundler's configuration or an index.html that loads a
// script, and the folder a root bundler configuration names as its root. Pure.
func browserFolders(files []File) []string {
	var out []string
	add := func(d string) {
		d = strings.Trim(path.Clean(d), "/")
		if d != "" && d != "." && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	for _, f := range files {
		dir := path.Dir(f.Path)
		switch {
		case bundlerConfig(f.Path) && dir != ".":
			add(dir)
		case bundlerConfig(f.Path) && f.Content != nil:
			if m := reViteRoot.FindSubmatch(f.Content); m != nil {
				add(string(m[1]))
			}
		case path.Base(f.Path) == "index.html" && dir != "." && f.Content != nil && reModuleHTML.Match(f.Content):
			add(dir)
		}
	}
	return out
}

// browserFile reports whether a JavaScript or TypeScript file is written for the browser: it
// says "use client", or uses the page (document, window, localStorage) or React's state and
// effect hooks. Pure.
func browserFile(f File) bool {
	return stack.LangOf(f.Path) == stack.JS && reBrowserJS.Match(blankComments(stack.JS, f.Content))
}
