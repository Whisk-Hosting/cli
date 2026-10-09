package doctor

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/internal/stack"
)

// The rules here find what the skill's "Do not" list warns about and what only fails after a
// deploy: a static folder the commit does not hold (W004), a health timeout longer than a wake
// waits (W005), timers inside the app (W023), session state on the pooled database connection
// (W024), a function name the code does not use (W043), a build secret the Dockerfile does not
// mount (W064), a cookie Domain (W091), identity read from a header the platform does not set
// (W092) and an outside service Whisk already provides (W093). Each is a warning: the patterns
// read code, they do not run it (doctor-rules.md).

// WakeLimitSeconds is how long a wake waits for the app to answer its health check
// (NODE-AGENT.md §A8).
const WakeLimitSeconds = 60

func w004(r Repo, _ Context) outcome {
	var out outcome
	for i, s := range r.Manifest.Static {
		dir := strings.Trim(path.Clean(filepath.ToSlash(s.Dir)), "/")
		if dir == "" || dir == "." {
			continue
		}
		onDisk := false
		if info, err := os.Stat(filepath.Join(r.Dir, filepath.FromSlash(dir))); err == nil && info.IsDir() {
			onDisk = true
		}
		if why := staticProblem(r.Files, dir, r.IsGit && r.GitOK, onDisk); why != "" {
			out = out.add("W004", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/static/%d/dir", i)), fmt.Sprintf("static folder %s %s; static files are read from the commit, not from the build.", dir, why))
		}
	}
	return out
}

// staticProblem says what is wrong with a static folder for a deploy, or "" when the commit
// will hold files in it: in a git repository a tracked file under it, elsewhere any file. Pure.
func staticProblem(files []File, dir string, git, onDisk bool) string {
	prefix := dir + "/"
	for _, f := range files {
		if strings.HasPrefix(f.Path, prefix) && (f.Tracked || !git) {
			return ""
		}
	}
	switch {
	case git && onDisk:
		return "holds no file git tracks (it is git-ignored or not added)"
	case onDisk:
		return "holds no file that would be committed (it is empty or git-ignored)"
	}
	return "does not exist"
}

func w005(r Repo, _ Context) outcome {
	line := yamlLine(r.ManifestNode, "/health/timeout")
	if !yamlHas(r.ManifestNode, "health", "timeout") || r.Manifest.AlwaysOn || r.Manifest.Health.Timeout <= WakeLimitSeconds {
		return outcome{}
	}
	return one("W005", manifestFile, line, fmt.Sprintf("health.timeout is %d seconds, but a sleeping app has %d seconds to answer when a visitor wakes it.", r.Manifest.Health.Timeout, WakeLimitSeconds))
}

// yamlHas reports whether the manifest writes the key at the path of mapping keys. Pure.
func yamlHas(root *yaml.Node, keys ...string) bool {
	if root == nil {
		return false
	}
	node := root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, k := range keys {
		if node.Kind != yaml.MappingNode {
			return false
		}
		if node = mappingValue(node, k); node == nil {
			return false
		}
	}
	return true
}

// timerPatterns are a timer or scheduler running inside the app.
var timerPatterns = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`\bsetInterval\(`),
		regexp.MustCompile(`(?:from\s+|require\(\s*)["'](node-cron|cron|node-schedule|toad-scheduler|agenda|bree|croner)["']`),
	},
	stack.PY: {
		regexp.MustCompile(`(?m)^\s*(?:from|import)\s+(apscheduler|schedule|crontab|rocketry)\b`),
		regexp.MustCompile(`\bthreading\.Timer\(`),
	},
	stack.GO: {
		regexp.MustCompile(`"(github\.com/robfig/cron(?:/v3)?|github\.com/go-co-op/gocron(?:/v2)?)"`),
		regexp.MustCompile(`\btime\.(?:NewTicker|Tick)\(`),
	},
}

// pageCode reports whether a code file is served to the browser rather than run by the
// server: under a static folder or a public/ or client/ folder. Pure.
func pageCode(p string, static []string) bool {
	for _, dir := range static {
		if dir != "" && strings.HasPrefix(p, strings.Trim(dir, "/")+"/") {
			return true
		}
	}
	return strings.HasPrefix(p, "public/") || strings.Contains(p, "/public/") || strings.HasPrefix(p, "client/") || strings.Contains(p, "/client/")
}

