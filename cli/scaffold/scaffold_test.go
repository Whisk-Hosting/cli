package scaffold

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/hostfiles"
	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/manifest"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{"My App": "my-app", "crm_v2": "crm-v2", "  Acme CRM!! ": "acme-crm", "ok": "", "a-very-long-directory-name-that-goes-on-and-on": "a-very-long-directory-name-that-goes-on"}
	for in, want := range cases {
		got, err := Slug(in)
		if want == "" {
			if err == nil {
				t.Errorf("%q: expected an error, got %q", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q: got %q (%v), want %q", in, got, err, want)
		}
	}
}

func TestManifestParses(t *testing.T) {
	for _, s := range []stack.Stack{{}, {Primary: stack.JS, Migrate: "npx prisma migrate deploy"}, {Primary: stack.PY, Migrate: "alembic upgrade head"}} {
		src := Manifest("my-app", s, hostfiles.Found{})
		m, err := manifest.Parse([]byte(src))
		if err != nil {
			t.Fatalf("generated manifest does not parse: %v\n%s", err, src)
		}
		if m.Name != "my-app" || m.Health.Path != "/health" || m.Database != "app" {
			t.Errorf("unexpected defaults: %+v", m)
		}
		if s.Migrate != "" && m.Migrate != s.Migrate {
			t.Errorf("migrate %q not carried into the manifest", s.Migrate)
		}
	}
}

func TestManifestFromHostFiles(t *testing.T) {
	h := hostfiles.Found{
		Files:      []string{"render.yaml"},
		Migrate:    "npm run migrate",
		Health:     "/healthz",
		Dockerfile: "./docker/api.Dockerfile",
		Secrets:    []string{"JWT_SECRET", "STRIPE_SECRET_KEY"},
		Env:        map[string]string{"APP_ENV": "production", "QUOTE": `say "hi"`},
		KV:         true,
		Notes:      []string{strings.Repeat("A long note that has to wrap across more than one comment line. ", 3)},
	}
	src := Manifest("my-app", stack.Stack{Migrate: "npx prisma migrate deploy"}, h)
	m, err := manifest.Parse([]byte(src))
	if err != nil {
		t.Fatalf("generated manifest does not parse: %v\n%s", err, src)
	}
	if m.Migrate != "npm run migrate" || m.Health.Path != "/healthz" || m.Build.Dockerfile != "docker/api.Dockerfile" || !m.KV {
		t.Errorf("not carried across: %+v\n%s", m, src)
	}
	if strings.Join(m.Secrets, ",") != "JWT_SECRET,STRIPE_SECRET_KEY" || m.Env["QUOTE"] != `say "hi"` || m.Env["APP_ENV"] != "production" {
		t.Errorf("secrets %v env %v", m.Secrets, m.Env)
	}
	if !strings.Contains(src, "# Written from render.yaml.") || !strings.Contains(src, "# - A long note") || !strings.Contains(src, "\n#   ") {
		t.Errorf("notes not written as wrapped comments:\n%s", src)
	}
	for _, l := range strings.Split(src, "\n") {
		if len(l) > 100 {
			t.Errorf("line over 100 characters: %q", l)
		}
	}
}

func TestManifestWithWorkers(t *testing.T) {
	files := map[string]string{
		"fly.toml":     "[deploy]\nrelease_command = \"bin/rails db:migrate\"\n[processes]\napp = \"bin/rails server\"\njobs = \"bundle exec good_job start\"\nmail = \"bundle exec sidekiq -q mail\"\n",
		"Gemfile":      "gem \"rails\"\n",
		"package.json": "{}",
	}
	h := hostfiles.Read([]string{"fly.toml", "Gemfile", "package.json"}, func(p string) []byte { return []byte(files[p]) })
	src := Manifest("legacy", stack.Stack{}, h)
	m, err := manifest.Parse([]byte(src))
	if err != nil {
		t.Fatalf("generated manifest does not parse: %v\n%s", err, src)
	}
	want := []manifest.Worker{
		{Name: "jobs", Command: "bundle exec good_job start", Database: manifest.WorkerDirect},
		{Name: "mail", Command: "bundle exec sidekiq -q mail", Database: manifest.WorkerPooled},
	}
	if !m.Promoted || !m.KV || len(m.Workers) != len(want) {
		t.Fatalf("got promoted %v kv %v workers %+v\n%s", m.Promoted, m.KV, m.Workers, src)
	}
	for i, w := range want {
		if m.Workers[i].Name != w.Name || m.Workers[i].Command != w.Command || m.Workers[i].Database != w.Database {
			t.Errorf("worker %d: got %+v, want %+v", i, m.Workers[i], w)
		}
	}
}

func TestMergeGitignore(t *testing.T) {
	out, added := MergeGitignore("node_modules\n.env\n")
	if len(added) != 3 || !strings.Contains(out, ".whisk/dev/") || !strings.Contains(out, "!.env.example") || strings.Count(out, ".env\n") != 1 {
		t.Errorf("merge: added %v\n%s", added, out)
	}
	same, none := MergeGitignore(out)
	if none != nil || same != out {
		t.Error("second merge changed the file")
	}
	fresh, _ := MergeGitignore("")
	if !strings.HasPrefix(fresh, ".whisk/dev/\n") {
		t.Errorf("fresh file:\n%s", fresh)
	}
}
