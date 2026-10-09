// Package stack detects what kind of app a directory holds, from its manifest files, the way
// whisk init and whisk doctor both need to.
package stack

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
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
	switch {
	case has("hono"):
		framework = "hono"
	case has("next"):
		framework = "next"
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
	deps := strings.ToLower(string(pyproject) + "\n" + string(requirements))
	switch {
	case strings.Contains(deps, "fastapi"):
		framework = "fastapi"
	case strings.Contains(deps, "django"):
		framework = "django"
	case strings.Contains(deps, "flask"):
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

func goFramework(gomod []byte) string {
	src := string(gomod)
	switch {
	case strings.Contains(src, "github.com/go-chi/chi"):
		return "chi"
	case strings.Contains(src, "github.com/gin-gonic/gin"):
		return "gin"
	case strings.Contains(src, "github.com/labstack/echo"):
		return "echo"
	case strings.Contains(src, "github.com/gofiber/fiber"):
		return "fiber"
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