// serverCode is the app's own code that runs on the server, in every language doctor reads.
func serverCode(r Repo) []File {
	static := make([]string, 0, len(r.Manifest.Static))
	for _, s := range r.Manifest.Static {
		static = append(static, s.Dir)
	}
	var out []File
	for _, lang := range []stack.Lang{stack.JS, stack.PY, stack.GO} {
		for _, f := range r.Code(lang) {
			if !pageCode(f.Path, static) {
				out = append(out, f)
			}
		}
	}
	return out
}

func w023(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range serverCode(r) {
		if hs := matches(f, timerPatterns[stack.LangOf(f.Path)], 0); len(hs) > 0 {
			out = out.add("W023", f.Path, hs[0].Line, fmt.Sprintf("%s runs a timer or scheduler inside the app (%s), which stops when the app sleeps and runs once per container.", f.Path, strings.TrimSpace(hs[0].Name)))
		}
	}
	return out
}

// sessionPatterns are database use that needs one connection held across statements, which the
// pooled DATABASE_URL does not give (the pool is transactional).
var sessionPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?i)[\"'`]\\s*LISTEN\\s+\\w"),
	regexp.MustCompile(`(?i)\bpg_(?:try_)?advisory_lock(?:_shared)?\s*\(`),
	regexp.MustCompile("(?i)[\"'`]\\s*SET\\s+(?:SESSION\\s+)?(?:search_path|role|statement_timeout|timezone|time\\s+zone|application_name|lock_timeout|[a-z_]+\\.[a-z_]+)\\b"),
}

func w024(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range serverCode(r) {
		if migrationFile(f.Path) {
			continue // migrate runs on a direct connection, where session state is fine
		}
		if hs := matches(f, sessionPatterns, 0); len(hs) > 0 {
			out = out.add("W024", f.Path, hs[0].Line, fmt.Sprintf("%s uses session state on the database connection (%s), which the pooled connection does not keep between statements.", f.Path, strings.Trim(strings.TrimSpace(hs[0].Name), "\"'`")))
		}
	}
	return out
}

// migrationFile reports whether a path is a migration or the migrate command's script, which
// runs on the direct connection rather than the pool. Pure.
func migrationFile(p string) bool {
	return strings.Contains(strings.ToLower(p), "migrat") || strings.HasPrefix(p, "alembic/") || strings.Contains(p, "/alembic/")
}

// functionIDPatterns capture the id a function is registered with, per language's SDK.
var functionIDPatterns = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {regexp.MustCompile(`(?s)createFunction\(\s*\{.{0,300}?\bid\s*:\s*["'` + "`" + `]([^"'` + "`" + `]+)["'` + "`" + `]`)},
	stack.PY: {regexp.MustCompile(`(?s)create_function\(.{0,300}?\bfn_id\s*=\s*["']([^"']+)["']`)},
	stack.GO: {regexp.MustCompile(`(?s)FunctionOpts\{.{0,300}?\bID\s*:\s*"([^"]+)"`)},
}

