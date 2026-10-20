// Package scaffold is what whisk init writes into an existing project: a commented manifest
// for the detected stack and the .gitignore entries the conventions need. The templates
// package covers the empty-directory case.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/internal/hostfiles"
	"github.com/whisk-run/cli/internal/stack"
)

// An app slug is 3 to 40 lowercase letters, digits and single hyphens, starting and ending with
// a letter or digit, so it is one DNS label in <app>.<org>.whisk.page and the double hyphen in a
// preview's <app>--<branch> stays unambiguous (DESIGN.md §5).
var (
	reSlug    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	reNotSlug = regexp.MustCompile(`[^a-z0-9]+`)
)

// MaxSlug is the longest app slug, the same as the platform's and the schema's (CONTRACT.md §3).
const MaxSlug = 40

func validSlug(s string) bool { return len(s) >= 3 && len(s) <= MaxSlug && reSlug.MatchString(s) }

// Slug turns a directory name into an app slug, or reports why a chosen one is invalid. Every
// run of other characters, hyphens included, becomes one hyphen.
func Slug(name string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(name))
	s = reNotSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > MaxSlug {
		s = strings.Trim(s[:MaxSlug], "-")
	}
	if !validSlug(s) {
		return "", fmt.Errorf("%q does not make a valid app slug (3 to 40 of a-z, 0-9 and single hyphens)", name)
	}
	return s, nil
}

// CheckSlug validates a slug the user chose.
func CheckSlug(s string) error {
	if !validSlug(s) {
		return fmt.Errorf("%q is not a valid app slug: 3 to 40 characters of a-z, 0-9 and single hyphens, starting and ending with a letter or digit", s)
	}
	return nil
}

// ManifestName is a slug as the value of name: in whisk.yaml: plain, or quoted when YAML would
// read the plain word as something other than text (null, true, 123, 1e2).
func ManifestName(slug string) string {
	var v any
	if err := yaml.Unmarshal([]byte(slug), &v); err == nil && v == slug {
		return slug
	}
	return strconv.Quote(slug)
}

// Manifest returns a commented whisk.yaml for a slug and stack, carrying across what the app's
// previous host's files said (hostfiles.Read): its migrate step, health path, Dockerfile, secret
// names, settings, Redis and workers, with what is left to do by hand as comments at the top.
func Manifest(slug string, s stack.Stack, h hostfiles.Found) string {
	var b strings.Builder
	w := func(lines ...string) {
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
	}
	w("# whisk.yaml, conventions version 1. `whisk schema` documents every field;",
		"# `whisk doctor` checks this file and the code against the conventions.")
	if !h.Empty() {
		w("#", "# Written from "+strings.Join(h.Files, ", ")+".")
		if len(h.Notes) > 0 {
			w("# Still to do by hand:")
			for _, n := range h.Notes {
				w(comment("- "+n, "  ")...)
			}
		}
	}
	health := "/health"
	if strings.HasPrefix(h.Health, "/") {
		health = h.Health
	}
	public := `["/", "/health"]`
	if health != "/health" {
		public = fmt.Sprintf(`["/", %q]`, health)
	}
	w("whisk: 1",
		"name: "+ManifestName(slug),
		"",
		"routes:",
		"  # Anyone can reach these paths; every other path needs a signed-in member of the org.",
		"  # `*` matches within a path segment, `**` across segments.",
		"  public: "+public,
		"",
		"health:",
		"  path: "+health+"   # must answer 200 once the app can serve",
		"  timeout: 90",
		"",
		"database: app   # app (its own database), shared:<name>, or none")
	switch {
	case h.Migrate != "":
		w(fmt.Sprintf("migrate: %q   # runs before each deploy with DATABASE_URL set", h.Migrate))
	case s.Migrate != "":
		w(fmt.Sprintf("migrate: %q   # runs before each deploy with DATABASE_URL set", s.Migrate))
	default:
		w("# migrate: \"<command>\"   # runs before each deploy with DATABASE_URL set, for example", "#   node dist/migrate.js, alembic upgrade head, ./server migrate")
	}
	if d := strings.TrimPrefix(h.Dockerfile, "./"); d != "" && d != "Dockerfile" {
		w("", "build:", fmt.Sprintf("  dockerfile: %q", d))
	}
	w("", "# Names of values a human sets in the dashboard; read them from the environment.")
	if len(h.Secrets) == 0 {
		w("secrets: []")
	} else {
		w("secrets:")
		for _, n := range h.Secrets {
			w("  - " + n)
		}
	}
	w("", "# Plain configuration the app reads from the environment.")
	if len(h.Env) == 0 {
		w("# env:", "#   LOG_LEVEL: info")
	} else {
		w("env:")
		for _, k := range sortedKeys(h.Env) {
			w(fmt.Sprintf("  %s: %q", k, h.Env[k]))
		}
	}
	if h.KV {
		w("", "kv: true   # a Redis-protocol store of the app's own, as WHISK_KV_URL")
	}
	if len(h.Workers) > 0 {
		w("",
			"# Workers run beside the web process, one copy each, and only in a Promoted app:",
			"# production waits for an owner to promote it. database: direct gives a worker a",
			"# session connection, for queues that LISTEN or take advisory locks.",
			"promoted: true",
			"workers:")
		for _, wk := range h.Workers {
			w("  - name: "+wk.Name,
				fmt.Sprintf("    command: %q", wk.Command),
				"    database: "+wk.Database)
		}
	}
	w("",
		"# Scheduled and event-driven functions, each with a declared graph.",
		"# functions:",
		"#   - name: nightly-summary",
		`#     cron: "0 6 * * *"`,
		"#     graph: workflows/nightly-summary.graph.yaml",
		"#   - name: on-order",
		"#     event: order.created",
		"#     graph: workflows/on-order.graph.yaml",
		"",
		"# Inbound webhooks: the platform verifies the signature and delivers to the handler.",
		"# webhooks:",
		"#   - name: stripe",
		"#     preset: stripe",
		"#     secret: STRIPE_WEBHOOK_SECRET",
		"#     handler: /hooks/stripe")
	return b.String()
}

