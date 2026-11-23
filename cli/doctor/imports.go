package doctor

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// The app's own imports, read so a rule can tell what code runs with what: the files a
// migration pulls in run in the migrate step (W105), and a folder nothing imports is not part of
// the deployed app (deployed.go). Only imports of the app's own files are resolved; packages are
// not followed.

var (
	reJSImport    = regexp.MustCompile(`(?:\bimport\s*(?:type\s+)?(?:[\w*{}\s,$]+?\s*from\s*)?|\bexport\s*(?:type\s+)?[\w*{}\s,$]+?\s*from\s*|\brequire\s*\(\s*|\bimport\s*\(\s*)["']([^"'\n]+)["']`)
	reOwnPyFrom   = regexp.MustCompile(`(?m)^\s*from\s+(\.*[\w.]*)\s+import\s+\(?\s*([\w,\s]+)`)
	reOwnPyImport = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+(?:\s*,\s*[\w.]+)*)`)
)

// jsExts are the extensions an extensionless or .js import may resolve to, in the order a
// bundler or TypeScript tries them.
var jsExts = []string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}

// fileSet is the repository's paths, for resolving imports. Pure.
func fileSet(files []File) map[string]bool {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[f.Path] = true
	}
	return set
}

// importedPaths is every file of the repository that f imports directly, as the language
// resolves it: relative JavaScript and TypeScript imports (with the extension or index file left
// out, or .js written for a .ts file), @/ and ~/ for src/, a bare path that is a file of the
// repository, and Python modules, relative or from the root or src/. Pure.
func importedPaths(f File, set map[string]bool) []string {
	src := blankComments(stack.LangOf(f.Path), f.Content)
	var out []string
	add := func(p string) {
		if p != "" && p != f.Path && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	switch stack.LangOf(f.Path) {
	case stack.JS:
		for _, m := range reJSImport.FindAllSubmatch(src, -1) {
			add(resolveJS(f.Path, string(m[1]), set))
		}
	case stack.GO:
		// Go imports packages, not files; reachedFrom matches a folder's import path instead.
	case stack.PY:
		for _, m := range reOwnPyFrom.FindAllSubmatch(src, -1) {
			mod := string(m[1])
			add(resolvePy(f.Path, mod, set))
			for _, name := range strings.Split(string(m[2]), ",") {
				if name = strings.TrimSpace(name); name != "" {
					sub := mod + "." + name
					if strings.HasSuffix(mod, ".") {
						sub = mod + name
					}
					add(resolvePy(f.Path, sub, set))
				}
			}
		}
		for _, m := range reOwnPyImport.FindAllSubmatch(src, -1) {
			for _, mod := range strings.Split(string(m[1]), ",") {
				add(resolvePy(f.Path, strings.TrimSpace(mod), set))
			}
		}
	}
	return out
}

// resolveJS is the repository file a JavaScript or TypeScript import specifier names, or "".
// Pure.
func resolveJS(from, spec string, set map[string]bool) string {
	var base string
	switch {
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"), spec == ".", spec == "..":
		base = path.Join(path.Dir(from), spec)
	case strings.HasPrefix(spec, "@/"), strings.HasPrefix(spec, "~/"):
		base = path.Join("src", spec[2:])
	case strings.HasPrefix(spec, "/"):
		base = strings.TrimPrefix(path.Clean(spec), "/")
	default:
		base = path.Clean(spec) // a bare path a baseUrl resolves; a package resolves to nothing here
	}
	if strings.HasPrefix(base, "../") || base == ".." {
		return ""
	}
	candidates := []string{base}
	stem := base
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs"} {
		if strings.HasSuffix(base, ext) {
			stem = strings.TrimSuffix(base, ext)
		}
	}
	for _, ext := range jsExts {
		candidates = append(candidates, stem+ext)
	}
	for _, ext := range jsExts {
		candidates = append(candidates, base+"/index"+ext)
	}
	for _, c := range candidates {
		if set[c] {
			return c
		}
	}
	return ""
}

// resolvePy is the repository file a Python module names, or "": relative to the importing
// file for leading dots, otherwise from the root or from src/. Pure.
func resolvePy(from, mod string, set map[string]bool) string {
	if mod == "" {
		return ""
	}
	var roots []string
	dots := len(mod) - len(strings.TrimLeft(mod, "."))
	if dots > 0 {
		dir := path.Dir(from)
		for i := 1; i < dots; i++ {
			dir = path.Dir(dir)
		}
		roots = []string{dir}
		mod = mod[dots:]
	} else {
		roots = []string{".", "src"}
	}
	rel := strings.ReplaceAll(mod, ".", "/")
	for _, root := range roots {
		p := path.Join(root, rel)
		if rel == "" {
			p = root
		}
		for _, c := range []string{p + ".py", p + "/__init__.py"} {
			if set[c] {
				return c
			}
		}
	}
	return ""
}

// importClosure is every file reachable through imports from the starting files, the starting
// files included. Pure.
func importClosure(files []File, start []string) map[string]bool {
	byPath := make(map[string]File, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}
	set := fileSet(files)
	seen := map[string]bool{}
	queue := append([]string(nil), start...)
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p] {
			continue
		}
		seen[p] = true
		f, ok := byPath[p]
		if !ok || f.Content == nil {
			continue
		}
		queue = append(queue, importedPaths(f, set)...)
	}
	return seen
}
