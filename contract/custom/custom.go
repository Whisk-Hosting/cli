//whisk:pure

// Package custom is the folder of a fork's code that holds the business's own SQL (FORKS.md §8).
// custom/migrations holds the files that make and change the business's own tables, in the
// schema custom, each run once in name order; custom/views holds its views, functions and
// triggers, in the schema custom_views, which Whisk takes away before each update of the product
// and makes again after. This package says which files of those folders Whisk runs and the limits
// they keep; the control plane and the node check them the same way.
package custom

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	// MigrationsDir and ViewsDir are the folders, from the code's root.
	MigrationsDir = "custom/migrations"
	ViewsDir      = "custom/views"
	// FileMax caps one file, and TotalMax every file of both folders together.
	FileMax  = 256 << 10
	TotalMax = 2 << 20
	// nameMax caps a file's name.
	nameMax = 128
)

// File is one file of custom/migrations or custom/views, by its name in the folder.
type File struct {
	Name string
	SQL  string
}

// Runs reports whether Whisk runs the file at p, a path from the code's root: a file directly in
// custom/migrations or custom/views whose name ends .sql. Anything else there, such as a README,
// is left alone.
func Runs(p string) bool {
	dir, name := path.Split(p)
	dir = strings.TrimSuffix(dir, "/")
	return (dir == MigrationsDir || dir == ViewsDir) && strings.HasSuffix(name, ".sql")
}

// FileName reports whether name may be the name of a file Whisk runs: letters, digits, '_', '-'
// and '.', ending .sql, starting with neither '.' nor '-', at most 128 characters.
func FileName(name string) bool {
	base, ok := strings.CutSuffix(name, ".sql")
	if !ok || base == "" || len(name) > nameMax || base[0] == '.' || base[0] == '-' {
		return false
	}
	return !strings.ContainsFunc(base, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.')
	})
}

// Check is why the files cannot run, "" when they can: a name FileName refuses, a name twice in
// one folder, a file that is not UTF-8 text, a file over FileMax or all of them over TotalMax.
func Check(migrations, views []File) string {
	total := 0
	for _, folder := range []struct {
		dir   string
		files []File
	}{{MigrationsDir, migrations}, {ViewsDir, views}} {
		seen := map[string]bool{}
		for _, f := range folder.files {
			switch {
			case !FileName(f.Name):
				return fmt.Sprintf("%s/%q is not a name Whisk runs: use letters, digits, _, - and . and end it .sql", folder.dir, f.Name)
			case seen[f.Name]:
				return fmt.Sprintf("%s/%s is there twice", folder.dir, f.Name)
			case len(f.SQL) > FileMax:
				return fmt.Sprintf("%s/%s is over %d KiB", folder.dir, f.Name, FileMax>>10)
			case !utf8.ValidString(f.SQL) || strings.ContainsRune(f.SQL, 0):
				return fmt.Sprintf("%s/%s is not UTF-8 text", folder.dir, f.Name)
			}
			seen[f.Name] = true
			total += len(f.SQL)
		}
	}
	if total > TotalMax {
		return fmt.Sprintf("the files of %s and %s are over %d MiB together", MigrationsDir, ViewsDir, TotalMax>>20)
	}
	return ""
}
