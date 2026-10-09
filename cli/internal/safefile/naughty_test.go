package safefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// A naughty relative path is refused or lands inside the root, and Write puts the file exactly
// where Inside said, never beside or above the root.
func TestNaughtyInside(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, s := range naughty.Strings() {
		for _, rel := range []string{s, "a/" + s, s + "/b", "link/" + s, "../" + s} {
			p, err := Inside(root, rel)
			if err != nil {
				continue
			}
			back, rerr := filepath.Rel(root, p)
			if rerr != nil || back == "." || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
				t.Errorf("Inside(root, %q) = %q, outside the root", rel, p)
				continue
			}
			if strings.HasPrefix(back, "link") && (back == "link" || strings.HasPrefix(back, "link"+string(filepath.Separator))) {
				t.Errorf("Inside(root, %q) = %q passes through a symbolic link", rel, p)
			}
			if len(rel) > 255 {
				continue // longer than a file name may be; Write would only fail
			}
			if err := Write(root, rel, []byte(s), 0o600); err != nil {
				continue
			}
			if got, err := os.ReadFile(p); err != nil || string(got) != s {
				t.Errorf("Write(root, %q) did not land at %q: %v", rel, p, err)
			}
		}
	}
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if e.Name() != "repo" {
			t.Errorf("a write escaped the root: %s", e.Name())
		}
	}
}
