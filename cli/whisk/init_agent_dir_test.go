package whisk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

// A folder where an agent works (CLAUDE.md, .claude/) takes a template; one with code is
// refused, naming the files, and the fix offers whisk init without --template.
func TestInitTemplateBesideAgentFiles(t *testing.T) {
	store := &memStore{m: map[string]config.Credential{}}
	dir := filepath.Join(t.TempDir(), "my-crm")
	must(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# notes\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("{}"), 0o644))
	r := runCLI(t, dir, nil, store, "init", "--template", "go", "--no-create", "--json")
	if r.Code != 0 {
		t.Fatalf("init beside agent files: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("template not written: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); string(b) != "# notes\n" {
		t.Fatalf("CLAUDE.md changed: %q", b)
	}

	code := filepath.Join(t.TempDir(), "legacy")
	must(t, os.MkdirAll(code, 0o755))
	must(t, os.WriteFile(filepath.Join(code, "package.json"), []byte("{}"), 0o644))
	must(t, os.WriteFile(filepath.Join(code, "CLAUDE.md"), []byte("x"), 0o644))
	r = runCLI(t, code, nil, store, "init", "--template", "ts", "--no-create", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 1 || e["code"] != "INVALID_REQUEST" || !strings.Contains(e["message"].(string), "package.json") || !strings.Contains(e["fix"].(string), "without --template") {
		t.Fatalf("init into code: %+v", r)
	}
	if files := anyStrings(e["details"].(map[string]any)["files"]); strings.Join(files, ",") != "package.json" {
		t.Fatalf("files: %v", files)
	}
}

func TestNamesList(t *testing.T) {
	if got := namesList([]string{"a", "b"}); got != "a, b" {
		t.Fatal(got)
	}
	if got := namesList([]string{"a", "b", "c", "d", "e", "f", "g"}); got != "a, b, c, d, e and 2 more" {
		t.Fatal(got)
	}
}
