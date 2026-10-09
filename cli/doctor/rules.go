package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract"
	rules "github.com/whisk-run/contract/doctor"
	"github.com/whisk-run/contract/graph"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/routes"
	"github.com/whisk-run/contract/run"
)

// Finding is the contract's finding shape.
type Finding = rules.Finding

// Context is what only the platform can answer, when doctor could ask it.
type Context struct {
	Validation *api.Validation
	// Packages is the app's newest package scan, when the platform was asked (W063).
	Packages *api.Packages
	// LockCheck is the platform's check of the local lockfiles, when it was asked (W063).
	LockCheck *api.PackageCheck
	// BoundGitURL is the git_url of the app .whisk/app.json names, when the platform was
	// asked (W006).
	BoundGitURL string
	RepoLimit   int64 // bytes in force for W070
	Ctx         context.Context
}

// FreeRepoLimit is the repository size the Free plan allows, assumed when the directory is
// not bound to an app or the platform cannot be reached.
const FreeRepoLimit = 100 << 20

type outcome struct {
	findings []Finding
	skipped  []string
}

func one(id, file string, line int, message string) outcome {
	return outcome{findings: []Finding{rules.NewFinding(id, file, line, message)}}
}

func skip(id, why string) outcome { return outcome{skipped: []string{id + ": skipped, " + why}} }

func (o outcome) add(id, file string, line int, message string) outcome {
	o.findings = append(o.findings, rules.NewFinding(id, file, line, message))
	return o
}

type rule struct {
	id            string
	needsManifest bool
	run           func(Repo, Context) outcome
}

// ruleTable is every rule in the order doctor reports them.
var ruleTable = []rule{
	{"W001", false, w001},
	{"W002", false, w002},
	{"W003", true, w003},
	{"W004", true, w004},
	{"W005", true, w005},
	{"W006", false, w006},
	{"W010", false, w010},
	{"W011", false, w011},
	{"W020", false, w020},
	{"W021", false, w021},
	{"W022", true, w022},
	{"W023", false, w023},
	{"W024", false, w024},
	{"W030", true, w030},
	{"W031", true, w031},
	{"W040", true, w040},
	{"W041", true, w041},
	{"W042", true, w042},
	{"W043", true, w043},
	{"W050", true, w050},
	{"W052", true, w052},
	{"W053", true, w053},
	{"W060", false, w060},
	{"W061", false, w061},
	{"W062", true, w062},
	{"W063", false, w063},
	{"W064", true, w064},
	{"W070", false, w070},
	{"W080", true, w080},
	{"W090", false, w090},
	{"W091", false, w091},
	{"W092", false, w092},
	{"W093", false, w093},
	{"W094", false, w094},
	{"W095", true, w095},
	{"W100", false, w100},
	{"W101", false, w101},
	{"W102", false, w102},
	{"W103", false, w103},
	{"W104", false, w104},
}

const manifestFile = "whisk.yaml"

func w001(r Repo, _ Context) outcome {
	if !r.HasManifest {
		msg := "whisk.yaml is missing from the repository root."
		for _, f := range r.Files {
			if f.Path == manifestFile && f.Binary {
				return one("W001", manifestFile, 0, "whisk.yaml is there but is not text this can read: save it as UTF-8 (in PowerShell, Set-Content -Encoding utf8, or write it from the editor as UTF-8).")
			}
			base := path.Base(f.Path)
			if (base == "whisk.yaml" || base == "whisk.yml") && f.Path != manifestFile {
				msg += " Found " + f.Path + " instead; the file must be exactly whisk.yaml at the root."
			}
		}
		return one("W001", "", 0, msg)
	}
	for _, f := range r.Files {
		if f.Path == manifestFile && f.UTF16 {
			return one("W001", manifestFile, 0, "whisk.yaml is saved as UTF-16, and the platform reads it as UTF-8: save it as UTF-8 (in PowerShell, Set-Content -Encoding utf8, or choose UTF-8 in the editor).")
		}
	}
	if r.ManifestNode == nil {
		line := 0
		msg := "whisk.yaml does not parse as YAML."
		if r.ManifestErr != nil {
			msg = "whisk.yaml does not parse as YAML: " + r.ManifestErr.Error()
			if m := regexp.MustCompile(`line (\d+)`).FindStringSubmatch(msg); m != nil {
				line, _ = strconv.Atoi(m[1])
			}
		}
		return one("W001", manifestFile, line, msg)
	}
	return outcome{}
}

