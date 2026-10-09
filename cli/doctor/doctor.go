// Package doctor checks a repository against the conventions (doctor-rules.md) without
// running the app: it reads the manifest, the graphs and the code, and reports findings with
// stable rule ids. It is what `whisk doctor` runs and what the harness calls as a library.
package doctor

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/whisk-run/cli/internal/api"
	rules "github.com/whisk-run/contract/doctor"
)

// Options for one run.
type Options struct {
	Dir string
	Fix bool
	// Validate asks the platform about the manifest when the directory is bound and a token
	// is available; nil runs offline. An error from it is reported as a skip, not a failure.
	Validate func(ctx context.Context, manifestYAML []byte) (api.Validation, error)
	// Packages asks the platform for the app's newest package scan (W063); nil skips it. An
	// error from it is reported as a skip.
	Packages func(ctx context.Context) (api.Packages, error)
	// CheckLockfiles asks the platform to check the local lockfiles before they are deployed
	// (W063), only when Packages says the plan includes scanning; nil leaves the lockfiles to
	// the live scan. An error from it is reported as a skip.
	CheckLockfiles func(ctx context.Context, files []api.Lockfile) (api.PackageCheck, error)
	// BoundGitURL asks the platform for the git_url of the app .whisk/app.json names (W006),
	// only when the binding is tracked and a remote whisk exists; nil skips the check. An
	// error from it is reported as a skip.
	BoundGitURL func(ctx context.Context) (string, error)
}

