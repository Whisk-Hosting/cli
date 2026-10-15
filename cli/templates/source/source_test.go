package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnored(t *testing.T) {
	goRules := parseIgnore("/server\n/whisk-go\n/bin/\n.whisk/dev/\n.env\n.env.*\n!.env.example\n")
	pyRules := parseIgnore("# the python template\n\n.venv/\n__pycache__/\n*.pyc\n")
	for _, tc := range []struct {
		name  string
		rules []rule
		path  string
		dir   bool
		want  bool
	}{
		{"a built binary at the root", goRules, "whisk-go", false, true},
		{"an anchored rule holds only at the root", goRules, "cmd/whisk-go", false, false},
		{"a directory rule", goRules, "bin", true, true},
		{"a directory rule does not name a file", goRules, "bin", false, false},
		{"a rule with a slash in it is anchored", goRules, ".whisk/dev", true, true},
		{"the parent of an ignored directory is not ignored", goRules, ".whisk", true, false},
		{"a glob", goRules, ".env.local", false, true},
		{"a negation wins when it comes last", goRules, ".env.example", false, false},
		{"an unanchored directory at any depth", pyRules, "app/__pycache__", true, true},
		{"an unanchored glob at any depth", pyRules, "app/whisk.pyc", false, true},
		{"source is not ignored", pyRules, "app/main.py", false, false},
		{"comments and blank lines are not rules", pyRules, "# the python template", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ignored(tc.rules, tc.path, tc.dir); got != tc.want {
				t.Errorf("ignored(%q, dir=%v) = %v, want %v", tc.path, tc.dir, got, tc.want)
			}
		})
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const elf = "\x7fELF\x02\x01\x01\x00\x00\x00binary"

// A binary the template builds in place is output, not source: its own .gitignore says so, and
// the embedded copy is what a fresh clone holds.
func TestReadLeavesOutWhatTheTemplateIgnores(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".gitignore", "/whisk-go\n.whisk/dev/\n")
	write(t, root, "main.go", "package main\r\n")
	write(t, root, "whisk-go", elf)
	write(t, root, ".whisk/dev/state.json", "{}")
	got, err := Read(root)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, ok := got["whisk-go"]; ok {
		t.Error("the ignored binary was read into the template")
	}
	if _, ok := got[".whisk/dev/state.json"]; ok {
		t.Error("the ignored dev state was read into the template")
	}
	if got["main.go"].Content != "package main\n" {
		t.Errorf("main.go = %q, want it read with line endings normalised", got["main.go"].Content)
	}
}

// A binary nobody ignored would be embedded in every whisk binary, so reading refuses it and
// names it rather than carrying it silently.
func TestReadRefusesABinaryNobodyIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\n")
	write(t, root, "tools/lint", elf)
	_, err := Read(root)
	if err == nil {
		t.Fatal("Read embedded a binary")
	}
	if !strings.Contains(err.Error(), "tools/lint") || !strings.Contains(err.Error(), ".gitignore") {
		t.Errorf("error %q should name the file and the way out", err)
	}
}