var reWebhookSecret = regexp.MustCompile(`^/webhooks/\d+/secret$`)

func w002(r Repo, _ Context) outcome {
	if !r.HasManifest || r.ManifestNode == nil || r.ManifestErr == nil {
		return outcome{}
	}
	var out outcome
	for _, p := range problems(r.ManifestErr) {
		line := yamlLine(r.ManifestNode, p.Path)
		if reWebhookSecret.MatchString(p.Path) && strings.Contains(p.Message, "not listed under secrets") {
			out = out.add("W051", manifestFile, line, p.Message+".")
			continue
		}
		out = out.add("W002", manifestFile, line, p.Path+": "+p.Message)
	}
	return out
}

func w003(r Repo, _ Context) outcome {
	if r.Binding == nil {
		return skip("W003", "the directory is not bound to an app (.whisk/app.json is absent)")
	}
	if r.Manifest.Name != r.Binding.App {
		return one("W003", manifestFile, yamlLine(r.ManifestNode, "/name"), fmt.Sprintf("name is %q but .whisk/app.json binds this directory to app %q.", r.Manifest.Name, r.Binding.App))
	}
	return outcome{}
}

// bindingTracked reports whether .whisk/app.json is part of what git carries, so it may have
// arrived with the repository rather than been written by whisk use on this machine.
func bindingTracked(r Repo) bool {
	if r.Binding == nil {
		return false
	}
	for _, f := range r.Files {
		if f.Path == config.BindingPath {
			return f.Tracked
		}
	}
	return false
}

func w006(r Repo, c Context) outcome {
	switch {
	case !bindingTracked(r):
		return outcome{}
	case r.WhiskRemote == "":
		return outcome{}
	case c.BoundGitURL == "":
		return skip("W006", "the platform was not asked for the bound app's repository")
	}
	if !gitcmd.RemoteDisagrees(r.WhiskRemote, c.BoundGitURL) {
		return outcome{}
	}
	ref := r.Binding.Org + "/" + r.Binding.App
	return one("W006", config.BindingPath, 0, fmt.Sprintf("%s is committed and binds this directory to %s, but the git remote whisk points at %s, which is not that app's repository.", config.BindingPath, ref, gitcmd.Redact(r.WhiskRemote)))
}

func w010(r Repo, _ Context) outcome {
	hits, err := secretFindings(r)
	if err != nil {
		return skip("W010", err.Error())
	}
	var out outcome
	for _, h := range hits {
		out = out.add("W010", h.File, h.Line, fmt.Sprintf("A value matching the %s rule is in %s line %d.", h.Rule, h.File, h.Line))
	}
	return out
}

func w011(r Repo, _ Context) outcome {
	var out outcome
	for _, f := range r.Files {
		if f.Tracked && localDevFile(f.Path) {
			out = out.add("W011", f.Path, 0, f.Path+" "+carried(r)+"; .whisk/dev/ holds whisk dev's local files, secrets.env among them, and must not be committed.")
			continue
		}
		base := path.Base(f.Path)
		if !f.Tracked || (base != ".env" && !strings.HasPrefix(base, ".env.")) {
			continue
		}
		if base == ".env.example" && envExampleClean(f.Content) {
			continue
		}
		out = out.add("W011", f.Path, 0, f.Path+" is tracked by git; environment files hold values and must not be committed.")
	}
	return out
}

// localDevFile reports whether a path is under .whisk/dev/, whisk dev's local folder. Pure.
func localDevFile(p string) bool { return strings.HasPrefix(p, ".whisk/dev/") }

// carried says how a file reaches a commit: tracked by git, or, outside a repository, not
// ignored by .gitignore.
func carried(r Repo) string {
	if r.IsGit && r.GitOK {
		return "is tracked by git"
	}
	return "is not git-ignored, so a commit would carry it"
}

