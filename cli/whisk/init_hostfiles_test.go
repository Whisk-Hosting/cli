package whisk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/contract/manifest"
)

// An app moving from another host keeps what its Procfile and app.json said, and init answers
// which files it read and what is left to do.
func TestInitFromHostFiles(t *testing.T) {
	store := &memStore{m: map[string]config.Credential{}}
	dir := filepath.Join(t.TempDir(), "legacy-api")
	must(t, os.MkdirAll(dir, 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"hono":"4"}}`), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "Procfile"), []byte("web: node dist/server.js\nrelease: node dist/migrate.js\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"env":{"STRIPE_SECRET_KEY":{"required":true},"LOG_LEVEL":{"value":"info"}}}`), 0o644))
	r := runCLI(t, dir, nil, store, "init", "--no-create", "--json")
	if r.Code != 0 {
		t.Fatalf("init: %+v", r)
	}
	if from := anyStrings(r.JSON["from"]); strings.Join(from, ",") != "Procfile,app.json" {
		t.Fatalf("from: %v", r.JSON["from"])
	}
	if todo := anyStrings(r.JSON["todo"]); len(todo) != 1 || !strings.Contains(todo[0], "node dist/server.js") {
		t.Fatalf("todo: %v", r.JSON["todo"])
	}
	src, err := os.ReadFile(filepath.Join(dir, "whisk.yaml"))
	must(t, err)
	m, err := manifest.Parse(src)
	if err != nil {
		t.Fatalf("whisk.yaml does not parse: %v\n%s", err, src)
	}
	if m.Migrate != "node dist/migrate.js" || strings.Join(m.Secrets, ",") != "STRIPE_SECRET_KEY" || m.Env["LOG_LEVEL"] != "info" {
		t.Fatalf("not carried across:\n%s", src)
	}
}
