package doctor

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/stack"
)

// Start-up rules (doctor-rules.md W100 to W103). A sleeping app makes its visitor wait for it to
// start, so what an app does when it starts is read here: the measured start the platform
// recorded, and three causes of a slow one that can be seen in the repository.

// SlowStartMS is the typical start past which W100 warns. Most apps start in about a second.
const SlowStartMS = 3000

// startCommand is one place the app's start command is written: a Dockerfile's CMD or
// ENTRYPOINT, a package.json script it runs, or a line of a shell script it runs.
type startCommand struct {
	File string
	Line int
	Text string
}

// startCommands is what runs when the app's container starts: with a Dockerfile, its final
// stage's CMD and ENTRYPOINT; without one, package.json's start script (what Railpack runs).
// A package script or a shell script the command runs is followed, up to three levels. Pure.
func startCommands(r Repo) []startCommand {
	var roots []startCommand
	if names := dockerfiles(r); len(names) > 0 {
		roots = finalStartInstructions(names[0], instructionLines(r.Read(names[0])))
	} else {
		scripts, lines := packageScripts(r.Read("package.json"))
		for _, name := range []string{"prestart", "start"} {
			if text, ok := scripts[name]; ok {
				roots = append(roots, startCommand{File: "package.json", Line: lines[name], Text: text})
			}
		}
	}
	seen := map[string]bool{}
	var out []startCommand
	var walk func(c startCommand, depth int)
	walk = func(c startCommand, depth int) {
		key := fmt.Sprintf("%s:%d:%s", c.File, c.Line, c.Text)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, c)
		if depth >= 3 {
			return
		}
		for _, next := range followed(r, c.Text) {
			walk(next, depth+1)
		}
	}
	for _, c := range roots {
		walk(c, 0)
	}
	return out
}

// finalStartInstructions is the last CMD and the last ENTRYPOINT of a Dockerfile's final stage,
// in file order.
func finalStartInstructions(file string, ins []instruction) []startCommand {
	start := 0
	for i, in := range ins {
		if strings.HasPrefix(in.Text, "FROM ") {
			start = i
		}
	}
	var cmd, entry *instruction
	for i := start; i < len(ins); i++ {
		switch strings.SplitN(ins[i].Text, " ", 2)[0] {
		case "CMD":
			cmd = &ins[i]
		case "ENTRYPOINT":
			entry = &ins[i]
		}
	}
	var out []startCommand
	for _, in := range []*instruction{entry, cmd} {
		if in != nil {
			out = append(out, startCommand{File: file, Line: in.Line, Text: commandWords(in.Text)})
		}
	}
	if len(out) == 2 && out[0].Line > out[1].Line {
		out[0], out[1] = out[1], out[0]
	}
	return out
}

// commandWords is a CMD or ENTRYPOINT's command as a shell would read it: the exec form's JSON
// array joined with spaces (["npm", "start"] is npm start), the shell form as written.
func commandWords(instr string) string {
	_, rest, _ := strings.Cut(instr, " ")
	rest = strings.TrimSpace(rest)
	var args []string
	if strings.HasPrefix(rest, "[") && json.Unmarshal([]byte(rest), &args) == nil {
		return strings.Join(args, " ")
	}
	return rest
}

var (
	rePackageRun = regexp.MustCompile(`\b(npm|pnpm|yarn|bun)\s+(?:run(?:-script)?\s+)?([A-Za-z0-9_:.-]+)`)
	reShellFile  = regexp.MustCompile(`[A-Za-z0-9_./-]+\.sh\b`)
)

// followed is what a command runs that the repository holds: a package.json script (with its
// pre script) and a shell script, one start command per line of it.
func followed(r Repo, text string) []startCommand {
	var out []startCommand
	scripts, lines := packageScripts(r.Read("package.json"))
	for _, m := range rePackageRun.FindAllStringSubmatch(text, -1) {
		name := m[2]
		if m[1] == "npm" && !strings.Contains(m[0], " run") && name != "start" && name != "test" {
			continue // npm runs only these without "run"; npm ci, npm install are its own.
		}
		for _, n := range []string{"pre" + name, name} {
			if s, ok := scripts[n]; ok {
				out = append(out, startCommand{File: "package.json", Line: lines[n], Text: s})
			}
		}
	}
	for _, p := range reShellFile.FindAllString(text, -1) {
		file, ok := repoPath(r, p)
		if !ok {
			continue
		}
		for i, line := range strings.Split(string(r.Read(file)), "\n") {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			out = append(out, startCommand{File: file, Line: i + 1, Text: t})
		}
	}
	return out
}