func w020(r Repo, _ Context) outcome {
	if len(r.Stack.Langs) == 0 && len(dockerfiles(r)) == 0 {
		return skip("W020", "no language doctor understands was detected")
	}
	var out outcome
	if ok, _ := portRead(r); !ok {
		out = out.add("W020", "", 0, "PORT is not read from the environment; the app must listen on the port in PORT.")
	}
	for _, h := range loopbackListens(r) {
		out = out.add("W020", h.File, h.Line, "The app listens on the loopback interface; listen on all interfaces (0.0.0.0) so the platform can reach it.")
	}
	return out
}

func w021(r Repo, _ Context) outcome {
	var out outcome
	for _, h := range writesOutsideTmp(r) {
		out = out.add("W021", h.File, h.Line, "Files are written under a relative directory that disappears on redeploy: "+strings.TrimSpace(h.Name)+".")
	}
	return out
}

// routesUnread is why a route check was skipped: what it would have confirmed, which ways of
// declaring routes doctor reads, and how to confirm it by hand.
func routesUnread(what string) string {
	return "doctor could not confirm " + what + " because it found no routes it can read. It reads " +
		"router calls (Express, Hono, Fastify and Koa app.get(\"/path\"); FastAPI, Flask and Django; " +
		"net/http, chi and gorilla), Next.js route files, and plain node:http checks " +
		"(req.url === \"/path\", case \"/path\":). Confirm the route by hand, or write it in one of those forms."
}

func w022(r Repo, _ Context) outcome {
	found, anyRouter := routeFound(r, r.Manifest.Health.Path, "GET")
	if found {
		return outcome{}
	}
	if !anyRouter {
		return skip("W022", routesUnread("a GET route for the health path "+r.Manifest.Health.Path))
	}
	return one("W022", manifestFile, yamlLine(r.ManifestNode, "/health/path"), fmt.Sprintf("No route for the health path %s was found in code.", r.Manifest.Health.Path))
}

var rePlatformName = regexp.MustCompile("`([A-Z][A-Z0-9_]+)`")

// platformSet is every name the platform injects (environment.md) plus the runtime names an
// app may read without declaring.
var platformSet = func() map[string]bool {
	set := map[string]bool{"NODE_ENV": true, "PYTHONPATH": true, "HOME": true, "PATH": true, "WHISK_DEV": true}
	for _, m := range rePlatformName.FindAllStringSubmatch(contract.EnvironmentDoc, -1) {
		set[m[1]] = true
	}
	return set
}()

// isPlatformName reports whether the platform owns a name. Every WHISK_ name is reserved for
// the platform (environment.md, Reserved), so a name this CLI's copy of environment.md does not
// list yet, because the platform is newer than the CLI, is still not the app's to declare.
func isPlatformName(n string) bool {
	return platformSet[n] || strings.HasPrefix(n, "WHISK_") || strings.HasPrefix(n, "INNGEST_")
}

func declaredNames(r Repo) map[string]bool {
	set := map[string]bool{}
	for _, n := range r.Manifest.Secrets {
		set[n] = true
	}
	for _, n := range r.Manifest.Build.Secrets {
		set[n] = true
	}
	for n := range r.Manifest.Env {
		set[n] = true
	}
	return set
}

func w030(r Repo, _ Context) outcome {
	declared := declaredNames(r)
	seen := map[string]bool{}
	var out outcome
	for _, f := range r.AllCode() {
		for _, h := range envReads(f) {
			if declared[h.Name] || isPlatformName(h.Name) || seen[h.Name] {
				continue
			}
			seen[h.Name] = true
			out = out.add("W030", h.File, h.Line, h.Name+" is read from the environment but not declared.")
		}
	}
	return out
}

