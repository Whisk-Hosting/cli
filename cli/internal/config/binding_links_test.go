package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A repository that commits .whisk as a link cannot make whisk use write app.json elsewhere.
func TestSaveBindingRefusesALinkedFolder(t *testing.T) {
	repo, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, ".whisk")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := SaveBinding(repo, Binding{Org: "o", App: "a"}); err == nil {
		t.Fatal("SaveBinding wrote through a linked .whisk")
	}
	if _, err := os.Stat(filepath.Join(outside, "app.json")); err == nil {
		t.Fatal("app.json landed outside the repository")
	}
	if err := SaveBinding(t.TempDir(), Binding{Org: "o", App: "a"}); err != nil {
		t.Fatal(err)
	}
}
