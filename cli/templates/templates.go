// Package templates carries the three starter apps and the shop from ../../templates inside the binary so
// whisk init --template works anywhere. The files_*_gen.go files are produced by `go generate`
// from the sibling folder, each adding part of a template; the test fails when the two drift.
package templates

//go:generate go run ./gen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/cli/scaffold"
)

// File is one template file.
type File struct {
	Mode    os.FileMode
	Content string
}

// files holds each template's files by folder name, filled by the generated files' init.
var files = map[string]map[string]File{}

// add puts part of a template into files.
func add(name string, set map[string]File) {
	if files[name] == nil {
		files[name] = map[string]File{}
	}
	for p, f := range set {
		files[name][p] = f
	}
}

// Names maps the --template flag values to folder names.
var Names = map[string]string{"ts": "typescript", "typescript": "typescript", "py": "python", "python": "python", "go": "go", "shop": "shop"}

// Files returns a template's files by relative slash path, or false for an unknown name.
func Files(name string) (map[string]File, bool) {
	folder, ok := Names[name]
	if !ok {
		return nil, false
	}
	f, ok := files[folder]
	return f, ok
}

// List returns the template folder names.
func List() []string {
	out := make([]string, 0, len(files))
	for k := range files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Write copies a template into dir, replacing the manifest name with slug. It refuses to
// overwrite anything already there, a dangling symbolic link included, and refuses a path that
// passes through a symbolic link, so it is only for an empty directory and cannot be pointed
// at a file elsewhere.
func Write(dir, name, slug string) ([]string, error) {
	tf, ok := Files(name)
	if !ok {
		return nil, fmt.Errorf("unknown template %q; use ts, py, go or shop", name)
	}
	paths := make([]string, 0, len(tf))
	for p := range tf {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		target, err := safefile.Inside(dir, p)
		if err != nil {
			return nil, err
		}
		if _, err := os.Lstat(target); err == nil {
			return nil, fmt.Errorf("%s exists; a template is only written into an empty directory", p)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	for _, p := range paths {
		f := tf[p]
		content := f.Content
		if p == "whisk.yaml" {
			content = RenameManifest(content, slug)
		}
		if err := safefile.Write(dir, p, []byte(content), f.Mode.Perm()); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// RenameManifest sets the top-level name in a manifest's text without touching anything else.
func RenameManifest(src, slug string) string {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "name:") {
			lines[i] = "name: " + scaffold.ManifestName(slug)
			break
		}
	}
	return strings.Join(lines, "\n")
}
