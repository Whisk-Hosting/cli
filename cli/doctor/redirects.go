package doctor

import (
	"context"
	"fmt"
	"path"
	"strings"

	rules "github.com/whisk-run/contract/doctor"
	"github.com/whisk-run/contract/redirects"
)

// RedirectSet is every redirect the app declares, whisk.yaml's then the file's, with where each
// was written, as the push records them (CONTRACT.md §3.1).
type RedirectSet struct {
	Rules []redirects.Rule
	File  []string // the file each rule is in
	Line  []int    // its line there
}

// loadRedirects reads the manifest's redirects and the file it names. The outcome holds a W110
// finding for a file that is missing or a line that is not a rule.
func loadRedirects(r Repo) (RedirectSet, outcome) {
	var set RedirectSet
	var out outcome
	for i, rule := range r.Manifest.Redirects {
		set.Rules = append(set.Rules, rule)
		set.File = append(set.File, manifestFile)
		set.Line = append(set.Line, yamlLine(r.ManifestNode, fmt.Sprintf("/redirects/%d", i)))
	}
	name := r.Manifest.RedirectsFile
	if name == "" {
		return set, out
	}
	name = path.Clean(name)
	if !r.Has(name) {
		line := yamlLine(r.ManifestNode, "/redirects_file")
		return set, out.add("W110", manifestFile, line, fmt.Sprintf("redirects_file names %s, which does not exist.", name))
	}
	parsed, bad := redirects.ParseFile(r.Read(name))
	for _, p := range bad {
		out = out.add("W110", name, p.Line, p.Message)
	}
	for i, rule := range parsed.Rules {
		set.Rules = append(set.Rules, rule)
		set.File = append(set.File, name)
		set.Line = append(set.Line, parsed.Lines[i])
	}
	return set, out
}

// w110 checks the manifest's and the file's redirects together: the file's own lines, then a
// duplicate, a loop or too many across both.
func w110(r Repo, _ Context) outcome {
	set, out := loadRedirects(r)
	if len(out.findings) > 0 || r.Manifest.RedirectsFile == "" {
		// Without a file, whisk.yaml's rules were already checked with the manifest (W002).
		return out
	}
	for _, p := range redirects.Check(set.Rules) {
		i := min(p.Index, len(set.Rules)-1)
		out = out.add("W110", set.File[i], set.Line[i], fmt.Sprintf("%s %s", p.Field, p.Message))
	}
	return out
}

// w111 reports each redirect whose target another rule redirects again.
func w111(r Repo, _ Context) outcome {
	set, bad := loadRedirects(r)
	var out outcome
	if len(bad.findings) > 0 || len(set.Rules) == 0 || len(redirects.Check(set.Rules)) > 0 {
		return out
	}
	out.findings = set.Chains()
	return out
}

// chainWords is a chain as a sentence: /a redirects to /b, which redirects to /c.
func chainWords(hops []string) string {
	var b strings.Builder
	b.WriteString(hops[0] + " redirects to " + hops[1])
	for _, h := range hops[2:] {
		b.WriteString(", which redirects to " + h)
	}
	return b.String() + "."
}

// LoadRedirects reads the redirects of the app in dir, whisk.yaml's then its file's, as the push
// records them, with the findings that stop them being used: a missing or invalid whisk.yaml
// (W001, W002) and anything W110 reports. whisk redirects check and test read them here.
func LoadRedirects(ctx context.Context, dir string) (RedirectSet, []Finding, error) {
	r, err := Load(ctx, dir)
	if err != nil {
		return RedirectSet{}, nil, err
	}
	for _, check := range []func(Repo, Context) outcome{w001, w002} {
		if out := check(r, Context{}); len(out.findings) > 0 {
			return RedirectSet{}, out.findings, nil
		}
	}
	set, _ := loadRedirects(r)
	return set, w110(r, Context{}).findings, nil
}

// Chains are the set's chains, each with where its first rule is written (W111).
func (s RedirectSet) Chains() []Finding {
	var out []Finding
	for _, c := range redirects.Chains(s.Rules) {
		out = append(out, rules.NewFinding("W111", s.File[c.Index], s.Line[c.Index], chainWords(c.Hops)))
	}
	return out
}
