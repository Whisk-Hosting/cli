package doctor

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// The rules here find the habits behind the common security holes in business apps, the ones
// the skill's "Who may see and change what" (§4) names: SQL built from strings (W096), text
// inserted as raw HTML (W097), a list or search on a table of customers' records that is not
// limited to the signed-in person (W098), and a public route that takes writes without a
// challenge or is named for admins (W099). Each is a warning read from code, not proof
// (doctor-rules.md).

// sqlShape is the start of a statement, shaped so English text ("Update your details") does not
// look like one.
const sqlShape = `(?:select\s[^"'` + "`" + `]*?\bfrom\b|insert\s+into\b|update\s+\S+\s+set\b|delete\s+from\b)`

var sqlFromStrings = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		// sql.unsafe(`… ${x}`), pool.query(`select … ${x}`), knex.raw(`… ${x}`): an untagged
		// template literal is plain text by the time the driver sees it. A tagged template
		// (sql`… ${x}`) sends parameters and is not matched.
		regexp.MustCompile("(?i)\\.(?:unsafe|query|execute|raw|run|all|get|prepare)\\(\\s*`\\s*" + sqlShape + "[^`]*\\$\\{"),
		// pool.query("select … " + x)
		regexp.MustCompile(`(?i)\.(?:unsafe|query|execute|raw|run|all|get|prepare)\(\s*(?:"\s*` + sqlShape + `[^"]*"|'\s*` + sqlShape + `[^']*')\s*\+`),
	},
	stack.PY: {
		// f"select … {x}", "select … %s" % x, "select … {}".format(x), "select … " + x
		regexp.MustCompile(`(?i)\bf["']{1,3}\s*` + sqlShape + `[^"']*\{`),
		regexp.MustCompile(`(?i)(?:"\s*` + sqlShape + `[^"]*"|'\s*` + sqlShape + `[^']*')\s*(?:%\s*[\w(]|\.format\(|\+\s*\w)`),
	},
	stack.GO: {
		// fmt.Sprintf("select … %s", x)
		regexp.MustCompile("(?i)fmt\\.Sprintf\\(\\s*[\"`]\\s*" + sqlShape + "[^\"`]*%[svdq]"),
	},
}

// startsWithSQL is a string literal holding a statement (W098). Its first character is the quote.
var startsWithSQL = regexp.MustCompile(`(?is)^.\s*` + sqlShape)

func w096(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range serverCode(r) {
		if schemaFile(f.Path) {
			continue
		}
		if hs := matches(f, sqlFromStrings[stack.LangOf(f.Path)], 0); len(hs) > 0 {
			h := earliest(hs)
			out = out.add("W096", f.Path, h.Line, fmt.Sprintf("%s line %d builds SQL by joining text, so a value a person types can change the statement.", f.Path, h.Line))
		}
	}
	return out
}

var rawHTML = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`\bdangerouslySetInnerHTML\b`),
		regexp.MustCompile("\\.(?:innerHTML|outerHTML)\\s*\\+?=\\s*(?:[^\"'`\\s]|`[^`]*\\$\\{|[\"'][^\"']*[\"']\\s*\\+)"),
		regexp.MustCompile(`\.insertAdjacentHTML\(`),
		regexp.MustCompile(`\bdocument\.write(?:ln)?\(`),
	},
	stack.PY: {
		regexp.MustCompile(`\bMarkup\(`),
		regexp.MustCompile(`\bmark_safe\(`),
		regexp.MustCompile(`\bautoescape\s*=\s*False\b`),
	},
	stack.GO: {
		regexp.MustCompile(`\btemplate\.HTML\(`),
	},
}

// rawHTMLMarkup is the same habit in page templates: Jinja's |safe, Vue's v-html, Svelte's
// {@html}, Handlebars' triple braces.
var rawHTMLMarkup = []*regexp.Regexp{
	regexp.MustCompile(`\|\s*safe\b`),
	regexp.MustCompile(`\bv-html\s*=`),
	regexp.MustCompile(`\{@html\s`),
	regexp.MustCompile(`\{\{\{[^}]`),
}