// repoPath finds the repository file a path in a command names: relative as written, or an
// absolute path in the image (/app/scripts/start.sh) by its trailing parts.
func repoPath(r Repo, p string) (string, bool) {
	p = strings.TrimPrefix(p, "./")
	if !strings.HasPrefix(p, "/") {
		p = path.Clean(p)
		return p, r.Read(p) != nil
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i := range parts {
		cand := strings.Join(parts[i:], "/")
		if r.Read(cand) != nil {
			return cand, true
		}
	}
	return "", false
}

var reScriptKey = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*:`)

// packageScripts reads package.json's scripts and the line each is on. Pure.
func packageScripts(src []byte) (map[string]string, map[string]int) {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if src == nil || json.Unmarshal(src, &pkg) != nil || len(pkg.Scripts) == 0 {
		return nil, nil
	}
	lines := map[string]int{}
	from := strings.Index(string(src), `"scripts"`)
	if from < 0 {
		return pkg.Scripts, lines
	}
	for _, m := range reScriptKey.FindAllSubmatchIndex(src[from:], -1) {
		name := string(src[from+m[2] : from+m[3]])
		if _, ok := pkg.Scripts[name]; ok {
			if _, done := lines[name]; !done {
				lines[name] = lineAt(src, from+m[0])
			}
		}
	}
	return pkg.Scripts, lines
}

// tsAtStart finds a tool that compiles TypeScript as the app starts, written as a command word
// (tsx, ts-node, babel-node, node --import tsx, node -r ts-node/register). A file name ending
// .tsx is not one.
var tsAtStart = regexp.MustCompile(`(?:^|[\s"'\[,=;&|(])(tsx|ts-node(?:-dev|-esm)?|babel-node)(?:$|[\s"',\]/;&|)])`)

// migrateCommands are migration tools run as a command. Each names the tool and its migrate
// verb, so a word like "migrate" alone in a command is never enough.
var migrateCommands = []*regexp.Regexp{
	regexp.MustCompile(`\bprisma\s+(?:migrate\s+deploy|db\s+push)\b`),
	regexp.MustCompile(`\bdrizzle-kit\s+(?:migrate|push)\b`),
	regexp.MustCompile(`\bknex\s+migrate:(?:latest|up)\b`),
	regexp.MustCompile(`\bsequelize(?:-cli)?\s+db:migrate\b`),
	regexp.MustCompile(`\btypeorm[\w-]*\s+migration:run\b`),
	regexp.MustCompile(`\balembic\b[^&|;\n]*\supgrade\b`),
	regexp.MustCompile(`["']alembic["']\s*,\s*["']upgrade["']`),
	regexp.MustCompile(`\bmanage\.py\s+migrate\b|\bdjango-admin\s+migrate\b`),
	regexp.MustCompile(`\bgoose\s(?:[^&|;\n]*\s)?up(?:$|[\s"'])`),
	regexp.MustCompile(`(?:^|[\s"'/])migrate\s+-(?:path|source|database)\b[^&|;\n]*\sup(?:$|[\s"'])`),
	regexp.MustCompile(`\b(?:npm|pnpm|yarn|bun)\s+(?:run(?:-script)?\s+)?[\w:.-]*migrat[\w:.-]*`),
	regexp.MustCompile(`\b(?:node|tsx|ts-node|python3?|bun|deno\s+run)\s+(?:-\S+\s+)*[\w./-]*migrat[\w.-]*\.(?:m?[jt]s|c[jt]s|py)\b`),
}

// migrateCode is the app's own code migrating as it runs: Drizzle's migrator, Knex, Kysely and
// Umzug in TypeScript and JavaScript; Alembic's command API and Django's call_command in Python.
var migrateCode = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`\bmigrate\(\s*\w+\s*,\s*\{\s*migrationsFolder\b`),
		regexp.MustCompile(`\.migrate\.latest\(`),
		regexp.MustCompile(`\.migrateToLatest\(`),
		regexp.MustCompile(`\bumzug\.up\(`),
	},
	stack.PY: {
		regexp.MustCompile(`\bcommand\.upgrade\(`),
		regexp.MustCompile(`\bcall_command\(\s*["']migrate["']`),
	},
}

