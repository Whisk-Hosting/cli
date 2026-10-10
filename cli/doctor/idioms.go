package doctor

import (
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
)

// The idioms doctor recognises, as listed in doctor-rules.md. Each table is data; the rules
// apply them. An agent that writes one of these forms writes code doctor reads correctly.

// hit is one match: what was named and where.
type hit struct {
	Name string
	File string
	Line int
}

const envName = `([A-Z][A-Z0-9_]*)`

// envReadPatterns capture an environment name being read.
var envReadPatterns = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`process\.env\.` + envName),
		regexp.MustCompile(`process\.env\[["']` + envName + `["']\]`),
		regexp.MustCompile(`Bun\.env\.` + envName),
		regexp.MustCompile(`Deno\.env\.get\(\s*["']` + envName + `["']`),
	},
	stack.PY: {
		regexp.MustCompile(`os\.environ\[["']` + envName + `["']\]`),
		regexp.MustCompile(`os\.environ\.get\(\s*["']` + envName + `["']`),
		regexp.MustCompile(`os\.getenv\(\s*["']` + envName + `["']`),
	},
	stack.GO: {
		regexp.MustCompile(`os\.Getenv\(\s*"` + envName + `"`),
		regexp.MustCompile(`os\.LookupEnv\(\s*"` + envName + `"`),
	},
}

// envHelperPattern is the helper form every language shares: a function whose name contains
// env, called with the name as its first literal argument (env("PORT"), mustEnv("X")).
var envHelperPattern = regexp.MustCompile(`\b[A-Za-z_]*[eE]nv[A-Za-z_]*\(\s*["']` + envName + `["']`)

func envReads(f File) []hit {
	lang := stack.LangOf(f.Path)
	pats := append([]*regexp.Regexp{envHelperPattern}, envReadPatterns[lang]...)
	return matches(f, pats, 1)
}

// nameLiteralPattern captures an environment-style name written as a whole string literal
// ("HUBSPOT_API_KEY"). Code that reads secrets through a table of names, env[KEYS.hubspot]
// with KEYS = { hubspot: 'HUBSPOT_API_KEY' }, names each secret this way and nowhere else.
var nameLiteralPattern = regexp.MustCompile("[\"'`]" + envName + "[\"'`]")

func nameLiterals(f File) []hit {
	return matches(f, []*regexp.Regexp{nameLiteralPattern}, 1)
}

// stepCallPatterns capture a step name in a step call.
var stepCallPatterns = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`\bstep\.(?:run|sleep|sleepUntil|waitForEvent|sendEvent|approval|invoke)\(\s*["'` + "`" + `]([^"'` + "`" + `]+)["'` + "`" + `]`),
	},
	stack.PY: {
		regexp.MustCompile(`\bstep\.(?:run|sleep|sleep_until|wait_for_event|send_event|invoke)\(\s*["']([^"']+)["']`),
	},
	stack.GO: {
		regexp.MustCompile(`\bstep\.(?:Run|Sleep|SleepUntil|WaitForEvent|Send|Invoke)(?:\[[^\]]*\])?\(\s*\w+\s*,\s*"([^"]+)"`),
	},
}

// stepCalls returns the literal step names used in a file. Names built at runtime (a template
// literal with ${}) are not literals and are left to the graph's own ids.
func stepCalls(f File) []hit {
	var out []hit
	for _, h := range matches(f, stepCallPatterns[stack.LangOf(f.Path)], 1) {
		if strings.Contains(h.Name, "${") {
			continue
		}
		h.Name = strings.TrimSuffix(h.Name, "/request")
		out = append(out, h)
	}
	return out
}

// routeIdioms register a path with a router. method is "" for forms that take any method.
type routeIdiom struct {
	lang   stack.Lang
	method string // GET, POST or "" for any
	// build returns the pattern that matches the idiom for one literal path.
	build func(quoted string) string
}

