package doctor

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/cli/scaffold"
)

// Safe fixes, doctor-rules.md: planned as file edits by pure functions, then applied.

// Edit is one file doctor will write.
type Edit struct {
	Path    string // slash separated, relative to the root
	Content []byte
	Note    string // what changed, for the report
}

// planFixes decides what --fix may change for the findings at hand. Notes without an edit
// are actions doctor reports but does not perform (a git rm, for example).
func planFixes(r Repo, findings []Finding) (edits []Edit, notes []string) {
	manifestSrc := r.ManifestSrc
	manifestChanged := false
	for _, f := range findings {
		switch f.Rule {
		case "W011":
			switch {
			case localDevFile(f.File):
				if r.IsGit {
					notes = appendNew(notes, []string{"run: git rm -r --cached .whisk/dev"})
				}
			case strings.HasPrefix(filepath.Base(f.File), ".env"):
				notes = append(notes, "run: git rm --cached "+f.File)
			}
		case "W051":
			name := firstWord(f.Message)
			if name == "" || manifestSrc == nil {
				continue
			}
			next, err := manifestAddSecret(manifestSrc, name)
			if err == nil && string(next) != string(manifestSrc) {
				manifestSrc, manifestChanged = next, true
				notes = append(notes, "whisk.yaml: added "+name+" to secrets")
			}
		case "W052":
			entry := publicEntry(f.Message)
			if entry == "" || manifestSrc == nil || strings.ContainsAny(entry, "*") {
				continue
			}
			next, err := manifestRemovePublic(manifestSrc, []string{entry})
			if err == nil && string(next) != string(manifestSrc) {
				manifestSrc, manifestChanged = next, true
				notes = append(notes, "whisk.yaml: removed "+entry+" from routes.public")
			}
		case "W022":
			if found, _ := routeFound(r, "/health", "GET"); found && r.Manifest.Health.Path != "/health" && manifestSrc != nil {
				next, err := manifestSetHealthPath(manifestSrc, "/health")
				if err == nil {
					manifestSrc, manifestChanged = next, true
					notes = append(notes, "whisk.yaml: health.path set to /health, the route that exists in code")
				}
			}
		case "W040":
			if e, ok := scaffoldGraph(r, f); ok {
				edits = append(edits, e)
				notes = append(notes, e.Note)
			}
		}
	}
	if hasRule(findings, "W011") {
		if e, ok := gitignoreEdit(r); ok {
			edits = append(edits, e)
			notes = append(notes, e.Note)
		}
	}
	if manifestChanged {
		edits = append(edits, Edit{Path: manifestFile, Content: manifestSrc, Note: "whisk.yaml rewritten"})
	}
	return edits, notes
}

func hasRule(findings []Finding, id string) bool {
	for _, f := range findings {
		if f.Rule == id {
			return true
		}
	}
	return false
}

var reFirstWord = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\b`)

func firstWord(msg string) string {
	if m := reFirstWord.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return ""
}

var rePublicEntry = regexp.MustCompile(`routes\.public entry (\S+) covers`)

func publicEntry(msg string) string {
	if m := rePublicEntry.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return ""
}

// gitignoreEdit makes sure .gitignore excludes environment files and whisk dev's folder.
func gitignoreEdit(r Repo) (Edit, bool) {
	existing := string(r.Read(".gitignore"))
	lines := strings.Split(existing, "\n")
	have := map[string]bool{}
	for _, l := range lines {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, want := range scaffold.GitignoreLines {
		if !have[want] {
			add = append(add, want)
		}
	}
	if len(add) == 0 {
		return Edit{}, false
	}
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += strings.Join(add, "\n") + "\n"
	return Edit{Path: ".gitignore", Content: []byte(out), Note: ".gitignore: added " + strings.Join(add, ", ")}, true
}

var reGraphMissing = regexp.MustCompile(`^Function (\S+) names graph (\S+), which does not exist\.$`)

// scaffoldGraph writes a graph from the step names found in the file that defines the
// function, when the graph file is missing or empty. With no step names there is nothing
// honest to write, so the finding stays.
func scaffoldGraph(r Repo, f Finding) (Edit, bool) {
	var name, file string
	switch m := reGraphMissing.FindStringSubmatch(f.Message); {
	case m != nil:
		name, file = m[1], m[2]
	case strings.Contains(f.Message, "the file is empty") && f.File != "":
		file = f.File
		for _, fn := range r.Manifest.Functions {
			if fn.Graph == file {
				name = fn.Name
			}
		}
	}
	if name == "" || file == "" {
		return Edit{}, false
	}
	steps := stepsForFunction(r, name)
	if len(steps) == 0 {
		return Edit{}, false
	}
	type step struct {
		ID    string `yaml:"id"`
		Label string `yaml:"label,omitempty"`
		Next  string `yaml:"next,omitempty"`
	}
	var doc struct {
		Steps []step `yaml:"steps"`
	}
	for i, s := range steps {
		st := step{ID: s}
		if i+1 < len(steps) {
			st.Next = steps[i+1]
		}
		doc.Steps = append(doc.Steps, st)
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return Edit{}, false
	}
	header := fmt.Sprintf("# Declared graph for %s, scaffolded by whisk doctor --fix from the step names in code.\n# Add labels, decisions (id ending in ?) and branches as the function grows.\n", name)
	return Edit{Path: file, Content: append([]byte(header), out...), Note: file + ": scaffolded from " + strings.Join(steps, ", ")}, true
}

// stepsForFunction returns the step names, in order of appearance, from the code file that
// names the function, or from every file when no file names it.
func stepsForFunction(r Repo, name string) []string {
	needle := regexp.MustCompile(`["'` + "`" + `]` + regexp.QuoteMeta(name) + `["'` + "`" + `]`)
	var files []File
	for _, f := range r.AllCode() {
		if needle.Match(f.Content) {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		for _, h := range stepCalls(f) {
			if !seen[h.Name] {
				seen[h.Name] = true
				out = append(out, h.Name)
			}
		}
	}
	return out
}

// applyEdits writes the planned files. Paths come from the repository (a graph name in
// whisk.yaml), so a path outside it or through a symbolic link is refused, never followed.
// Every path is checked before anything is written.
func applyEdits(dir string, edits []Edit) error {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Path < edits[j].Path })
	for _, e := range edits {
		if _, err := safefile.Inside(dir, e.Path); err != nil {
			return err
		}
	}
	for _, e := range edits {
		if err := safefile.Write(dir, e.Path, e.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