// entryFiles are the app's own code files that run first when it starts: the files its start
// command names (dist/x.js read as src/x.ts, uvicorn app.main:app as app/main.py), package.json's
// main, and the conventional entry names. Tests and vendored code are never entries. Pure.
func entryFiles(r Repo, cmds []startCommand) []File {
	var names []string
	for _, c := range cmds {
		names = append(names, commandFiles(c.Text)...)
	}
	var pkg struct {
		Main string `json:"main"`
	}
	if src := r.Read("package.json"); src != nil && json.Unmarshal(src, &pkg) == nil && pkg.Main != "" {
		names = append(names, pkg.Main)
	}
	names = append(names, conventionalEntries...)
	code := map[string]File{}
	for _, f := range r.AllCode() {
		code[f.Path] = f
	}
	seen := map[string]bool{}
	var out []File
	for _, n := range names {
		for _, cand := range sourceCandidates(n) {
			if f, ok := code[cand]; ok && !seen[cand] && stack.LangOf(cand) != stack.GO {
				seen[cand] = true
				out = append(out, f)
			}
		}
	}
	return out
}

var conventionalEntries = []string{
	"src/index.ts", "src/index.js", "src/index.mjs", "src/server.ts", "src/server.js", "src/main.ts", "src/main.js",
	"index.ts", "index.js", "index.mjs", "server.ts", "server.js", "server.mjs", "main.ts", "main.js",
	"main.py", "app.py", "server.py", "app/main.py", "src/main.py", "wsgi.py", "asgi.py",
}

var (
	reCodePath  = regexp.MustCompile(`[\w@./-]+\.(?:m?[jt]sx?|c[jt]s|py)\b`)
	reASGIApp   = regexp.MustCompile(`\b(?:uvicorn|gunicorn|hypercorn|granian)\b[^&|;]*?\s["']?([A-Za-z_][\w.]*):[A-Za-z_]\w*`)
	rePyModule  = regexp.MustCompile(`\bpython3?\s+(?:-\S+\s+)*-m\s+([A-Za-z_][\w.]*)`)
	buildOutDir = regexp.MustCompile(`^(?:dist|build|out|\.output)/`)
)

// commandFiles are the code files a start command names, as written.
func commandFiles(text string) []string {
	var out []string
	for _, p := range reCodePath.FindAllString(text, -1) {
		if strings.Contains(p, "migrat") {
			continue // a migrate script run before the app is not its entry.
		}
		out = append(out, p)
	}
	for _, m := range reASGIApp.FindAllStringSubmatch(text, -1) {
		out = append(out, strings.ReplaceAll(m[1], ".", "/")+".py")
	}
	for _, m := range rePyModule.FindAllStringSubmatch(text, -1) {
		mod := strings.ReplaceAll(m[1], ".", "/")
		out = append(out, mod+".py", mod+"/__main__.py")
	}
	return out
}

// sourceCandidates are the repository paths a file a command names may be: as written, and
// for built JavaScript (dist/index.js) the source it was built from (src/index.ts).
func sourceCandidates(p string) []string {
	p = strings.TrimPrefix(p, "./")
	for _, prefix := range []string{"/app/", "/srv/", "/usr/src/app/", "/"} {
		p = strings.TrimPrefix(p, prefix)
	}
	out := []string{p}
	ext := path.Ext(p)
	if buildOutDir.MatchString(p) && (ext == ".js" || ext == ".mjs" || ext == ".cjs") {
		base := strings.TrimSuffix(buildOutDir.ReplaceAllString(p, ""), ext)
		for _, dir := range []string{"src/", ""} {
			for _, e := range []string{".ts", ".mts", ".js", ".mjs"} {
				out = append(out, dir+base+e)
			}
		}
	}
	return out
}

