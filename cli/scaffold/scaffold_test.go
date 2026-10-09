package scaffold

import (
	"strings"
	"testing"

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
		src := Manifest("my-app", s)
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
