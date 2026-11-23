package doctor

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// W105: an app with `database_role: restricted` connects as a login that owns nothing and is a
// member of no role (CONTRACT.md §8, SKILL.md §5). Schema changes outside the migrate step and
// SET ROLE fail under it with permission denied, so they are found before a deploy does.
// Migrations run at start are W102's finding, whose message says they fail under this setting.

var (
	reSetRole = regexp.MustCompile(`(?i)\bset\s+(?:local\s+|session\s+)?role\s+\S`)
	reRunDDL  = regexp.MustCompile(`(?i)\b(?:create\s+(?:or\s+replace\s+)?(?:unique\s+)?(?:table|index|schema|policy|extension|function|trigger|view|sequence|type)|alter\s+(?:table|policy|schema|sequence|function|view)|drop\s+(?:table|policy|schema|index|function|trigger|view|sequence))\b`)
)

// migrationPath reports whether a file is part of the app's migrations, which run as the owner
// in the migrate step: anything under a folder or named for migrations, Alembic or Drizzle, or
// Prisma's migrations folder. Pure.
func migrationPath(p string) bool {
	lower := strings.ToLower(p)
	return strings.Contains(lower, "migrat") || strings.Contains(lower, "alembic") || strings.HasPrefix(lower, "drizzle/") || strings.Contains(lower, "/drizzle/") || strings.HasSuffix(lower, ".sql")
}

// migrationFiles is every code file that runs in the migrate step: the files migrationPath
// names, the file the manifest's migrate command runs (src/migrate.ts for "node
// dist/migrate.js"), and every file those import, whose statements the migration runs as the
// owner. Pure.
func migrationFiles(files []File, migrate string) map[string]bool {
	var start []string
	for _, f := range files {
		if migrationPath(f.Path) || commandNames(migrate, f.Path) {
			start = append(start, f.Path)
		}
	}
	return importClosure(files, start)
}

// commandNames reports whether a command runs a code file, by the file's name without its
// extension or the folders built output adds: "node dist/setup.js" runs src/setup.ts. Pure.
func commandNames(command, p string) bool {
	if command == "" {
		return false
	}
	stem := strings.TrimSuffix(path.Base(p), path.Ext(p))
	for _, word := range strings.Fields(command) {
		w := strings.Trim(word, `"'`)
		if strings.TrimSuffix(path.Base(w), path.Ext(w)) == stem && strings.ContainsAny(w, "./") {
			return true
		}
	}
	return false
}

// runLoginHits are the places in server code that a run login cannot run: a role switch, and the
// first schema change in each file outside the migrations. Comments are not read. Pure.
func runLoginHits(files []File, migrations map[string]bool) (roles, ddl []hit) {
	for _, f := range files {
		src := blankComments(stack.LangOf(f.Path), f.Content)
		for _, loc := range reSetRole.FindAllIndex(src, -1) {
			roles = append(roles, hit{Name: strings.TrimSpace(string(src[loc[0]:loc[1]])), File: f.Path, Line: lineAt(src, loc[0])})
		}
		if migrations[f.Path] {
			continue
		}
		if loc := reRunDDL.FindIndex(src); loc != nil {
			ddl = append(ddl, hit{Name: strings.Join(strings.Fields(string(src[loc[0]:loc[1]])), " "), File: f.Path, Line: lineAt(src, loc[0])})
		}
	}
	return roles, ddl
}

func w105(r Repo, _ Context) outcome {
	if !r.Manifest.Restricted() {
		return outcome{}
	}
	code := r.AllCode()
	roles, ddl := runLoginHits(code, migrationFiles(code, r.Manifest.Migrate))
	var out outcome
	for _, h := range roles {
		out = out.add("W105", h.File, h.Line, fmt.Sprintf("%s switches role at line %d; with database_role: restricted the app's login is a member of no role, so it fails.", h.File, h.Line))
	}
	for _, h := range ddl {
		out = out.add("W105", h.File, h.Line, fmt.Sprintf("%s runs %s outside the migrations; with database_role: restricted the app's login cannot change the schema, so it fails with permission denied.", h.File, strings.ToLower(h.Name)))
	}
	return out
}