func w097(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range pageFiles(r) {
		pats := rawHTML[stack.LangOf(f.Path)]
		if pats == nil {
			pats = rawHTMLMarkup
		}
		if hs := matches(f, pats, 0); len(hs) > 0 {
			h := earliest(hs)
			out = out.add("W097", f.Path, h.Line, fmt.Sprintf("%s line %d inserts text as raw HTML; if any of it came from a person, it can run as script in another person's browser.", f.Path, h.Line))
		}
	}
	return out
}

// ownerColumns name the person a record belongs to.
const ownerColumns = `owner_id|author_id|customer_id|user_id|created_by`

var (
	createTableSQL   = regexp.MustCompile(`(?is)create\s+table\s+(?:if\s+not\s+exists\s+)?(?:"?\w+"?\.)?"?(\w+)"?\s*\((.*?)\)\s*;`)
	ownerColumnSQL   = regexp.MustCompile(`(?i)(?:^|[\s,(])"?(` + ownerColumns + `)"?\s+\w`)
	drizzleTable     = regexp.MustCompile(`(?s)pgTable\(\s*["'](\w+)["']\s*,\s*\{(.*?)\n\}`)
	alembicTable     = regexp.MustCompile(`(?s)create_table\(\s*["'](\w+)["'](.*?)\n\s*\)`)
	ownerColumnQuote = regexp.MustCompile(`["'](` + ownerColumns + `)["']`)
	sqlLiteral       = regexp.MustCompile("(?s)\"(?:[^\"\\\\\\n]|\\\\.)*\"|'(?:[^'\\\\\\n]|\\\\.)*'|`[^`]*`")
)

// ownedTables maps each table that has an owner column to that column, read from SQL
// migrations, Drizzle schemas and Alembic migrations. Pure.
func ownedTables(files []File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		if f.Content == nil || f.Binary {
			continue
		}
		switch {
		case strings.HasSuffix(strings.ToLower(f.Path), ".sql"):
			for _, m := range createTableSQL.FindAllSubmatch(f.Content, -1) {
				if c := ownerColumnSQL.FindSubmatch(m[2]); c != nil {
					out[strings.ToLower(string(m[1]))] = strings.ToLower(string(c[1]))
				}
			}
		case stack.LangOf(f.Path) == stack.JS:
			for _, m := range drizzleTable.FindAllSubmatch(f.Content, -1) {
				if c := ownerColumnQuote.FindSubmatch(m[2]); c != nil {
					out[strings.ToLower(string(m[1]))] = string(c[1])
				}
			}
		case stack.LangOf(f.Path) == stack.PY:
			for _, m := range alembicTable.FindAllSubmatch(f.Content, -1) {
				if c := ownerColumnQuote.FindSubmatch(m[2]); c != nil {
					out[strings.ToLower(string(m[1]))] = string(c[1])
				}
			}
		}
	}
	return out
}

// unscopedRead reports whether a SQL statement reads or changes rows of table without naming
// the owner column, and not one row by its id (which the app then checks with canSee). Pure.
func unscopedRead(stmt, table, owner string) bool {
	s := strings.ToLower(stmt)
	t := regexp.QuoteMeta(table)
	touches := regexp.MustCompile(`\b(?:from|join|update)\s+(?:"?\w+"?\.)?"?` + t + `"?(?:\s|$|,|;|\))`)
	if !touches.MatchString(s) || strings.Contains(s, owner) {
		return false
	}
	byID := regexp.MustCompile(`\bwhere\s+(?:"?` + t + `"?\.)?"?id"?\s*=`)
	return !byID.MatchString(s)
}