// heavyJS and heavyPython are libraries that take a second or more to load, so loading them
// when the app starts slows every start. Kept short so a finding is worth acting on.
var heavyJS = map[string]bool{
	"aws-sdk": true, "googleapis": true, "firebase-admin": true, "puppeteer": true, "playwright": true,
	"@tensorflow/tfjs-node": true, "@tensorflow/tfjs": true, "@huggingface/transformers": true, "@xenova/transformers": true,
}

var heavyPython = map[string]bool{
	"pandas": true, "torch": true, "tensorflow": true, "transformers": true, "sklearn": true,
	"scipy": true, "matplotlib": true, "spacy": true,
}

var (
	reJSImportFrom = regexp.MustCompile(`(?m)^import\s+([^;'"]*?)\s*from\s*["']([^"']+)["']`)
	reJSImportBare = regexp.MustCompile(`(?m)^import\s*["']([^"']+)["']`)
	reJSRequire    = regexp.MustCompile(`(?m)^(?:const|let|var)\s+[^=;]+=\s*require\(\s*["']([^"']+)["']\s*\)`)
	rePyImport     = regexp.MustCompile(`(?m)^import\s+([\w.]+(?:\s+as\s+\w+)?(?:\s*,\s*[\w.]+(?:\s+as\s+\w+)?)*)`)
	rePyFrom       = regexp.MustCompile(`(?m)^from\s+([\w.]+)\s+import\b`)
)

// heavyImport is the first heavy library a file loads at its top level: a static import or a
// top-level require in TypeScript and JavaScript (not import type), an import at column 0 in
// Python. An import inside a function loads when the function first runs, which is the fix.
func heavyImport(f File) (hit, bool) {
	src := blankComments(stack.LangOf(f.Path), f.Content)
	best := hit{Line: -1}
	offset := -1
	consider := func(pkg string, at int) {
		if offset < 0 || at < offset {
			offset = at
			best = hit{Name: pkg, File: f.Path, Line: lineAt(src, at)}
		}
	}
	switch stack.LangOf(f.Path) {
	case stack.JS:
		for _, m := range reJSImportFrom.FindAllSubmatchIndex(src, -1) {
			clause, pkg := string(src[m[2]:m[3]]), string(src[m[4]:m[5]])
			if heavyJS[pkg] && clause != "type" && !strings.HasPrefix(clause, "type ") && !strings.HasPrefix(clause, "type{") {
				consider(pkg, m[0])
			}
		}
		for _, re := range []*regexp.Regexp{reJSImportBare, reJSRequire} {
			for _, m := range re.FindAllSubmatchIndex(src, -1) {
				if pkg := string(src[m[2]:m[3]]); heavyJS[pkg] {
					consider(pkg, m[0])
				}
			}
		}
	case stack.PY:
		for _, m := range rePyImport.FindAllSubmatchIndex(src, -1) {
			for _, part := range strings.Split(string(src[m[2]:m[3]]), ",") {
				mod := strings.Fields(strings.TrimSpace(part))
				if len(mod) > 0 && heavyPython[strings.SplitN(mod[0], ".", 2)[0]] {
					consider(strings.SplitN(mod[0], ".", 2)[0], m[0])
				}
			}
		}
		for _, m := range rePyFrom.FindAllSubmatchIndex(src, -1) {
			if top := strings.SplitN(string(src[m[2]:m[3]]), ".", 2)[0]; heavyPython[top] {
				consider(top, m[0])
			}
		}
	}
	return best, offset >= 0
}

func w100(r Repo, c Context) outcome {
	switch {
	case r.Binding == nil:
		return skip("W100", "the directory is not bound to an app, so no start times are known")
	case c.Validation == nil:
		return skip("W100", "the platform was not asked, so no start times are known")
	case c.Validation.Start == nil || c.Validation.Start.Count == 0:
		return skip("W100", "the platform has recorded no starts of this app yet")
	}
	s := *c.Validation.Start
	if s.TypicalMS <= SlowStartMS {
		return outcome{}
	}
	of := fmt.Sprintf("typical of the last %d starts", s.Count)
	if s.Count == 1 {
		of = "the one start recorded"
	}
	wait := "Visitors wait that long after it sleeps."
	if r.ManifestErr == nil && r.Manifest.AlwaysOn {
		wait = "Every deploy and restart waits that long."
	}
	return one("W100", "", 0, fmt.Sprintf("%s takes %s to start (%s). %s", r.Binding.App, output.Seconds(s.TypicalMS), of, wait))
}

