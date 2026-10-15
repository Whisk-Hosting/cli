package doctor

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/manifest"
)

// W095: an app that serves customers keeps each customer's rows apart with row-level security
// (CONTRACT.md §8, SKILL.md §5), so every table its migrations create is expected to carry a
// forced policy, or a doctor: allow comment saying why no customer owns it.

var (
	reCreateTable  = regexp.MustCompile(`(?i)\bcreate\s+(?:unlogged\s+)?table\s+(?:if\s+not\s+exists\s+)?((?:"[^"]+"|\w+)(?:\s*\.\s*(?:"[^"]+"|\w+))?)`)
	reAlembicTable = regexp.MustCompile(`\bop\.create_table\(\s*["'](\w+)["']`)
	reForceRLS     = regexp.MustCompile(`(?i)\balter\s+table\s+(?:if\s+exists\s+)?(?:only\s+)?((?:"[^"]+"|\w+)(?:\s*\.\s*(?:"[^"]+"|\w+))?)\s+force\s+row\s+level\s+security\b`)
)

// tableName is a table reference as Postgres resolves it within the app's own schema: the last
// part of a qualified name, quotes removed, lower case. Pure.
func tableName(ref string) string {
	parts := strings.Split(ref, ".")
	return strings.ToLower(strings.Trim(strings.TrimSpace(parts[len(parts)-1]), `"`))
}

// migrationSources are the files a table can be created or given a policy in: SQL files, and
// Python code for Alembic and op.execute. Tests and vendored folders are left out.
func migrationSources(r Repo) []File {
	out := r.Code(stack.PY)
	for _, f := range r.Files {
		if f.Content != nil && strings.EqualFold(path.Ext(f.Path), ".sql") && !stack.IsTest(f.Path) && !stack.IsVendored(f.Path) {
			out = append(out, f)
		}
	}
	return out
}

// unprotectedTables are the tables created in files that no file forces row-level security
// on, each at its first create. Pure.
func unprotectedTables(files []File) []hit {
	forced := map[string]bool{}
	for _, h := range matchesAll(files, []*regexp.Regexp{reForceRLS}, 1) {
		forced[tableName(h.Name)] = true
	}
	seen := map[string]bool{}
	var out []hit
	for _, h := range matchesAll(files, []*regexp.Regexp{reCreateTable, reAlembicTable}, 1) {
		name := tableName(h.Name)
		if forced[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, hit{Name: name, File: h.File, Line: h.Line})
	}
	return out
}

func w095(r Repo, _ Context) outcome {
	if r.Manifest.CustomerIdentity != manifest.CustomerIdentityApp && r.Manifest.CustomerIdentity != manifest.CustomerIdentityOrg {
		return outcome{}
	}
	var out outcome
	for _, h := range unprotectedTables(migrationSources(r)) {
		out = out.add("W095", h.File, h.Line, fmt.Sprintf("Table %s is created in %s without row-level security, so a query that forgets its filter can show one customer another's rows.", h.Name, h.File))
	}
	return out
}