// Report is the outcome: findings, what --fix changed, and what could not be checked.
type Report struct {
	Findings []Finding `json:"findings"`
	Fixed    []string  `json:"fixed,omitempty"`
	Skipped  []string  `json:"skipped,omitempty"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Plan     string    `json:"plan,omitempty"`
}

// OK is true when no finding is an error.
func (r Report) OK() bool { return r.Errors == 0 }

// Run loads the directory, applies every rule, and, with Fix, applies the safe fixes and runs
// the rules again so the report shows what remains.
func Run(ctx context.Context, opts Options) (Report, error) {
	repo, err := Load(ctx, opts.Dir)
	if err != nil {
		return Report{}, err
	}
	c := Context{Ctx: ctx, RepoLimit: FreeRepoLimit}
	var skipped []string
	if opts.Validate != nil && repo.HasManifest && repo.ManifestErr == nil {
		v, err := opts.Validate(ctx, repo.ManifestSrc)
		if err != nil {
			skipped = append(skipped, "validate: the platform could not be asked ("+err.Error()+"); plan checks skipped")
		} else {
			c.Validation = &v
			if v.Limits.RepoBytes > 0 {
				c.RepoLimit = v.Limits.RepoBytes
			}
		}
	}
	if opts.Packages != nil {
		p, err := opts.Packages(ctx)
		if err != nil {
			skipped = append(skipped, "packages: the platform could not be asked ("+err.Error()+"); package scan skipped")
		} else {
			c.Packages = &p
		}
		if err == nil && p.Included && opts.CheckLockfiles != nil {
			if files := lockfiles(repo); len(files) > 0 {
				check, err := opts.CheckLockfiles(ctx, files)
				if err != nil {
					skipped = append(skipped, "packages: the local lockfiles could not be checked ("+err.Error()+"); the live app's scan is used instead")
				} else {
					c.LockCheck = &check
				}
			}
		}
	}
	if opts.BoundGitURL != nil && bindingTracked(repo) && repo.WhiskRemote != "" {
		u, err := opts.BoundGitURL(ctx)
		if err != nil {
			skipped = append(skipped, "W006: the platform could not be asked for the bound app's repository ("+err.Error()+"); remote check skipped")
		} else {
			c.BoundGitURL = u
		}
	}
	report := Check(repo, c)
	report.Skipped = append(skipped, report.Skipped...)
	if !opts.Fix {
		return report, nil
	}
	// A fix can unlock rules that were skipped while the manifest was invalid, so fixing is
	// repeated until a pass changes nothing.
	var fixed []string
	for pass := 0; pass < 4; pass++ {
		edits, notes := planFixes(repo, report.Findings)
		fixed = appendNew(fixed, notes)
		if len(edits) == 0 {
			break
		}
		if err := applyEdits(repo.Dir, edits); err != nil {
			return report, err
		}
		if repo, err = Load(ctx, opts.Dir); err != nil {
			return report, err
		}
		report = Check(repo, c)
		report.Skipped = append(skipped, report.Skipped...)
	}
	report.Fixed = fixed
	return report, nil
}

func appendNew(list []string, more []string) []string {
	seen := map[string]bool{}
	for _, s := range list {
		seen[s] = true
	}
	for _, s := range more {
		if !seen[s] {
			seen[s] = true
			list = append(list, s)
		}
	}
	return list
}

// Check applies the rules to a loaded repository. It is pure: a function of the repo and the
// platform's answers.
func Check(r Repo, c Context) Report {
	var report Report
	manifestOK := r.HasManifest && r.ManifestErr == nil
	for _, rule := range ruleTable {
		if rule.needsManifest && !manifestOK {
			report.Skipped = append(report.Skipped, rule.id+": skipped, whisk.yaml is missing or invalid")
			continue
		}
		o := rule.run(r, c)
		for _, f := range o.findings {
			if f.Level == rules.Warning && f.Line > 0 && allowedAt(r.Read(f.File), f.Line, f.Rule) {
				report.Skipped = append(report.Skipped, fmt.Sprintf("%s: allowed at %s:%d by a doctor: allow comment", f.Rule, f.File, f.Line))
				continue
			}
			report.Findings = append(report.Findings, f)
		}
		report.Skipped = append(report.Skipped, o.skipped...)
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	for _, f := range report.Findings {
		if f.Level == rules.Error {
			report.Errors++
		} else {
			report.Warnings++
		}
	}
	if c.Validation != nil {
		report.Plan = c.Validation.Plan
	}
	return report
}

// allowedAt reports whether the code at a 1-based line, or the line above it, carries a
// "doctor: allow <rule>" comment, which keeps a warning the author meant out of the report.
// Errors cannot be allowed. Pure.
func allowedAt(content []byte, line int, rule string) bool {
	lines := strings.Split(string(content), "\n")
	mark := "doctor: allow " + rule
	for _, n := range []int{line, line - 1} {
		if n >= 1 && n <= len(lines) && strings.Contains(lines[n-1], mark) {
			return true
		}
	}
	return false
}

// LockfileNames are the lockfiles whisk doctor sends for a check before a deploy, the names
// POST …/packages/check accepts (CLI.md §5.3).
var LockfileNames = []string{
	"package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock",
	"requirements.txt", "poetry.lock", "Pipfile.lock", "uv.lock", "pdm.lock",
	"go.mod", "Cargo.lock", "Gemfile.lock", "composer.lock", "gradle.lockfile",
	"packages.lock.json", "mix.lock", "pubspec.lock",
}

// maxLockfile and maxLockfiles are the check's limits: 8 MiB a file, 50 files.
const (
	maxLockfile  = 8 << 20
	maxLockfiles = 50
)

// lockfiles reads the repository's lockfiles for a check: every file named in LockfileNames
// outside installed-dependency folders, read from disk when it is too large to be loaded, up to
// the check's limits.
func lockfiles(r Repo) []api.Lockfile {
	var out []api.Lockfile
	for _, f := range r.Files {
		if len(out) == maxLockfiles {
			break
		}
		if !isLockfile(f.Path) || f.Binary || f.Size > maxLockfile {
			continue
		}
		content := f.Content
		if content == nil {
			b, err := os.ReadFile(filepath.Join(r.Dir, filepath.FromSlash(f.Path)))
			if err != nil {
				continue
			}
			content = b
		}
		out = append(out, api.Lockfile{Path: f.Path, Content: string(content)})
	}
	return out
}

func isLockfile(p string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		switch seg {
		case "node_modules", "vendor", ".venv", "venv", ".git":
			return false
		}
	}
	for _, n := range LockfileNames {
		if path.Base(p) == n {
			return true
		}
	}
	return false
}