func w101(r Repo, _ Context) outcome {
	cmds := startCommands(r)
	if len(cmds) == 0 {
		return skip("W101", "no start command was found (a Dockerfile CMD or ENTRYPOINT, or package.json's start script)")
	}
	var out outcome
	reported := map[string]bool{}
	for _, c := range cmds {
		m := tsAtStart.FindStringSubmatch(c.Text)
		if m == nil || reported[c.File] {
			continue
		}
		reported[c.File] = true
		out = out.add("W101", c.File, c.Line, fmt.Sprintf("The app starts with %s, which compiles TypeScript every time it starts.", m[1]))
	}
	return out
}

func w102(r Repo, _ Context) outcome {
	cmds := startCommands(r)
	entries := entryFiles(r, cmds)
	if len(cmds) == 0 && len(entries) == 0 {
		return skip("W102", "no start command or entry file was found")
	}
	where := "whisk.yaml migrate runs them once per deploy instead"
	if r.ManifestErr == nil && r.Manifest.Migrate != "" {
		where = "whisk.yaml migrate already runs them once per deploy"
	}
	var out outcome
	reported := map[string]bool{}
	for _, c := range cmds {
		if reported[c.File] {
			continue
		}
		for _, re := range migrateCommands {
			if m := re.FindString(c.Text); m != "" {
				reported[c.File] = true
				out = out.add("W102", c.File, c.Line, fmt.Sprintf("The start command runs migrations (%s) every time the app starts; %s.", strings.Trim(strings.TrimSpace(m), `"'`), where))
				break
			}
		}
	}
	for _, f := range entries {
		if reported[f.Path] {
			continue
		}
		lang := stack.LangOf(f.Path)
		src := blankComments(lang, f.Content)
		pats := append(append([]*regexp.Regexp{}, migrateCode[lang]...), migrateCommands...)
		first := -1
		for _, re := range pats {
			if loc := re.FindIndex(src); loc != nil && (first < 0 || loc[0] < first) {
				first = loc[0]
			}
		}
		if first >= 0 {
			reported[f.Path] = true
			out = out.add("W102", f.Path, lineAt(src, first), fmt.Sprintf("%s runs migrations when the app starts; %s.", f.Path, where))
		}
	}
	return out
}

func w103(r Repo, _ Context) outcome {
	entries := entryFiles(r, startCommands(r))
	if len(entries) == 0 {
		return skip("W103", "no entry file was found (the file the start command runs, package.json main, or src/index.ts, main.py and the like)")
	}
	var out outcome
	for _, f := range entries {
		if h, ok := heavyImport(f); ok {
			out = out.add("W103", h.File, h.Line, fmt.Sprintf("%s loads %s when the app starts; it is a large library and slows every start.", h.File, h.Name))
		}
	}
	return out
}

// blankComments is stripComments that keeps every line where it was, so a line number read from
// the result is the file's own: each comment becomes the line breaks it held. Pure.
func blankComments(lang stack.Lang, content []byte) []byte {
	keep := func(m []byte) []byte { return []byte(strings.Repeat("\n", strings.Count(string(m), "\n"))) }
	if lang == stack.PY {
		return rePyComment.ReplaceAllFunc(content, keep)
	}
	return reJSComment.ReplaceAllFunc(content, keep)
}

var (
	rePyComment = regexp.MustCompile(`(?m)^[ \t]*#.*$`)
	reJSComment = regexp.MustCompile(`(?s)/\*.*?\*/|(?m)^[ \t]*//[^\n]*|[ \t]//\s[^\n]*`)
)

// reNodeFile is a start command running a JavaScript file with node: node dist/index.js,
// node --enable-source-maps server.mjs.
var reNodeFile = regexp.MustCompile(`(?:^|[\s"'/])node\s+(?:-[\w-]+(?:=\S+)?\s+)*[\w@./-]+\.(?:m?js|cjs)\b`)

