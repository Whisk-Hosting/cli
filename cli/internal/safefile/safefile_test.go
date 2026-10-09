package safefile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInside(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	links := runtime.GOOS != "windows"
	if links {
		must(t, os.Symlink(filepath.Join(outside, "profile"), filepath.Join(root, "x.graph.yaml")))
		must(t, os.Symlink(outside, filepath.Join(root, "dir")))
	}
	cases := []struct {
		rel     string
		ok      bool
		symlink bool
	}{
		{"whisk.yaml", true, false},
		{"real/new/a.graph.yaml", true, false},
		{"./real/../b.yaml", true, false},
		{"../escape.yaml", false, false},
		{"real/../../escape.yaml", false, false},
		{"/etc/passwd", false, false},
		{".", false, false},
		{"x.graph.yaml", false, true},
		{"dir/a.yaml", false, true},
	}
	for _, c := range cases {
		if c.symlink && !links {
			continue
		}
		got, err := Inside(root, c.rel)
		if (err == nil) != c.ok {
			t.Errorf("Inside(%q) = %q, %v; want ok=%v", c.rel, got, err, c.ok)
		}
		if err == nil && !strings.HasPrefix(got, root) {
			t.Errorf("Inside(%q) = %q, outside the root", c.rel, got)
		}
	}
}

func TestWriteRefusesLinkAndLeavesTargetAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, ".bashrc")
	must(t, os.WriteFile(victim, []byte("original"), 0o644))
	must(t, os.Symlink(victim, filepath.Join(root, ".gitignore")))
	if err := Write(root, ".gitignore", []byte("clobbered"), 0o644); err == nil {
		t.Fatal("Write followed a symbolic link out of the root")
	}
	if b, _ := os.ReadFile(victim); string(b) != "original" {
		t.Fatalf("the link target changed to %q", b)
	}
}

func TestWriteFileTightensMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	p := filepath.Join(t.TempDir(), "credentials.json")
	must(t, os.WriteFile(p, []byte("{}"), 0o644))
	must(t, WriteFile(p, []byte(`{"a":1}`), 0o600))
	info, err := os.Stat(p)
	must(t, err)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", info.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); string(b) != `{"a":1}` {
		t.Fatalf("content %q", b)
	}
	if entries, _ := os.ReadDir(filepath.Dir(p)); len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
