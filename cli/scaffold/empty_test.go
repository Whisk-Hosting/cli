package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnorable(t *testing.T) {
	for name, want := range map[string]bool{
		".git": true, ".claude": true, ".gitignore": true, ".cursor": true, "CLAUDE.md": true, "AGENTS.md": true, "GEMINI.md": true,
		"README.md": false, "package.json": false, "src": false, "claude.md": false, "main.go": false,
	} {
		if Ignorable(name) != want {
			t.Errorf("%s: want %v", name, want)
		}
	}
}

func TestIsEmptyDir(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		empty bool
		code  string
	}{
		{"missing", nil, true, ""},
		{"nothing", []string{}, true, ""},
		{"where an agent works", []string{".git/HEAD", ".claude/settings.json", "CLAUDE.md", "AGENTS.md"}, true, ""},
		{"code", []string{"CLAUDE.md", "package.json", "src/index.ts"}, false, "package.json,src"},
	}
	for _, c := range cases {
		dir := filepath.Join(t.TempDir(), "app")
		if c.files != nil {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		for _, f := range c.files {
			p := filepath.Join(dir, filepath.FromSlash(f))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		empty, err := IsEmptyDir(dir)
		code, _ := CodeFiles(dir)
		if err != nil || empty != c.empty || strings.Join(code, ",") != c.code {
			t.Errorf("%s: empty %v code %v err %v", c.name, empty, code, err)
		}
	}
}