// reBundler is a build that bundles the app into one file: esbuild, tsup, rollup, webpack,
// ncc, bun build, or vite build for the server (--ssr).
var reBundler = regexp.MustCompile(`\b(?:esbuild|tsup|rollup|webpack|ncc)\b|\bbun\s+build\b|\bvite\s+build\b[^&|;\n]*--ssr\b`)

// reInstall is a package install in a Dockerfile RUN.
var reInstall = regexp.MustCompile(`\b(?:npm\s+(?:ci|install|i)|yarn(?:\s+install)?|pnpm\s+(?:install|i)|bun\s+install)\b`)

// bundlerConfigs are config files only a bundling build has.
var bundlerConfigs = regexp.MustCompile(`^(?:esbuild|tsup|rollup|webpack)\.config\.[cm]?[jt]s$`)

// reScriptFile is a script file a package.json script runs with node: node build.mjs.
var reScriptFile = regexp.MustCompile(`\bnode\s+(?:-[\w-]+(?:=\S+)?\s+)*([\w./-]+\.(?:m?js|cjs))\b`)

// reBundlerImport is a build script loading a bundler's API: import { build } from "esbuild".
var reBundlerImport = regexp.MustCompile(`["'](?:esbuild|rollup|tsup|webpack|@vercel/ncc)["']`)

// bundles reports whether the build bundles the app: a package.json script or a Dockerfile RUN
// that runs a bundler, a script file a package.json script runs that loads a bundler, or a
// bundler's config file at the root. Pure.
func bundles(r Repo) bool {
	scripts, _ := packageScripts(r.Read("package.json"))
	for _, s := range scripts {
		if reBundler.MatchString(s) {
			return true
		}
		for _, m := range reScriptFile.FindAllStringSubmatch(s, -1) {
			if src := r.Read(strings.TrimPrefix(m[1], "./")); src != nil && reBundlerImport.Match(src) {
				return true
			}
		}
	}
	for _, name := range dockerfiles(r) {
		for _, in := range instructionLines(r.Read(name)) {
			if strings.HasPrefix(in.Text, "RUN ") && reBundler.MatchString(in.Text) {
				return true
			}
		}
	}
	for _, f := range r.Files {
		if bundlerConfigs.MatchString(f.Path) {
			return true
		}
	}
	return false
}

// shipsNodeModules reports whether the running image holds node_modules: a Dockerfile whose
// final stage copies node_modules in or installs packages, or, without a Dockerfile, a
// package.json with dependencies (the build installs them). Pure.
func shipsNodeModules(r Repo) bool {
	names := dockerfiles(r)
	if len(names) == 0 {
		var pkg struct {
			Dependencies map[string]string `json:"dependencies"`
		}
		src := r.Read("package.json")
		return src != nil && json.Unmarshal(src, &pkg) == nil && len(pkg.Dependencies) > 0
	}
	ins := instructionLines(r.Read(names[0]))
	start := 0
	for i, in := range ins {
		if strings.HasPrefix(in.Text, "FROM ") {
			start = i
		}
	}
	for _, in := range ins[start:] {
		switch strings.SplitN(in.Text, " ", 2)[0] {
		case "COPY", "ADD":
			if strings.Contains(in.Text, "node_modules") {
				return true
			}
		case "RUN":
			if reInstall.MatchString(in.Text) {
				return true
			}
		}
	}
	return false
}

func w104(r Repo, _ Context) outcome {
	if r.Read("package.json") == nil {
		return outcome{}
	}
	var at *startCommand
	for _, c := range startCommands(r) {
		if reNodeFile.MatchString(c.Text) {
			c := c
			at = &c
			break
		}
	}
	if at == nil || !shipsNodeModules(r) || bundles(r) {
		return outcome{}
	}
	name := "The app"
	if r.ManifestErr == nil && r.Manifest.Name != "" {
		name = r.Manifest.Name
	}
	return one("W104", at.File, at.Line, name+" loads its libraries file by file at start, which is slow inside Whisk's sandbox.")
}