// comment wraps text into YAML comment lines of at most 100 characters, continuing with indent.
func comment(text, indent string) []string {
	var out []string
	line := "#"
	for _, word := range strings.Fields(hostfiles.Clean(text)) {
		if len(line)+1+len(word) > 100 && line != "#" {
			out = append(out, line)
			line = "# " + indent[1:]
		}
		line += " " + word
	}
	return append(out, line)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// GitignoreLines are the entries every app repository needs.
var GitignoreLines = []string{".whisk/dev/", ".env", ".env.*", "!.env.example"}

// MergeGitignore returns the .gitignore content with the needed entries present, and which
// were added.
func MergeGitignore(existing string) (string, []string) {
	have := map[string]bool{}
	for _, l := range strings.Split(existing, "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var added []string
	for _, l := range GitignoreLines {
		if !have[l] {
			added = append(added, l)
		}
	}
	if len(added) == 0 {
		return existing, nil
	}
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if out != "" {
		out += "\n# whisk\n"
	}
	return out + strings.Join(added, "\n") + "\n", added
}

// IsEmptyDir reports whether a directory holds no code: nothing, or only what an agent or an
// editor keeps beside it (Ignorable). A template is written into such a directory.
func IsEmptyDir(dir string) (bool, error) {
	others, err := CodeFiles(dir)
	return len(others) == 0, err
}

// CodeFiles is the top-level names in dir that are not Ignorable, sorted; none when dir does
// not exist.
func CodeFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !Ignorable(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// agentFiles are the instruction files coding agents keep at a project's root.
var agentFiles = map[string]bool{"CLAUDE.md": true, "AGENTS.md": true, "GEMINI.md": true, "CLAUDE.local.md": true}

// Ignorable reports whether a top-level name is not the app's code: a dot-file or folder (.git,
// .claude, .cursor, .vscode, .gitignore) or an agent's instruction file. Pure.
func Ignorable(name string) bool { return strings.HasPrefix(name, ".") || agentFiles[name] }

// ListFiles returns the top-level and one-level-deep file names, for stack detection.
func ListFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if stack.IsVendored(filepath.ToSlash(rel)+"/x") || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if strings.Count(filepath.ToSlash(rel), "/") >= 1 {
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out
}
