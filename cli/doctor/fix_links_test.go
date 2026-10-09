package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A repository can name a graph that is a link to the user's shell profile, or one that
// climbs out of it; --fix must refuse both and write nothing at all.
func TestApplyEditsRefusesLinksAndEscapes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	root, home := t.TempDir(), t.TempDir()
	profile := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(profile, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(profile, filepath.Join(root, "x.graph.yaml")); err != nil {
		t.Fatal(err)
	}
	for _, edits := range [][]Edit{
		{{Path: "a.graph.yaml", Content: []byte("ok")}, {Path: "x.graph.yaml", Content: []byte("steps: []")}},
		{{Path: "../escape.graph.yaml", Content: []byte("steps: []")}},
	} {
		if err := applyEdits(root, edits); err == nil {
			t.Errorf("applyEdits(%v) wrote through a link or outside the repository", edits[len(edits)-1].Path)
		}
	}
	if b, _ := os.ReadFile(profile); string(b) != "original" {
		t.Fatalf("the profile became %q", b)
	}
	if _, err := os.Stat(filepath.Join(root, "a.graph.yaml")); err == nil {
		t.Fatal("a file was written before the refusal")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.graph.yaml")); err == nil {
		t.Fatal("a file was written outside the repository")
	}
}
