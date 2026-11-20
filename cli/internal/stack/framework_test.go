package stack

import "testing"

func TestJSFramework(t *testing.T) {
	cases := []struct{ name, pkg, want string }{
		{"hono", `{"dependencies":{"hono":"4"}}`, "hono"},
		{"next beside hono's types is next", `{"dependencies":{"next":"16"},"devDependencies":{"hono":"4"}}`, "next"},
		{"express", `{"dependencies":{"express":"5"}}`, "express"},
		{"none", `{}`, ""},
	}
	for _, c := range cases {
		if got, _ := jsFramework([]byte(c.pkg)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPyFramework(t *testing.T) {
	cases := []struct{ name, pyproject, requirements, want string }{
		{"project dependencies", "[project]\ndependencies = [\"FastAPI>=0.115\", \"uvicorn[standard]\"]\n", "", "fastapi"},
		{"poetry's table", "[tool.poetry.dependencies]\npython = \"^3.13\"\nDjango = \"^5\"\n", "", "django"},
		{"a dependency group", "[dependency-groups]\nweb = [\"flask\", {include-group = \"x\"}]\n", "", "flask"},
		{"requirements.txt", "", "# the web server\nFlask==3.1  # pinned\n-r base.txt\n", "flask"},
		{"a word in the description is no dependency", "[project]\ndescription = \"moved off fastapi\"\ndependencies = [\"django\"]\n", "", "django"},
		{"another package's name is no framework", "", "flask-cors==5\ndjango-environ\n# fastapi\n", ""},
		{"a comment is no dependency", "[project]\n# dependencies = [\"fastapi\"]\ndependencies = []\n", "", ""},
		{"a pyproject that does not parse leaves requirements.txt", "[project\n", "django\n", "django"},
	}
	for _, c := range cases {
		if got, _ := pyFramework(nil, []byte(c.pyproject), []byte(c.requirements)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGoFramework(t *testing.T) {
	cases := []struct{ name, gomod, want string }{
		{"chi at a major version", "module x\n\ngo 1.26\n\nrequire github.com/go-chi/chi/v5 v5.2.0\n", "chi"},
		{"a block", "module x\n\nrequire (\n\tgithub.com/labstack/echo/v4 v4.13.0\n)\n", "echo"},
		{"a router a dependency brought in is not the app's", "module x\n\nrequire (\n\tgithub.com/gin-gonic/gin v1.10.0 // indirect\n)\n", "net/http"},
		{"a comment names no router", "module x\n\n// was github.com/gofiber/fiber\n", "net/http"},
		{"a module with a router's name in its own is another module", "module x\n\nrequire github.com/go-chi/chimiddleware v1.0.0\n", "net/http"},
		{"none", "module x\n", "net/http"},
		{"not a go.mod", "{", "net/http"},
	}
	for _, c := range cases {
		if got := goFramework([]byte(c.gomod)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