func w098(r Repo, _ Context) outcome {
	if r.Manifest.CustomerIdentity != "app" && r.Manifest.CustomerIdentity != "org" {
		return outcome{}
	}
	tables := ownedTables(r.Files)
	if len(tables) == 0 {
		return outcome{}
	}
	names := make([]string, 0, len(tables))
	for t := range tables {
		names = append(names, t)
	}
	sort.Strings(names)
	var out outcome
	for _, table := range names {
		owner := tables[table]
	files:
		for _, f := range serverCode(r) {
			if schemaFile(f.Path) {
				continue
			}
			for _, loc := range sqlLiteral.FindAllIndex(f.Content, -1) {
				lit := string(f.Content[loc[0]:loc[1]])
				if !startsWithSQL.MatchString(lit) || !unscopedRead(lit, table, owner) {
					continue
				}
				line := lineAt(f.Content, loc[0])
				out = out.add("W098", f.Path, line, fmt.Sprintf("%s line %d reads %s without limiting it to %s, so with customers signed in one customer may see another's records.", f.Path, line, table, owner))
				break files
			}
		}
	}
	return out
}

// schemaFile is a migration or a schema definition, which names tables on purpose. Pure.
func schemaFile(p string) bool {
	l := strings.ToLower(p)
	return migrationFile(p) || strings.HasPrefix(l, "drizzle/") || strings.Contains(l, "/drizzle/") || strings.HasPrefix(path.Base(l), "schema.")
}

// writeRoute captures a route registered for a method that changes data, with its path.
var writeRoute = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {regexp.MustCompile("(?i)\\.(post|put|patch|delete)\\(\\s*[\"'`](/[^\"'`]*)[\"'`]")},
	stack.PY: {regexp.MustCompile(`@\w+\.(post|put|patch|delete)\(\s*["'](/[^"']*)["']`)},
	stack.GO: {
		regexp.MustCompile(`\.(Post|Put|Patch|Delete|POST|PUT|PATCH|DELETE)\(\s*"(/[^"]*)"`),
		regexp.MustCompile(`\b(?:Handle|HandleFunc)\(\s*"(POST|PUT|PATCH|DELETE) (/[^"]*)"`),
	},
}

// routeGlob turns a router's path parameters (:id, {id}, <id>) into the manifest's * so the
// route can be matched against routes.public. Pure.
func routeGlob(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, ":") || (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) || (strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">")) {
			segs[i] = "x"
		}
	}
	return strings.Join(segs, "/")
}

var adminSegment = regexp.MustCompile(`(?i)(?:^|/)(?:admin|administrator|manage|internal|staff)(?:/|$|\*)`)

func w099(r Repo, _ Context) outcome {
	var out outcome
	for i, p := range r.Manifest.Routes.Public {
		if adminSegment.MatchString(p) {
			out = out.add("W099", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/routes/public/%d", i)), fmt.Sprintf("routes.public lists %s, which is named for staff; anyone on the internet can open it.", p))
		}
	}
	if r.Manifest.Promoted || medusaShop(r) {
		// A Promoted app with its own sign-in, or a Medusa shop, lists every route as public;
		// its sign-in decides who may write.
		return out
	}
	seen := map[string]bool{}
	for _, f := range serverCode(r) {
		for _, re := range writeRoute[stack.LangOf(f.Path)] {
			for _, m := range re.FindAllSubmatchIndex(f.Content, -1) {
				method := strings.ToUpper(string(f.Content[m[2]:m[3]]))
				route := string(f.Content[m[4]:m[5]])
				concrete := routeGlob(route)
				if !r.Manifest.IsPublic(concrete) || r.Manifest.IsService(route) || (method == "POST" && r.Manifest.NeedsChallenge(concrete)) || seen[method+" "+route] {
					continue
				}
				seen[method+" "+route] = true
				line := lineAt(f.Content, m[0])
				why := "anyone on the internet can send it, with no challenge to slow down a script"
				if method != "POST" {
					why = "anyone on the internet can change or delete data through it"
				}
				out = out.add("W099", f.Path, line, fmt.Sprintf("%s %s is a public route, so %s.", method, route, why))
			}
		}
	}
	return out
}

func earliest(hs []hit) hit {
	first := hs[0]
	for _, h := range hs[1:] {
		if h.Line < first.Line {
			first = h
		}
	}
	return first
}