var routeIdioms = []routeIdiom{
	{stack.JS, "GET", func(p string) string { return `\.(?:get)\(\s*["'` + "`" + `]` + p + `["'` + "`" + `]` }},
	{stack.JS, "POST", func(p string) string { return `\.(?:post)\(\s*["'` + "`" + `]` + p + `["'` + "`" + `]` }},
	{stack.JS, "", func(p string) string {
		return `\.(?:all|route|use|put|patch|delete)\(\s*["'` + "`" + `]` + p + `["'` + "`" + `]`
	}},
	{stack.JS, "", func(p string) string {
		return `\.on\(\s*["'][A-Za-z]+["']\s*,\s*["'` + "`" + `]` + p + `["'` + "`" + `]`
	}},
	// A plain node:http server compares the path itself: req.url === "/health",
	// pathname == "/health", or case "/health": in a switch. Any method, since the method is
	// checked separately if at all.
	{stack.JS, "", func(p string) string {
		return `\b(?:url|pathname|path)\s*===?\s*["'` + "`" + `]` + p + `["'` + "`" + `]|\bcase\s+["'` + "`" + `]` + p + `["'` + "`" + `]\s*:`
	}},
	{stack.PY, "GET", func(p string) string { return `\.(?:get)\(\s*["']` + p + `["']` }},
	{stack.PY, "POST", func(p string) string { return `\.(?:post)\(\s*["']` + p + `["']` }},
	{stack.PY, "", func(p string) string {
		return `\.(?:route|api_route|add_api_route|add_url_rule|put|patch|delete)\(\s*["']` + p + `["']`
	}},
	{stack.PY, "", func(p string) string { return `\bpath\(\s*["']` + strings.TrimPrefix(p, `/`) + `/?["']` }},
	{stack.GO, "GET", func(p string) string { return `\.(?:Get|GET)\(\s*"` + p + `"` }},
	{stack.GO, "POST", func(p string) string { return `\.(?:Post|POST)\(\s*"` + p + `"` }},
	{stack.GO, "", func(p string) string {
		return `\b(?:Handle|HandleFunc|Route|Mount|Any|Put|Patch|Delete)\(\s*"` + p + `"`
	}},
	{stack.GO, "", func(p string) string { return `\b(?:Handle|HandleFunc)\(\s*"[A-Z]+ ` + p + `"` }},
	{stack.GO, "", func(p string) string { return `\.Method\(\s*"[A-Z]+"\s*,\s*"` + p + `"` }},
}

// anyRoutePattern says whether a language's files register routes at all; when none does,
// route checks are skipped rather than failed.
var anyRoutePattern = map[stack.Lang]*regexp.Regexp{
	stack.JS: regexp.MustCompile(`\.(?:get|post|put|patch|delete|all|route|use|on)\(\s*["'` + "`" + `]/|\b(?:url|pathname)\s*===?\s*["'` + "`" + `]/|\bcase\s+["'` + "`" + `]/`),
	stack.PY: regexp.MustCompile(`\.(?:get|post|put|patch|delete|route|api_route|add_api_route|add_url_rule)\(\s*["']/|\bpath\(\s*["']`),
	stack.GO: regexp.MustCompile(`\.(?:Get|Post|Put|Patch|Delete|Handle|HandleFunc|Route|Mount|Method)\(\s*"`),
}

// routeFound reports whether a path is registered for the method (GET, POST) in any code file,
// and whether any router idiom exists at all for the languages present.
func routeFound(r Repo, routePath, method string) (found, anyRouter bool) {
	if len(routeFiles(r, routePath, method)) > 0 || fileRouteExists(r, routePath) {
		return true, true
	}
	for _, lang := range []stack.Lang{stack.JS, stack.PY, stack.GO} {
		for _, f := range r.Code(lang) {
			if anyRoutePattern[lang].Match(f.Content) {
				return false, true
			}
		}
	}
	return false, false
}

// routeFiles is every code file that registers the path for the method through a router
// idiom, so a rule can read the handler's own file.
func routeFiles(r Repo, routePath, method string) []File {
	quoted := regexp.QuoteMeta(routePath)
	var out []File
	for _, lang := range []stack.Lang{stack.JS, stack.PY, stack.GO} {
		files := r.Code(lang)
		for _, idiom := range routeIdioms {
			if idiom.lang != lang || (idiom.method != "" && idiom.method != method) {
				continue
			}
			re := regexp.MustCompile(idiom.build(quoted))
			for _, f := range files {
				if re.Match(f.Content) && !hasPath(out, f.Path) {
					out = append(out, f)
				}
			}
		}
	}
	return out
}

