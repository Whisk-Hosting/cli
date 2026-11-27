package doctor

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// Staff routes in an app customers sign in to (W180): the edge signs everyone in before a request
// reaches the app, customers too once customer_identity is on, so a route named for staff that never
// asks who the caller is opens to every signed-in customer. Doctor warns; nothing is refused.

// staffRoute is a route a framework registers in code, any method, whose path has a segment named
// for staff (adminSegment).
var staffRoute = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {regexp.MustCompile("(?i)\\.(?:get|post|put|patch|delete|all|use|route)\\(\\s*[\"'`](/[^\"'`]*)[\"'`]")},
	stack.PY: {regexp.MustCompile(`@\w+\.(?:get|post|put|patch|delete|route|api_route)\(\s*["'](/[^"']*)["']`)},
	stack.GO: {
		regexp.MustCompile(`\.(?:Get|Post|Put|Patch|Delete|Any|Group|Route|Handle|HandleFunc|GET|POST|PUT|PATCH|DELETE)\(\s*"(?:[A-Z]+ )?(/[^"]*)"`),
	},
}

// fileRouteDir is a folder whose files a framework serves by their path (Next.js, SvelteKit,
// Nuxt, Astro, Remix), so a file under a staff segment there is a staff route itself.
var fileRouteDir = regexp.MustCompile(`(?:^|/)(?:app|pages|routes|server/api)/`)

// fileRouteExt are the files those frameworks serve by path; a Python package named app/ is not
// one.
var fileRouteExt = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".svelte": true, ".vue": true, ".astro": true}

// callerChecks are the signs that code asks who the caller is before it answers: the team or
// customer audience, the caller's roles, or the template's helpers that read them.
var callerChecks = regexp.MustCompile(`(?i)x-whisk-(?:audience|roles)|x_whisk_(?:audience|roles)|\b(?:has_?role|scope_?for|can_?see|can_?change|require_?team|is_?team)\s*\(|\.(?:audience|roles)\b|\[["'](?:audience|roles)["']\]`)

// middlewareFile is a file a framework runs before every route (Next.js and SvelteKit's), whose
// check covers each route.
func middlewareFile(p string) bool {
	base := strings.TrimSuffix(path.Base(p), path.Ext(p))
	return base == "middleware" || base == "hooks.server"
}

func w180(r Repo, c Context) outcome {
	if r.Manifest.CustomerIdentity != "app" && r.Manifest.CustomerIdentity != "org" {
		return outcome{}
	}
	if promotedNow(c) || isShop(r) || ownSignin(r) {
		return outcome{}
	}
	code := serverCode(r)
	for _, f := range code {
		if middlewareFile(f.Path) && callerChecks.Match(f.Content) {
			return outcome{}
		}
	}
	var out outcome
	for _, f := range code {
		if helperModule(f.Path) || callerChecks.Match(f.Content) {
			continue
		}
		if route, line, ok := firstStaffRoute(f); ok {
			out = out.add("W180", f.Path, line, fmt.Sprintf("%s serves %s, a route named for staff, and never checks who the caller is, so every signed-in customer can open it.", f.Path, route))
		}
	}
	return out
}

// firstStaffRoute is the first route named for staff that a file registers or is, and its line.
func firstStaffRoute(f File) (string, int, bool) {
	if loc := fileRouteDir.FindStringIndex(f.Path); loc != nil && fileRouteExt[path.Ext(f.Path)] {
		rest := "/" + f.Path[loc[1]:]
		if adminSegment.MatchString(path.Dir(rest)) {
			return path.Dir(rest), 1, true
		}
	}
	for _, re := range staffRoute[stack.LangOf(f.Path)] {
		for _, m := range re.FindAllSubmatchIndex(f.Content, -1) {
			if route := string(f.Content[m[2]:m[3]]); adminSegment.MatchString(route) {
				return route, lineAt(f.Content, m[0]), true
			}
		}
	}
	return "", 0, false
}