func w031(r Repo, _ Context) outcome {
	read := map[string]bool{}
	for _, f := range r.AllCode() {
		for _, h := range append(envReads(f), nameLiterals(f)...) {
			read[h.Name] = true
		}
	}
	for _, w := range r.Manifest.Webhooks {
		read[w.Secret] = true
	}
	for _, n := range r.Manifest.Build.Secrets {
		read[n] = true
	}
	var out outcome
	for i, n := range r.Manifest.Secrets {
		if !read[n] {
			out = out.add("W031", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/secrets/%d", i)), n+" is declared under secrets but never read in code.")
		}
	}
	return out
}

// parsedGraph is a function's graph when it exists and validates.
type parsedGraph struct {
	Function string
	File     string
	Graph    graph.Graph
	Node     *yaml.Node
}

func graphs(r Repo) ([]parsedGraph, outcome) {
	var out outcome
	var ok []parsedGraph
	for i, fn := range r.Manifest.Functions {
		src := r.Read(fn.Graph)
		line := yamlLine(r.ManifestNode, fmt.Sprintf("/functions/%d/graph", i))
		if src == nil {
			out = out.add("W040", manifestFile, line, fmt.Sprintf("Function %s names graph %s, which does not exist.", fn.Name, fn.Graph))
			continue
		}
		g, err := graph.Parse(src)
		if err != nil {
			var node yaml.Node
			_ = yaml.Unmarshal(src, &node)
			for _, p := range graphProblems(err) {
				out = out.add("W040", fn.Graph, yamlLine(&node, p.Path), p.Path+": "+p.Message)
			}
			continue
		}
		var node yaml.Node
		_ = yaml.Unmarshal(src, &node)
		ok = append(ok, parsedGraph{Function: fn.Name, File: fn.Graph, Graph: g, Node: &node})
	}
	return ok, out
}

func w040(r Repo, _ Context) outcome {
	_, out := graphs(r)
	return out
}

func w041(r Repo, _ Context) outcome {
	parsed, _ := graphs(r)
	var out outcome
	for _, pg := range parsed {
		for i, s := range pg.Graph.Steps {
			if s.IsDecision() || literalFound(r, s.ID) {
				continue
			}
			out = out.add("W041", pg.File, yamlLine(pg.Node, fmt.Sprintf("/steps/%d/id", i)), fmt.Sprintf("Step %s of %s is not found as a string literal in code.", s.ID, pg.Function))
		}
	}
	return out
}

func w042(r Repo, _ Context) outcome {
	parsed, _ := graphs(r)
	if len(r.Manifest.Functions) == 0 {
		return outcome{}
	}
	declared := map[string]bool{}
	for _, pg := range parsed {
		for _, s := range pg.Graph.Steps {
			declared[s.ID] = true
		}
	}
	seen := map[string]bool{}
	var out outcome
	for _, f := range r.AllCode() {
		for _, h := range stepCalls(f) {
			if declared[h.Name] || seen[h.Name] {
				continue
			}
			seen[h.Name] = true
			out = out.add("W042", h.File, h.Line, fmt.Sprintf("Step %q is used in code but is not in any declared graph.", h.Name))
		}
	}
	return out
}

func w050(r Repo, _ Context) outcome {
	var out outcome
	for i, w := range r.Manifest.Webhooks {
		found, anyRouter := routeFound(r, w.Handler, "POST")
		if found {
			continue
		}
		if !anyRouter {
			return skip("W050", routesUnread(fmt.Sprintf("a POST route for webhook %s at %s", w.Name, w.Handler)))
		}
		out = out.add("W050", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/webhooks/%d/handler", i)), fmt.Sprintf("No POST route for webhook %s at %s was found in code.", w.Name, w.Handler))
	}
	return out
}

// deliveryCheckPattern is what a handler's file must reference to count as verifying the
// delivery: the template helper by name, or the signature header itself.
var deliveryCheckPattern = regexp.MustCompile(`\bdeliveries\.(?:Handle|handle)\b|X-Whisk-Delivery-Signature`)

func w053(r Repo, _ Context) outcome {
	var out outcome
	for i, w := range r.Manifest.Webhooks {
		files := routeFiles(r, w.Handler, "POST")
		if len(files) == 0 {
			continue // W050 reports a missing route; nothing to inspect here.
		}
		verified := false
		for _, f := range files {
			if deliveryCheckPattern.Match(f.Content) {
				verified = true
				break
			}
		}
		if !verified {
			out = out.add("W053", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/webhooks/%d/handler", i)), fmt.Sprintf("The handler for webhook %s at %s (%s) does not verify X-Whisk-Delivery-Signature.", w.Name, w.Handler, files[0].Path))
		}
	}
	return out
}

func w052(r Repo, _ Context) outcome {
	var out outcome
	for i, p := range r.Manifest.Routes.Public {
		for _, s := range r.Manifest.ServiceRoutes() {
			if p == s || routes.Match(p, s) {
				out = out.add("W052", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/routes/public/%d", i)), fmt.Sprintf("routes.public entry %s covers %s, which the platform delivers to itself.", p, s))
				break
			}
		}
	}
	return out
}

func w060(r Repo, _ Context) outcome {
	names := dockerfiles(r)
	if len(names) == 0 {
		return skip("W060", "no Dockerfile; the build uses Railpack")
	}
	name := names[0]
	ins := instructions(r.Read(name))
	start := 0
	for i, line := range ins {
		if strings.HasPrefix(line, "FROM ") {
			start = i
		}
	}
	final := ins[start:]
	user := ""
	port := false
	for _, line := range final {
		fields := strings.Fields(line)
		switch fields[0] {
		case "USER":
			if len(fields) > 1 {
				user = fields[1]
			}
		case "EXPOSE":
			if strings.Contains(line, "8080") {
				port = true
			}
		case "CMD", "ENTRYPOINT":
			if strings.Contains(line, "$PORT") || strings.Contains(line, "${PORT") {
				port = true
			}
		}
	}
	var out outcome
	switch u := strings.SplitN(user, ":", 2)[0]; u {
	case "":
		out = out.add("W060", name, 0, "The Dockerfile has no USER instruction in its final stage, so the container runs as root.")
	case "root", "0":
		out = out.add("W060", name, 0, "The Dockerfile's final USER is root.")
	}
	if !port {
		out = out.add("W060", name, 0, "The Dockerfile neither exposes 8080 nor reads PORT in CMD or ENTRYPOINT.")
	}
	return out
}

func w062(r Repo, _ Context) outcome {
	names := dockerfiles(r)
	if len(names) == 0 {
		return skip("W062", "no Dockerfile; the build uses Railpack")
	}
	migrate := strings.TrimSpace(r.Manifest.Migrate)
	if migrate == "" {
		return skip("W062", "no migrate command")
	}
	image := finalImage(instructions(r.Read(names[0])))
	if !shellless(image) {
		return outcome{}
	}
	if why := needsShell(migrate); why != "" {
		return one("W062", manifestFile, yamlLine(r.ManifestNode, "/migrate"),
			fmt.Sprintf("migrate %q %s, but the final image %s has no shell or package manager.", migrate, why, image))
	}
	return outcome{}
}

// finalImage is the image the Dockerfile's last stage starts from, without its flags.
func finalImage(ins []string) string {
	image := ""
	for _, line := range ins {
		fields := strings.Fields(line)
		if fields[0] != "FROM" {
			continue
		}
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "--") {
				image = f
				break
			}
		}
	}
	return image
}

// shellless reports whether an image is known to ship without /bin/sh.
func shellless(image string) bool {
	name, _, _ := strings.Cut(image, "@")
	return name == "scratch" || (strings.HasPrefix(name, "gcr.io/distroless/") && !strings.Contains(name, ":debug"))
}

// needsShell says why a command cannot run as plain words, or "" when it can.
func needsShell(command string) string {
	switch strings.Fields(command)[0] {
	case "npm", "npx", "yarn", "pnpm", "bun":
		return "runs " + strings.Fields(command)[0]
	case "sh", "bash", "/bin/sh", "/bin/bash":
		return "runs a shell"
	}
	if strings.ContainsAny(command, "&|;<>$`*") {
		return "uses shell syntax"
	}
	return ""
}

func w063(r Repo, c Context) outcome {
	if c.Packages == nil {
		return skip("W063", "the platform was not asked for the app's package scan")
	}
	if !c.Packages.Included {
		return outcome{}
	}
	// The lockfiles are judged by the check of the local files when there is one, so a fix is
	// seen before it is deployed; otherwise by the live scan, less what the local lockfiles no
	// longer pin. The image is only known once it is built, so it comes from the live scan.
	var findings []api.PackageFinding
	if c.LockCheck != nil {
		findings = append(findings, c.LockCheck.Findings...)
	}
	if c.Packages.Scan != nil {
		for _, f := range c.Packages.Scan.Findings {
			if f.Where == "image" || c.LockCheck == nil {
				findings = append(findings, f)
			}
		}
	}
	var out outcome
	for _, g := range toFix(findings) {
		if g.Where == "source" && c.LockCheck == nil && !stillPinned(r.Read(g.Path), g.Path, g.Package, g.Version) {
			continue
		}
		file, where := g.Path, "the lockfile"
		if g.Where == "image" {
			file, where = "Dockerfile", "the live image"
		}
		out = out.add("W063", file, 0, fmt.Sprintf("%s %s in %s has %d known %s vulnerabilit%s (%s); %s fixes %s.",
			g.Package, g.Version, where, g.Count, g.Severity, plural(g.Count, "y", "ies"), g.ID, g.Fixed, plural(g.Count, "it", "them")))
	}
	return out
}

// packageGroup is one package version's serious findings, folded into one line.
type packageGroup struct {
	api.PackageFinding
	Count int
}

// toFix folds the critical and high findings that have a fix into one group per package version
// and place, keeping the worst severity, the first finding's ID and the highest fixed version.
func toFix(fs []api.PackageFinding) []packageGroup {
	var out []packageGroup
	index := map[string]int{}
	for _, f := range fs {
		if (f.Severity != "critical" && f.Severity != "high") || f.Fixed == "" {
			continue
		}
		key := string(f.Where) + "\x00" + f.Path + "\x00" + f.Package + "\x00" + f.Version
		i, ok := index[key]
		if !ok {
			index[key] = len(out)
			out = append(out, packageGroup{PackageFinding: f, Count: 1})
			continue
		}
		g := &out[i]
		g.Count++
		if f.Severity == "critical" {
			g.Severity = "critical"
		}
		if versionLess(g.Fixed, f.Fixed) {
			g.Fixed = f.Fixed
		}
	}
	return out
}

// stillPinned reports whether the local lockfile still holds the package at the scanned version,
// so a lockfile the agent has already updated is not reported again.
func stillPinned(lock []byte, file, pkg, version string) bool {
	if lock == nil {
		return false
	}
	if path.Base(file) == "package-lock.json" {
		var l struct {
			Packages map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if json.Unmarshal(lock, &l) == nil && l.Packages != nil {
			for k, v := range l.Packages {
				if (k == "node_modules/"+pkg || strings.HasSuffix(k, "/node_modules/"+pkg)) && v.Version == version {
					return true
				}
			}
			return false
		}
	}
	for _, line := range strings.Split(string(lock), "\n") {
		if strings.Contains(line, pkg) && strings.Contains(line, version) {
			return true
		}
	}
	return false
}

// versionLess compares dotted versions number by number, falling back to text.
func versionLess(a, b string) bool {
	as, bs := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, errA := strconv.Atoi(as[i])
		y, errB := strconv.Atoi(bs[i])
		if errA != nil || errB != nil {
			if as[i] != bs[i] {
				return as[i] < bs[i]
			}
			continue
		}
		if x != y {
			return x < y
		}
	}
	return len(as) < len(bs)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func w061(r Repo, _ Context) outcome {
	var out outcome
	for _, lang := range r.Stack.Langs {
		switch lang {
		case stack.JS:
			if !r.Has("package-lock.json") && !r.Has("pnpm-lock.yaml") && !r.Has("bun.lock") && !r.Has("bun.lockb") && !r.Has("yarn.lock") {
				out = out.add("W061", "package.json", 0, "package.json has no lockfile; commit package-lock.json, pnpm-lock.yaml, bun.lock or yarn.lock.")
			}
		case stack.PY:
			if r.Has("uv.lock") || r.Has("poetry.lock") || r.Has("pdm.lock") {
				continue
			}
			if req := r.Read("requirements.txt"); req != nil && pinned(req) {
				continue
			}
			file := "pyproject.toml"
			if !r.Has(file) {
				file = "requirements.txt"
			}
			out = out.add("W061", file, 0, "No Python lockfile; commit uv.lock, poetry.lock, pdm.lock or a requirements.txt with every version pinned.")
		case stack.GO:
			if !r.Has("go.sum") && bytes.Contains(r.Read("go.mod"), []byte("require")) {
				out = out.add("W061", "go.mod", 0, "go.mod declares requirements but go.sum is missing; run go mod tidy and commit it.")
			}
		}
	}
	return out
}

func pinned(req []byte) bool {
	any := false
	for _, line := range strings.Split(string(req), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "-") {
			continue
		}
		any = true
		if !strings.Contains(t, "==") && !strings.Contains(t, "@") {
			return false
		}
	}
	return any
}

func w070(r Repo, c Context) outcome {
	if !r.IsGit {
		return skip("W070", "not a git repository yet; whisk deploy creates one")
	}
	if !r.GitOK {
		return skip("W070", "git is not installed, so the packed size cannot be measured")
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	size, err := repoSize(ctx, r.Dir)
	if err != nil {
		return skip("W070", err.Error())
	}
	limit := c.RepoLimit
	if limit <= 0 {
		limit = FreeRepoLimit
	}
	if size <= limit {
		return outcome{}
	}
	msg := fmt.Sprintf("The repository is %s packed; the plan allows %s.", human(size), human(limit))
	if blobs := largestBlobs(ctx, r.Dir, 5); len(blobs) > 0 {
		parts := make([]string, len(blobs))
		for i, b := range blobs {
			parts[i] = b.Path + " (" + human(b.Bytes) + ")"
		}
		msg += " Largest: " + strings.Join(parts, ", ") + "."
	}
	return one("W070", "", 0, msg)
}

func w080(r Repo, c Context) outcome {
	if c.Validation == nil {
		if r.Binding == nil {
			return skip("W080", "the directory is not bound to an app, so the plan is unknown")
		}
		return skip("W080", "the platform was not asked, so the plan is unknown")
	}
	var out outcome
	for _, setting := range c.Validation.Unavailable {
		out = out.add("W080", manifestFile, yamlLine(r.ManifestNode, "/"+setting), fmt.Sprintf("%s needs a plan the org does not have (current plan: %s).", setting, c.Validation.Plan))
	}
	return out
}

// Git measurements for W070.

func repoSize(ctx context.Context, dir string) (int64, error) {
	cmd := run.Command(ctx, gitQuick, "git", "count-objects", "-v")
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git count-objects: %v", err)
	}
	var kib int64
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if k == "size" || k == "size-pack" {
			n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			kib += n
		}
	}
	return kib * 1024, nil
}

type blob struct {
	Path  string
	Bytes int64
}

func largestBlobs(ctx context.Context, dir string, n int) []blob {
	objects, err := gitOutput(ctx, dir, gitHistory, nil, "rev-list", "--objects", "--all")
	if err != nil {
		return nil
	}
	raw, err := gitOutput(ctx, dir, gitHistory, bytes.NewReader(objects), "cat-file", "--batch-check=%(objecttype) %(objectsize) %(rest)")
	if err != nil {
		return nil
	}
	var blobs []blob
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) < 3 || fields[0] != "blob" {
			continue
		}
		size, _ := strconv.ParseInt(fields[1], 10, 64)
		blobs = append(blobs, blob{Path: fields[2], Bytes: size})
	}
	sort.Slice(blobs, func(i, j int) bool { return blobs[i].Bytes > blobs[j].Bytes })
	if len(blobs) > n {
		blobs = blobs[:n]
	}
	return blobs
}

func human(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%d KB", b/1024)
	}
}

// Problem adapters: the contract's manifest and graph packages return their own Problems
// types; doctor only needs path and message.

type problem struct{ Path, Message string }

func problems(err error) []problem {
	var out []problem
	switch ps := err.(type) {
	case manifest.Problems:
		for _, p := range ps {
			out = append(out, problem{p.Path, p.Message})
		}
	case graph.Problems:
		for _, p := range ps {
			out = append(out, problem{p.Path, p.Message})
		}
	default:
		out = append(out, problem{"", err.Error()})
	}
	return out
}

func graphProblems(err error) []problem { return problems(err) }

func w090(r Repo, _ Context) outcome {
	var out outcome
	for _, h := range passwordFields(r) {
		out = out.add("W090", h.File, h.Line, fmt.Sprintf("%s line %d asks for a password; the platform signs people in, so the app never does.", h.File, h.Line))
	}
	return out
}
