// Package safefile writes files the CLI is asked to create inside a directory it does not
// trust: a cloned repository, an app directory. A path that climbs out of the directory or
// passes through a symbolic link is refused, so a repository cannot point a write at the
// user's shell profile or anything else outside it. Writes go through a temporary file that is
// renamed into place, so a half-written file is never left behind.
package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Inside returns the path of rel under root. It refuses an absolute rel, one that climbs out of
// root with "..", and one whose existing parts (any directory on the way, or the file itself)
// are symbolic links.
func Inside(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || strings.HasPrefix(clean, string(filepath.Separator)) ||
		clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write %s: the path is outside %s", rel, root)
	}
	walked := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		walked = filepath.Join(walked, part)
		info, err := os.Lstat(walked)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to write %s: %s is a symbolic link", rel, walked)
		}
	}
	return filepath.Join(root, clean), nil
}

// Write writes data to rel under root, creating directories as needed, after Inside agrees.
func Write(root, rel string, data []byte, perm fs.FileMode) error {
	target, err := Inside(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return WriteFile(target, data, perm)
}

// WriteFile replaces path with data: a temporary file beside it, then a rename. The rename
// replaces a symbolic link at path rather than following it, and the file ends with exactly
// perm whatever the old file's mode was.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}
