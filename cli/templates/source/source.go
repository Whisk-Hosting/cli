// Package source reads a template folder from disk the way the generator and its test do:
// every file a fresh clone would hold, with its mode. What a clone holds is what the template's
// own .gitignore leaves in, over a floor of output and secrets no template ever tracks. A file
// that is not text and that nothing ignores is refused rather than carried, because the embedded
// copy ships inside every whisk binary.
package source

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// File mirrors templates.File without importing it (the generator runs before it compiles).
type File struct {
	Mode    fs.FileMode
	Content string
}

var skipDirs = map[string]bool{"node_modules": true, ".venv": true, "venv": true, "dist": true, "__pycache__": true, ".whisk": true, "bin": true, ".git": true, "coverage": true}

func skipFile(base string) bool {
	switch {
	case base == ".DS_Store", strings.HasSuffix(base, ".pyc"), strings.HasSuffix(base, ".test.out"):
		return true
	case strings.HasPrefix(base, ".env") && base != ".env.example":
		return true
	}
	return false
}

// rule is one line of a .gitignore, in the shapes the templates use and with git's meaning for
// them: a leading ! re-includes, a trailing / names only directories, and a slash anywhere
// before the end anchors the pattern to the template's root instead of matching at any depth.
type rule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
}

func parseIgnore(text string) []rule {
	var rules []rule
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := rule{}
		if strings.HasPrefix(line, "!") {
			r.negate, line = true, line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly, line = true, strings.TrimSuffix(line, "/")
		}
		r.anchored = strings.Contains(line, "/")
		r.pattern = strings.TrimPrefix(line, "/")
		rules = append(rules, r)
	}
	return rules
}

// ignored is whether rel, slash-separated and relative to the template's root, is ignored. As
// in git, the last rule that matches decides.
func ignored(rules []rule, rel string, dir bool) bool {
	out := false
	for _, r := range rules {
		if r.matches(rel, dir) {
			out = !r.negate
		}
	}
	return out
}

func (r rule) matches(rel string, dir bool) bool {
	if r.dirOnly && !dir {
		return false
	}
	subject := rel
	if !r.anchored {
		subject = path.Base(rel)
	}
	ok, err := path.Match(r.pattern, subject)
	return err == nil && ok
}

// isText is whether content is source a person could read: UTF-8 with no NUL in it.
func isText(content []byte) bool {
	return bytes.IndexByte(content, 0) < 0 && utf8.Valid(content)
}

func readIgnore(root string) ([]rule, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseIgnore(string(raw)), nil
}

// Read walks root and returns its files keyed by slash-separated relative path.
func Read(root string) (map[string]File, error) {
	rules, err := readIgnore(root)
	if err != nil {
		return nil, err
	}
	out := map[string]File{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir() && (skipDirs[d.Name()] || ignored(rules, rel, true)):
			return filepath.SkipDir
		case d.IsDir(), skipFile(d.Name()), ignored(rules, rel, false):
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !isText(content) {
			dir := filepath.ToSlash(root)
			return fmt.Errorf("%s/%s is not text: a template holds source only, and this file would ship inside every whisk binary. Delete it, or list it in %s/.gitignore if the template builds it", dir, rel, dir)
		}
		// 0644, or 0755 for a canary test. The checkout's own bits are not consulted: git
		// tracks no executable in a template, and a Windows checkout reports every file as one.
		mode := fs.FileMode(0o644)
		if strings.HasSuffix(d.Name(), ".test") {
			mode = 0o755
		}
		out[rel] = File{Mode: mode, Content: strings.ReplaceAll(string(content), "\r\n", "\n")}
		return nil
	})
	return out, err
}
