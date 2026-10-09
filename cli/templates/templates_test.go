package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/templates/source"
)

// The embedded copy must equal the sibling templates folder. When the sibling is absent (the
// published module on its own) there is nothing to compare against.
func TestEmbeddedMatchesSource(t *testing.T) {
	for _, name := range []string{"typescript", "python", "go"} {
		root := filepath.Join("..", "..", "templates", name)
		if _, err := os.Stat(root); err != nil {
			t.Skipf("%s not present; generated copy cannot be compared", root)
		}
		want, err := source.Read(root)
		if err != nil {
			t.Fatal(err)
		}
		got := files[name]
		for p, f := range want {
			g, ok := got[p]
			if !ok {
				t.Errorf("%s: %s missing from files_gen.go; run go generate ./templates", name, p)
				continue
			}
			if g.Content != f.Content || g.Mode != f.Mode {
				t.Errorf("%s: %s differs from files_gen.go; run go generate ./templates", name, p)
			}
		}
		for p := range got {
			if _, ok := want[p]; !ok {
				t.Errorf("%s: %s is in files_gen.go but not on disk; run go generate ./templates", name, p)
			}
		}
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	paths, err := Write(dir, "ts", "my-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 10 {
		t.Fatalf("only %d files written", len(paths))
	}
	m, err := os.ReadFile(filepath.Join(dir, "whisk.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "name: my-app\n"; !strings.Contains(string(m), want) {
		t.Errorf("manifest not renamed:\n%s", m)
	}
	if _, err := Write(dir, "ts", "my-app"); err == nil {
		t.Error("second write into the same directory should refuse")
	}
	if _, ok := Files("rust"); ok {
		t.Error("unknown template accepted")
	}
}

// A planted entry, a dangling symbolic link included, stops the write before anything is
// written, and nothing is created where a link points.
func TestWriteRefusesPlantedEntries(t *testing.T) {
	cases := []struct {
		name  string
		plant func(t *testing.T, dir, outside string)
	}{
		{"dangling link at a file", func(t *testing.T, dir, outside string) {
			must(t, os.Symlink(filepath.Join(outside, "pwned"), filepath.Join(dir, "whisk.yaml")))
		}},
		{"link as a parent directory", func(t *testing.T, dir, outside string) {
			paths := templatePaths(t)
			for _, p := range paths {
				if i := strings.Index(p, "/"); i > 0 {
					must(t, os.Symlink(outside, filepath.Join(dir, p[:i])))
					return
				}
			}
			t.Skip("the template has no nested file")
		}},
		{"regular file", func(t *testing.T, dir, outside string) {
			must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte("keep\n"), 0o644))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			c.plant(t, dir, outside)
			if _, err := Write(dir, "go", "crm"); err == nil {
				t.Fatal("Write succeeded over a planted entry")
			}
			if got, _ := os.ReadDir(outside); len(got) != 0 {
				t.Errorf("Write created %d entries outside the directory", len(got))
			}
		})
	}
}

func TestWriteIntoEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	paths, err := Write(dir, "go", "crm")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != len(templatePaths(t)) {
		t.Errorf("wrote %d files, want %d", len(paths), len(templatePaths(t)))
	}
	src, err := os.ReadFile(filepath.Join(dir, "whisk.yaml"))
	if err != nil || !strings.Contains(string(src), "name: crm") {
		t.Errorf("whisk.yaml = %q, %v", src, err)
	}
}

func templatePaths(t *testing.T) []string {
	tf, ok := Files("go")
	if !ok {
		t.Fatal("no go template")
	}
	out := make([]string, 0, len(tf))
	for p := range tf {
		out = append(out, p)
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