func hasPath(files []File, p string) bool {
	for _, f := range files {
		if f.Path == p {
			return true
		}
	}
	return false
}

// fileRouteExists covers file-system routers (Next.js app and pages directories).
func fileRouteExists(r Repo, routePath string) bool {
	p := strings.Trim(routePath, "/")
	candidates := []string{
		"app/" + p + "/route", "src/app/" + p + "/route",
		"pages/api/" + p, "src/pages/api/" + p, "pages/" + p, "src/pages/" + p,
		"pages/api/" + p + "/index", "pages/" + p + "/index",
		"src/api/" + p + "/route", // Medusa
	}
	for _, f := range r.Files {
		ext := path.Ext(f.Path)
		if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
			continue
		}
		base := strings.TrimSuffix(f.Path, ext)
		for _, c := range candidates {
			if base == c {
				return true
			}
		}
	}
	return false
}

// portRead reports whether PORT is read from the environment anywhere doctor understands.
func portRead(r Repo) (bool, string) {
	for _, f := range r.AllCode() {
		for _, h := range envReads(f) {
			if h.Name == "PORT" {
				return true, f.Path
			}
		}
	}
	for _, name := range dockerfiles(r) {
		for _, line := range instructions(r.Read(name)) {
			if (strings.HasPrefix(line, "CMD") || strings.HasPrefix(line, "ENTRYPOINT")) && strings.Contains(line, "$PORT") || strings.Contains(line, "${PORT") {
				return true, name
			}
		}
	}
	if pkg := r.Read("package.json"); pkg != nil && (strings.Contains(string(pkg), "$PORT") || strings.Contains(string(pkg), "${PORT")) {
		return true, "package.json"
	}
	return false, ""
}

var loopbackPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:hostname|host|addr|bind|listen)\s*[:=]\s*["'](?:127\.0\.0\.1|localhost)`),
	regexp.MustCompile(`Listen\(\s*"tcp"\s*,\s*"(?:127\.0\.0\.1|localhost):`),
	regexp.MustCompile(`--host[= ]["']?(?:127\.0\.0\.1|localhost)\b`),
	regexp.MustCompile(`["'](?:127\.0\.0\.1|localhost):\d+["']\s*(?:,|\))`),
}

// loopbackListens finds code that binds to the loopback interface.
func loopbackListens(r Repo) []hit {
	return matchesAll(r.AllCode(), loopbackPatterns, 0)
}

var writePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bfs(?:\.promises)?\.(?:writeFile|writeFileSync|appendFile|appendFileSync|mkdir|mkdirSync|createWriteStream)\(\s*["'` + "`" + `](?:\./)?(?:data|uploads|tmp|cache|logs|db)\b`),
	regexp.MustCompile(`\bopen\(\s*["'](?:\./)?(?:data|uploads|tmp|cache|logs|db)/[^"']*["']\s*,\s*["'][wa]`),
	regexp.MustCompile(`\bos\.(?:makedirs|mkdir)\(\s*["'](?:\./)?(?:data|uploads|tmp|cache|logs|db)\b`),
	regexp.MustCompile(`\bos\.(?:WriteFile|MkdirAll|Mkdir|Create|OpenFile)\(\s*"(?:\./)?(?:data|uploads|tmp|cache|logs|db)\b`),
	regexp.MustCompile(`["'](?:\./)?(?:data|db|tmp|cache)/[^"']*\.(?:db|sqlite3?)["']`),
}

// writesOutsideTmp finds file writes into relative directories that vanish on redeploy.
func writesOutsideTmp(r Repo) []hit {
	return matchesAll(r.AllCode(), writePatterns, 0)
}

// dockerfiles lists the Dockerfile the build uses, when present.
func dockerfiles(r Repo) []string {
	name := "Dockerfile"
	if r.ManifestErr == nil && r.Manifest.Build.Dockerfile != "" {
		name = r.Manifest.Build.Dockerfile
	}
	if r.Has(name) {
		return []string{name}
	}
	return nil
}

// instructions returns a Dockerfile's instructions with continuation lines joined and
// comments removed, upper-cased keyword first.
func instructions(src []byte) []string {
	lines := instructionLines(src)
	out := make([]string, len(lines))
	for i, in := range lines {
		out[i] = in.Text
	}
	return out
}

// instruction is one Dockerfile instruction and the line it starts on.
type instruction struct {
	Line int
	Text string
}

// instructionLines is instructions with the line each one starts on.
func instructionLines(src []byte) []instruction {
	var out []instruction
	var cur string
	first := 0
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if cur == "" {
			first = i + 1
		}
		if strings.HasSuffix(line, "\\") {
			cur += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		cur += line
		fields := strings.Fields(cur)
		if len(fields) > 0 {
			fields[0] = strings.ToUpper(fields[0])
			out = append(out, instruction{Line: first, Text: strings.Join(fields, " ")})
		}
		cur = ""
	}
	return out
}

// stripComments removes whole-line and block comments so a literal inside one does not count.
func stripComments(lang stack.Lang, content []byte) []byte {
	s := string(content)
	switch lang {
	case stack.PY:
		s = reHashLine.ReplaceAllString(s, "")
	default:
		s = reBlock.ReplaceAllString(s, "")
		s = reSlashLine.ReplaceAllString(s, "")
		s = reTrailingSlash.ReplaceAllString(s, "")
	}
	return []byte(s)
}

var (
	reHashLine      = regexp.MustCompile(`(?m)^\s*#.*$`)
	reSlashLine     = regexp.MustCompile(`(?m)^\s*//.*$`)
	reTrailingSlash = regexp.MustCompile(`(?m)\s//\s.*$`)
	reBlock         = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// literalFound reports whether a string literal with exactly this value appears in any code
// file outside comments.
func literalFound(r Repo, value string) bool {
	re := regexp.MustCompile(`["'` + "`" + `]` + regexp.QuoteMeta(value) + `["'` + "`" + `]`)
	for _, f := range r.AllCode() {
		if re.Match(stripComments(stack.LangOf(f.Path), f.Content)) {
			return true
		}
	}
	return false
}

func matches(f File, pats []*regexp.Regexp, group int) []hit {
	var out []hit
	for _, re := range pats {
		for _, m := range re.FindAllSubmatchIndex(f.Content, -1) {
			name := ""
			if group > 0 && len(m) > 2*group+1 && m[2*group] >= 0 {
				name = string(f.Content[m[2*group]:m[2*group+1]])
			} else {
				name = string(f.Content[m[0]:m[1]])
			}
			out = append(out, hit{Name: name, File: f.Path, Line: lineAt(f.Content, m[0])})
		}
	}
	return out
}

func matchesAll(files []File, pats []*regexp.Regexp, group int) []hit {
	var out []hit
	for _, f := range files {
		out = append(out, matches(f, pats, group)...)
	}
	return out
}

// pageExts are the template and markup files a password field can live in, besides code.
var pageExts = map[string]bool{
	".html": true, ".htm": true, ".jinja": true, ".jinja2": true, ".j2": true, ".ejs": true,
	".hbs": true, ".handlebars": true, ".njk": true, ".svelte": true, ".vue": true, ".astro": true,
	".tmpl": true, ".gohtml": true, ".templ": true, ".erb": true, ".mustache": true,
}

// pageFiles is the app's own code plus its templates and markup.
func pageFiles(r Repo) []File {
	out := r.AllCode()
	for _, f := range r.Files {
		if f.Content != nil && pageExts[strings.ToLower(path.Ext(f.Path))] && !stack.IsTest(f.Path) && !stack.IsVendored(f.Path) {
			out = append(out, f)
		}
	}
	return out
}

// passwordPatterns find a password field the app renders itself: an HTML or JSX input, an
// input built in script, or a Python form library's password field.
var passwordPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\btype\s*[=:]\s*\{?\s*["'` + "`" + `]?password\b`),
	regexp.MustCompile(`\.type\s*=\s*["'` + "`" + `]password["'` + "`" + `]`),
	regexp.MustCompile(`\b(?:PasswordField|PasswordInput)\b`),
}

// passwordFields is the first password field in each file that has one.
func passwordFields(r Repo) []hit {
	var out []hit
	for _, f := range pageFiles(r) {
		hs := matches(f, passwordPatterns, 0)
		if len(hs) == 0 {
			continue
		}
		first := hs[0]
		for _, h := range hs[1:] {
			if h.Line < first.Line {
				first = h
			}
		}
		out = append(out, first)
	}
	return out
}
