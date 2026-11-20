// Package stack detects what kind of app a directory holds, from its manifest files, the way
// whisk init and whisk doctor both need to.
package stack

import (
	"encoding/json"
	"path"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// Lang is a language doctor understands.
type Lang string

const (
	JS Lang = "js" // TypeScript and JavaScript
	PY Lang = "py"
	GO Lang = "go"
)

// Stack is what was detected. Langs is every language present, Primary the one whisk init
// scaffolds for.
type Stack struct {
	Langs         []Lang
	Primary       Lang
	Manager       string // npm | pnpm | bun | yarn | uv | poetry | pdm | pip | gomod
	Lockfile      string // the lockfile that exists, if any
	HasDockerfile bool
	Framework     string // hono | express | fastify | next | fastapi | flask | django | chi | net/http | ""
	Migrate       string // a migrate command guessed from the dependencies, or ""
}

// Detect looks at the file list (paths relative to the root, slash separated) and the content
// of package.json and pyproject.toml when given.
func Detect(files []string, read func(string) []byte) Stack {
	set := map[string]bool{}
	for _, f := range files {
		set[f] = true
	}
	var s Stack
	if set["package.json"] {
		s.Langs = append(s.Langs, JS)
		s.Manager, s.Lockfile = firstOf(set, [][2]string{{"pnpm", "pnpm-lock.yaml"}, {"bun", "bun.lock"}, {"bun", "bun.lockb"}, {"yarn", "yarn.lock"}, {"npm", "package-lock.json"}}, "npm")
		s.Framework, s.Migrate = jsFramework(read("package.json"))
	}
	if set["pyproject.toml"] || set["requirements.txt"] {
		s.Langs = append(s.Langs, PY)
		s.Manager, s.Lockfile = firstOf(set, [][2]string{{"uv", "uv.lock"}, {"poetry", "poetry.lock"}, {"pdm", "pdm.lock"}, {"pip", "requirements.txt"}}, "uv")
		if !set["pyproject.toml"] {
			s.Manager = "pip"
		}
		s.Framework, s.Migrate = pyFramework(set, read("pyproject.toml"), read("requirements.txt"))
	}
	if set["go.mod"] {
		s.Langs = append(s.Langs, GO)
		s.Manager = "gomod"
		if set["go.sum"] {
			s.Lockfile = "go.sum"
		}
		s.Framework = goFramework(read("go.mod"))
	}
	s.HasDockerfile = set["Dockerfile"]
	if len(s.Langs) > 0 {
		s.Primary = s.Langs[0]
	}
	return s
}

func firstOf(set map[string]bool, pairs [][2]string, fallback string) (manager, lockfile string) {
	for _, p := range pairs {
		if set[p[1]] {
			return p[0], p[1]
		}
	}
	return fallback, ""
}

func jsFramework(pkg []byte) (framework, migrate string) {
	var p struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Scripts         map[string]string `json:"scripts"`
	}
	_ = json.Unmarshal(pkg, &p)
	has := func(name string) bool {
		_, a := p.Dependencies[name]
		_, b := p.DevDependencies[name]
		return a || b
	}
	// Next.js is the server whenever it is there, even beside a router used inside it.
	switch {
	case has("next"):
		framework = "next"
	case has("hono"):
		framework = "hono"
	case has("fastify"):
		framework = "fastify"
	case has("express"):
		framework = "express"
	}
	switch {
	case p.Scripts["migrate"] != "":
		migrate = "npm run migrate"
	case has("prisma") || has("@prisma/client"):
		migrate = "npx prisma migrate deploy"
	case has("drizzle-kit"):
		migrate = "npx drizzle-kit migrate"
	}
	return framework, migrate
}

func pyFramework(set map[string]bool, pyproject, requirements []byte) (framework, migrate string) {
	deps := pyDependencies(pyproject, requirements)
	switch {
	case deps["fastapi"]:
		framework = "fastapi"
	case deps["django"]:
		framework = "django"
	case deps["flask"]:
		framework = "flask"
	}
	switch {
	case set["alembic.ini"]:
		migrate = "alembic upgrade head"
	case set["manage.py"]:
		migrate = "python manage.py migrate"
	}
	return framework, migrate
}