func w043(r Repo, _ Context) outcome {
	if len(r.Manifest.Functions) == 0 {
		return outcome{}
	}
	ids := map[string]bool{}
	for _, f := range serverCode(r) {
		for _, h := range matches(f, functionIDPatterns[stack.LangOf(f.Path)], 1) {
			ids[h.Name] = true
		}
	}
	if len(ids) == 0 {
		return skip("W043", "no function id was found in code (createFunction({ id }), create_function(fn_id=), FunctionOpts{ID:})")
	}
	var out outcome
	for i, fn := range r.Manifest.Functions {
		if !ids[fn.Name] {
			out = out.add("W043", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/functions/%d/name", i)), fmt.Sprintf("Function %s is declared in whisk.yaml, but no function in code has that id (found: %s), so it is never registered.", fn.Name, strings.Join(sortedKeys(ids), ", ")))
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func w064(r Repo, _ Context) outcome {
	if len(r.Manifest.Build.Secrets) == 0 {
		return outcome{}
	}
	names := dockerfiles(r)
	if len(names) == 0 {
		return skip("W064", "no Dockerfile; the build uses Railpack, which reads build secrets itself")
	}
	src := string(r.Read(names[0]))
	var out outcome
	for i, name := range r.Manifest.Build.Secrets {
		if !mountsSecret(src, name) {
			out = out.add("W064", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/build/secrets/%d", i)), fmt.Sprintf("Build secret %s is not mounted by any RUN in %s, so the build never sees it.", name, names[0]))
		}
	}
	return out
}

// mountsSecret reports whether a Dockerfile mounts the named build secret: a
// --mount=type=secret with id=<name> (in either order, or with env=<name>). Pure.
func mountsSecret(dockerfile, name string) bool {
	for _, m := range reSecretMount.FindAllString(dockerfile, -1) {
		if regexp.MustCompile(`\b(?:id|env)=` + regexp.QuoteMeta(name) + `\b`).MatchString(m) {
			return true
		}
	}
	return false
}

var reSecretMount = regexp.MustCompile(`--mount=\S*type=secret\S*`)

// cookieDomainPatterns set a cookie's Domain attribute.
var cookieDomainPatterns = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`(?i);\s*domain=`),
		regexp.MustCompile(`(?i)(?:setCookie|cookie|cookies\.set|serialize)\([^)]*\bdomain\s*:`),
	},
	stack.PY: {
		regexp.MustCompile(`(?i);\s*domain=`),
		regexp.MustCompile(`set_cookie\([^)]*\bdomain\s*=`),
	},
	stack.GO: {
		regexp.MustCompile(`(?i);\s*domain=`),
		regexp.MustCompile(`http\.Cookie\{[^}]*\bDomain\s*:`),
	},
}

func w091(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range serverCode(r) {
		if hs := matches(f, cookieDomainPatterns[stack.LangOf(f.Path)], 0); len(hs) > 0 {
			out = out.add("W091", f.Path, hs[0].Line, fmt.Sprintf("%s sets a cookie with a Domain attribute, which the platform drops; app cookies are bound to the app's own address.", f.Path))
		}
	}
	return out
}

// identityPatterns read who the caller is from something the platform does not vouch for: the
// Authorization header of the incoming request, or a token the app verifies itself.
var identityPatterns = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`(?i)\.header\(\s*["']authorization["']\s*\)`),
		regexp.MustCompile(`(?i)\bheaders\.authorization\b`),
		regexp.MustCompile(`(?i)\bheaders\[\s*["']authorization["']\s*\]`),
		regexp.MustCompile(`(?i)\b(?:req|request|c\.req)\.headers\.get\(\s*["']authorization["']\s*\)`),
		regexp.MustCompile(`\bjwt\.verify\(|\bjwtVerify\(`),
	},
	stack.PY: {
		regexp.MustCompile(`(?i)\brequest\.headers\.get\(\s*["']authorization["']`),
		regexp.MustCompile(`(?i)\brequest\.headers\[\s*["']authorization["']\s*\]`),
		regexp.MustCompile(`(?i)\bHTTP_AUTHORIZATION\b`),
		regexp.MustCompile(`\bjwt\.decode\(`),
	},
	stack.GO: {
		regexp.MustCompile(`(?i)\b(?:r|req|request)\.Header\.Get\(\s*"authorization"\s*\)`),
		regexp.MustCompile(`\bjwt\.Parse(?:WithClaims)?\(`),
	},
}

func w092(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range serverCode(r) {
		for _, h := range matches(f, identityPatterns[stack.LangOf(f.Path)], 0) {
			if assigns(f.Content, h) {
				continue
			}
			out = out.add("W092", f.Path, h.Line, fmt.Sprintf("%s reads who the caller is from %s; the platform's X-Whisk-* headers are the only identity it vouches for.", f.Path, strings.TrimSpace(h.Name)))
			break
		}
	}
	return out
}

// assigns reports whether the match is the left side of an assignment (headers["Authorization"]
// = ...), which sets an outgoing header rather than reading the caller's. Pure.
func assigns(content []byte, h hit) bool {
	lines := strings.Split(string(content), "\n")
	if h.Line < 1 || h.Line > len(lines) {
		return false
	}
	line := lines[h.Line-1]
	i := strings.Index(line, h.Name)
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(line[i+len(h.Name):])
	return strings.HasPrefix(rest, "=") && !strings.HasPrefix(rest, "==")
}

