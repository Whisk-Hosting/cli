package doctor

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/internal/stack"
	rules "github.com/whisk-run/contract/doctor"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/naughty"
)

// memRepo is what Load would read from these files, without git or a disk.
func memRepo(files map[string]string) Repo {
	r := Repo{Dir: "/nonexistent"}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		r.Files = append(r.Files, File{Path: p, Content: []byte(files[p]), Size: int64(len(files[p]))})
	}
	r.Stack = stack.Detect(paths, func(p string) []byte { return r.Read(p) })
	if src := r.Read("whisk.yaml"); src != nil {
		r.HasManifest, r.ManifestSrc = true, src
		var node yaml.Node
		if err := yaml.Unmarshal(src, &node); err == nil {
			r.ManifestNode = &node
		}
		r.Manifest, r.ManifestErr = manifest.Parse(src)
	}
	return r
}

// checkWithout is Check without W010, whose secret scan builds a gitleaks detector (tens of
// milliseconds) each time; TestNaughtyDoctor runs the full Check once over every string.
func checkWithout(r Repo, skip string) Report {
	saved := ruleTable
	defer func() { ruleTable = saved }()
	ruleTable = nil
	for _, rule := range saved {
		if rule.id != skip {
			ruleTable = append(ruleTable, rule)
		}
	}
	return Check(r, Context{})
}

// checkReport asserts every finding names a rule of the table with that rule's level and fix,
// and that the fixes planned for them leave whisk.yaml readable.
func checkReport(t *testing.T, s string, rep Report, r Repo) {
	t.Helper()
	for _, f := range rep.Findings {
		rule, ok := rules.Lookup(f.Rule)
		if !ok || f.Level != rule.Level || f.Fix != rule.Fix || f.Line < 0 || f.Message == "" {
			t.Errorf("%q: finding %+v does not match the rule table", s, f)
		}
	}
	if rep.Errors+rep.Warnings != len(rep.Findings) {
		t.Errorf("%q: counted %d errors and %d warnings for %d findings", s, rep.Errors, rep.Warnings, len(rep.Findings))
	}
	edits, _ := planFixes(r, rep.Findings)
	for _, e := range edits {
		var v any
		if strings.HasSuffix(e.Path, ".yaml") && yaml.Unmarshal(e.Content, &v) != nil {
			t.Errorf("%q: the fix for %s is not YAML:\n%s", s, e.Path, e.Content)
		}
	}
}

// Doctor reads a repository written by anyone: naughty strings in the manifest, in file names
// and in code give findings from the rule table and never a panic. Tests in this package do not
// run in parallel, so swapping the rule table is safe.
func TestNaughtyDoctor(t *testing.T) {
	for _, s := range naughty.Strings() {
		q, _ := yaml.Marshal(s)
		v := strings.TrimSpace(string(q))
		manifests := []string{
			strings.Replace(passing["whisk.yaml"], "name: fixture", "name: "+v, 1),
			strings.Replace(passing["whisk.yaml"], `public: ["/", "/health"]`, "public: [\"/\", \"/health\", "+v+"]", 1),
			strings.Replace(passing["whisk.yaml"], "secrets: [API_KEY, STRIPE_WEBHOOK_SECRET]", "secrets: [API_KEY, STRIPE_WEBHOOK_SECRET, "+v+"]", 1),
			strings.Replace(passing["whisk.yaml"], "graph: workflows/nightly.graph.yaml", "graph: "+v, 1),
			strings.Replace(passing["whisk.yaml"], "handler: /hooks/stripe", "handler: "+v, 1),
			strings.Replace(passing["whisk.yaml"], "name: nightly", "name: "+v, 1),
			passing["whisk.yaml"] + "env:\n  " + v + ": x\n",
			s,
		}
		for _, m := range manifests {
			r := memRepo(with(passing, map[string]string{"whisk.yaml": m}))
			checkReport(t, s, checkWithout(r, "W010"), r)
		}
		code := with(passing, map[string]string{
			"src/" + s + ".ts":             "const v = process.env." + s + ";\nconst w = process.env[" + v + "];\napp.get(" + v + ", (c) => c.text(" + v + "));\n",
			"src/functions.ts":             strings.Replace(passing["src/functions.ts"], `step.run("record"`, "step.run("+v, 1),
			"workflows/nightly.graph.yaml": s,
			s:                              s,
			"Dockerfile":                   "FROM node:22-slim\n" + s + "\nCMD [" + v + "]\n",
			".env.example":                 s,
			"requirements.txt":             s,
		})
		r := memRepo(code)
		checkReport(t, s, checkWithout(r, "W010"), r)
	}
	all := map[string]string{}
	for i, s := range naughty.Strings() {
		all[fmt.Sprintf("src/n%d.ts", i)] = "const v = " + s + ";\n"
		all[fmt.Sprintf("%d-%s", i, s)] = s
	}
	r := memRepo(with(passing, all))
	for i := range r.Files {
		r.Files[i].Tracked = true
	}
	checkReport(t, "every string", Check(r, Context{}), r)
}

// The manifest edits --fix makes keep the file YAML and do exactly what they say for any name.
func TestNaughtyManifestEdits(t *testing.T) {
	src := []byte(passing["whisk.yaml"])
	for _, s := range naughty.Strings() {
		if !utf8.ValidString(s) {
			continue
		}
		out, err := manifestAddSecret(src, s)
		if err != nil {
			t.Errorf("manifestAddSecret(%q): %v", s, err)
			continue
		}
		var doc struct {
			Secrets []string `yaml:"secrets"`
			Routes  struct {
				Public []string `yaml:"public"`
			} `yaml:"routes"`
			Health struct {
				Path string `yaml:"path"`
			} `yaml:"health"`
		}
		if err := yaml.Unmarshal(out, &doc); err != nil || doc.Secrets[len(doc.Secrets)-1] != s {
			t.Errorf("manifestAddSecret(%q) wrote %v:\n%s", s, err, out)
		}
		out, err = manifestSetHealthPath(src, s)
		if err != nil || yaml.Unmarshal(out, &doc) != nil || doc.Health.Path != s {
			t.Errorf("manifestSetHealthPath(%q): %v\n%s", s, err, out)
		}
		out, err = manifestRemovePublic(src, []string{s, "/health"})
		left := 1
		if s == "/" {
			left = 0
		}
		if err != nil || yaml.Unmarshal(out, &doc) != nil || len(doc.Routes.Public) != left {
			t.Errorf("manifestRemovePublic(%q): %v\n%s", s, err, out)
		}
		yamlLine(nil, s)
		var node yaml.Node
		_ = yaml.Unmarshal(src, &node)
		if l := yamlLine(&node, "/"+s); l < 1 {
			t.Errorf("yamlLine(%q) = %d", s, l)
		}
	}
}