// pyDependencies is the packages a Python app says it depends on, by their normalised names
// (PEP 503: lower case, runs of -, _ and . as one -): pyproject.toml's project.dependencies,
// its optional and group dependencies and Poetry's table, and each line of requirements.txt.
// A package only named in a comment, a description or another package's name is not one. Pure.
func pyDependencies(pyproject, requirements []byte) map[string]bool {
	out := map[string]bool{}
	add := func(spec string) {
		if name := pyName(spec); name != "" {
			out[name] = true
		}
	}
	var doc struct {
		Project struct {
			Dependencies         []string            `toml:"dependencies"`
			OptionalDependencies map[string][]string `toml:"optional-dependencies"`
		} `toml:"project"`
		DependencyGroups map[string][]any `toml:"dependency-groups"`
		Tool             struct {
			Poetry struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if toml.Unmarshal(pyproject, &doc) == nil {
		for _, d := range doc.Project.Dependencies {
			add(d)
		}
		for _, group := range doc.Project.OptionalDependencies {
			for _, d := range group {
				add(d)
			}
		}
		for _, group := range doc.DependencyGroups {
			for _, d := range group {
				if spec, ok := d.(string); ok {
					add(spec)
				}
			}
		}
		for name := range doc.Tool.Poetry.Dependencies {
			add(name)
		}
	}
	for _, line := range strings.Split(string(requirements), "\n") {
		line, _, _ = strings.Cut(line, "#")
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "-") {
			add(line)
		}
	}
	return out
}

// pyName is the normalised name a requirement starts with ("Django>=5" is django,
// "uvicorn[standard]" is uvicorn), "" when it starts with none. Pure.
func pyName(spec string) string {
	spec = strings.TrimSpace(spec)
	end := strings.IndexFunc(spec, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	})
	if end >= 0 {
		spec = spec[:end]
	}
	name := strings.Join(strings.FieldsFunc(strings.ToLower(spec), func(r rune) bool { return r == '-' || r == '_' || r == '.' }), "-")
	return name
}

// goFramework is the router a Go app requires itself: a module go.mod requires directly, at any
// major version. One a dependency brought in (// indirect) is not the app's router. Pure.
func goFramework(gomod []byte) string {
	f, err := modfile.ParseLax("go.mod", gomod, nil)
	if err != nil {
		return "net/http"
	}
	direct := map[string]bool{}
	for _, r := range f.Require {
		if !r.Indirect {
			prefix, _, _ := module.SplitPathVersion(r.Mod.Path)
			direct[prefix] = true
		}
	}
	for _, known := range []struct{ path, name string }{
		{"github.com/go-chi/chi", "chi"},
		{"github.com/gin-gonic/gin", "gin"},
		{"github.com/labstack/echo", "echo"},
		{"github.com/gofiber/fiber", "fiber"},
	} {
		if direct[known.path] {
			return known.name
		}
	}
	return "net/http"
}

// LangOf classifies a file by extension; "" when it is not code doctor reads.
func LangOf(p string) Lang {
	switch strings.ToLower(path.Ext(p)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts":
		return JS
	case ".py":
		return PY
	case ".go":
		return GO
	}
	return ""
}

// IsTest reports files that hold tests, which doctor leaves out of code reads.
func IsTest(p string) bool {
	base := path.Base(p)
	switch {
	case strings.HasSuffix(base, "_test.go"), strings.HasPrefix(base, "test_"),
		strings.Contains(base, ".test."), strings.Contains(base, ".spec."):
		return true
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if seg == "test" || seg == "tests" || seg == "__tests__" || seg == "spec" || seg == "fuzz" {
			return true
		}
	}
	return false
}

// IsVendored reports directories doctor never reads as the app's own code.
func IsVendored(p string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		switch seg {
		case "node_modules", ".venv", "venv", "vendor", "dist", "build", ".next", "__pycache__", ".whisk", ".git", "site-packages":
			return true
		}
	}
	return false
}

// Names returns the language names for messages.
func Names(langs []Lang) string {
	out := make([]string, 0, len(langs))
	for _, l := range langs {
		out = append(out, string(l))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