// outsideServices are libraries for something Whisk already provides, by the import that
// names them, with the built-in to use instead.
var outsideServices = []struct {
	lang     stack.Lang
	pattern  *regexp.Regexp
	instead  string
	category string
}{
	{stack.JS, regexp.MustCompile(`(?:from\s+|require\(\s*|import\s+)["'](@auth0/[\w.-]+|auth0|@clerk/[\w.-]+|next-auth|@auth/core|@supabase/auth-js|@supabase/auth-helpers-[\w-]+|firebase/auth|lucia)["']`), "the platform's login and X-Whisk-* headers (skill §4)", "sign-in"},
	{stack.JS, regexp.MustCompile(`(?:from\s+|require\(\s*|import\s+)["'](nodemailer|@sendgrid/mail|resend|mailgun\.js|mailgun-js|postmark|@aws-sdk/client-ses)["']`), "email: true (skill §9)", "email"},
	{stack.JS, regexp.MustCompile(`(?:from\s+|require\(\s*|import\s+)["'](@upstash/redis|@upstash/ratelimit)["']`), "kv: true (skill §9)", "cache"},
	{stack.PY, regexp.MustCompile(`(?m)^\s*(?:from|import)\s+(auth0|clerk_backend_api|flask_login|allauth)\b`), "the platform's login and X-Whisk-* headers (skill §4)", "sign-in"},
	{stack.PY, regexp.MustCompile(`(?m)^\s*(?:from|import)\s+(smtplib|sendgrid|resend|postmarker|mailgun)\b`), "email: true (skill §9)", "email"},
	{stack.PY, regexp.MustCompile(`(?m)^\s*(?:from|import)\s+(upstash_redis)\b`), "kv: true (skill §9)", "cache"},
	{stack.GO, regexp.MustCompile(`"(github\.com/clerk/[\w./-]+|github\.com/auth0/[\w./-]+|github\.com/markbates/goth)"`), "the platform's login and X-Whisk-* headers (skill §4)", "sign-in"},
	{stack.GO, regexp.MustCompile(`"(net/smtp|github\.com/sendgrid/[\w./-]+|github\.com/resend/[\w./-]+|github\.com/wneessen/go-mail|gopkg\.in/gomail\.v2)"`), "email: true (skill §9)", "email"},
	{stack.GO, regexp.MustCompile(`"(github\.com/upstash/[\w./-]+)"`), "kv: true (skill §9)", "cache"},
}

func w093(r Repo, _ Context) outcome {
	var out outcome
	seen := map[string]bool{}
	for _, f := range serverCode(r) {
		lang := stack.LangOf(f.Path)
		for _, svc := range outsideServices {
			if svc.lang != lang {
				continue
			}
			for _, h := range matches(f, []*regexp.Regexp{svc.pattern}, 1) {
				if seen[h.Name] {
					continue
				}
				seen[h.Name] = true
				out = out.add("W093", f.Path, h.Line, fmt.Sprintf("%s uses %s for %s, which Whisk already provides: use %s.", f.Path, h.Name, svc.category, svc.instead))
			}
		}
	}
	return out
}

// customerScopePatterns are the signs that an app tells customers apart: a call to the template
// helpers, or a comparison of the audience with "customer" (W094).
var customerScopePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:scopeFor|canSee|canChange|scope_for|can_see|can_change|ScopeFor)\(`),
	regexp.MustCompile(`\.(?:CanSee|CanChange)\(`),
	regexp.MustCompile(`(?:===?|!==?)\s*["']customer["']|["']customer["']\s*(?:===?|!==?)`),
}

// helperModule reports whether a file is the template's own whisk module, whose helpers call
// each other and so say nothing about whether the app uses them. Pure.
func helperModule(p string) bool {
	switch path.Base(p) {
	case "whisk.ts", "whisk.js", "whisk.py", "whisk.go":
		return true
	}
	return false
}

func w094(r Repo, _ Context) outcome {
	if r.Manifest.CustomerIdentity != "app" && r.Manifest.CustomerIdentity != "org" {
		return outcome{}
	}
	for _, f := range serverCode(r) {
		if !helperModule(f.Path) && len(matches(f, customerScopePatterns, 0)) > 0 {
			return outcome{}
		}
	}
	return one("W094", manifestFile, yamlLine(r.ManifestNode, "/customer_identity"), fmt.Sprintf("customer_identity is %s, but no code limits what a customer sees to the signed-in person, so one customer may read another's records.", r.Manifest.CustomerIdentity))
}
