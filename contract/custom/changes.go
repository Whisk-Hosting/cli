//whisk:pure

package custom

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// A fork's change records (FORKS.md §9). Each change of the business's own gets one file in
// custom/changes, named for it, that says what the business asked for in its own plain words, the
// files the change adds, changes or deletes, and the tests that prove it works. Which files differ
// from the release a fork holds is worked out from its code by machine and joined with the
// records here: a file no record names is a change without one (Uncovered), and a record whose
// files are all as the release has them is a change the release holds now, or one undone
// (Unchanged).

// ChangesDir is the folder of the records, from the code's root.
const ChangesDir = "custom/changes"

const (
	// AskedMax caps what a record says was asked, in characters, ListMax each of its lists, and
	// RecordMax the file, in bytes.
	AskedMax  = 1000
	ListMax   = 200
	RecordMax = 64 << 10
	// pathMax caps a path a record names.
	pathMax = 512
)

// Change is one change of the business's own: its name, what the business asked for, the files it
// adds, changes or deletes from the code's root (a folder ending "/" for everything in it), and the
// tests that prove it works.
type Change struct {
	Name    string   `json:"name"`
	Asked   string   `json:"asked"`
	Touches []string `json:"touches"`
	Checks  []string `json:"checks"`
}

// Problem is one reason a file of custom/changes is not a record that can be read, at a line of
// the file when there is one.
type Problem struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

var changeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ChangeName reports whether name may name a change: lower-case letters, digits and dashes,
// starting with a letter or a digit, at most 63 characters.
func ChangeName(name string) bool { return changeName.MatchString(name) }

// RecordPath is the file of the record of the change named name.
func RecordPath(name string) string { return ChangesDir + "/" + name + ".yaml" }

// RecordName is the change whose record the file at p is, and whether it is one: a file directly in
// custom/changes, named for its change and ending .yaml.
func RecordName(p string) (string, bool) {
	dir, file := path.Split(p)
	name, ok := strings.CutSuffix(file, ".yaml")
	return name, ok && dir == ChangesDir+"/" && ChangeName(name)
}

// InChanges reports whether p is in custom/changes, which holds the records and nothing else.
func InChanges(p string) bool { return strings.HasPrefix(p, ChangesDir+"/") }

// Exempt reports whether a difference at p is never a change: the records themselves, and the
// folder .whisk, which holds the checkout's own ties to Whisk.
func Exempt(p string) bool { return InChanges(p) || strings.HasPrefix(p, ".whisk/") }

// WhiskSQL reports whether p is SQL outside custom/, which in a fork is Whisk's: its migrations,
// which every update runs as the release has them.
func WhiskSQL(p string) bool {
	return strings.HasSuffix(strings.ToLower(p), ".sql") && !strings.HasPrefix(p, "custom/")
}

// ParseChange reads the record of the change named name: asked, touches and checks, each once and
// none empty, and nothing else. A record that cannot be read answers each problem, with its line.
func ParseChange(name string, src []byte) (Change, []Problem) {
	c := Change{Name: name}
	switch {
	case len(src) > RecordMax:
		return c, []Problem{{Message: fmt.Sprintf("is over %d KiB, and a record says what was asked in a few sentences", RecordMax>>10)}}
	case !utf8.Valid(src) || slices.Contains(src, 0):
		return c, []Problem{{Message: "is not UTF-8 text"}}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		line, text := yamlError(err)
		return c, []Problem{{Line: line, Message: "does not parse as YAML: " + text}}
	}
	if len(doc.Content) == 0 {
		return c, []Problem{{Message: "is empty: write what was asked in asked, the files in touches and the tests in checks"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return c, []Problem{{Line: root.Line, Message: "is not a mapping of asked, touches and checks"}}
	}
	var out []Problem
	seen := map[string]bool{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if seen[k.Value] {
			out = append(out, Problem{k.Line, k.Value + " is there twice"})
			continue
		}
		seen[k.Value] = true
		var ps []Problem
		switch k.Value {
		case "asked":
			c.Asked, ps = asked(k, v)
		case "touches":
			c.Touches, ps = paths("touches", k, v, true)
		case "checks":
			c.Checks, ps = paths("checks", k, v, false)
		default:
			ps = []Problem{{k.Line, fmt.Sprintf("has %s, which a record does not take: it takes asked, touches and checks", k.Value)}}
		}
		out = append(out, ps...)
	}
	for _, missing := range []struct{ key, why string }{
		{"asked", "says nothing in asked: write what the business asked for, in its own plain words"},
		{"touches", "names nothing in touches: name each file the change adds, changes or deletes"},
		{"checks", "names nothing in checks: name the tests that prove the change works"},
	} {
		if !seen[missing.key] {
			out = append(out, Problem{Message: missing.why})
		}
	}
	return c, out
}

// asked is a record's asked: one piece of text, not empty, at most AskedMax characters.
func asked(k, v *yaml.Node) (string, []Problem) {
	if v.Kind != yaml.ScalarNode {
		return "", []Problem{{v.Line, "asked is not text: write what the business asked for, in its own plain words"}}
	}
	text := strings.TrimSpace(v.Value)
	switch {
	case text == "" || v.Tag == "!!null":
		return "", []Problem{{k.Line, "says nothing in asked: write what the business asked for, in its own plain words"}}
	case utf8.RuneCountInString(text) > AskedMax:
		return text, []Problem{{v.Line, fmt.Sprintf("asked is over %d characters: say what was asked in a few sentences", AskedMax)}}
	}
	return text, nil
}

// paths is a record's touches or checks: a list of paths from the code's root, not empty, at most
// ListMax of them; folder allows a folder ending "/".
func paths(key string, k, v *yaml.Node, folder bool) ([]string, []Problem) {
	if v.Kind != yaml.SequenceNode {
		if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
			return nil, []Problem{{k.Line, emptyList(key)}}
		}
		return nil, []Problem{{v.Line, key + " is not a list: write each path on a line of its own starting -"}}
	}
	if len(v.Content) == 0 {
		return nil, []Problem{{k.Line, emptyList(key)}}
	}
	if len(v.Content) > ListMax {
		return nil, []Problem{{k.Line, fmt.Sprintf("%s names over %d paths: name a folder ending / for everything in it", key, ListMax)}}
	}
	var out []string
	var ps []Problem
	for _, item := range v.Content {
		if item.Kind != yaml.ScalarNode {
			ps = append(ps, Problem{item.Line, key + " has an entry that is not a path"})
			continue
		}
		p := strings.TrimSpace(item.Value)
		if why := PathProblem(p, folder); why != "" {
			ps = append(ps, Problem{item.Line, fmt.Sprintf("%s has %s, which %s", key, strconv.Quote(p), why)})
			continue
		}
		out = append(out, p)
	}
	return out, ps
}

func emptyList(key string) string {
	if key == "checks" {
		return "names nothing in checks: name the tests that prove the change works"
	}
	return "names nothing in touches: name each file the change adds, changes or deletes"
}

// PathProblem is why p cannot be a path a record names, "" when it can: a file from the code's
// root, or with folder a folder ending "/" for everything in it, never a pattern or a record.
func PathProblem(p string, folder bool) string {
	file := strings.TrimSuffix(p, "/")
	switch {
	case p == "":
		return "is empty"
	case len(p) > pathMax:
		return fmt.Sprintf("is over %d characters", pathMax)
	case strings.ContainsAny(p, "*?[]{}"):
		if folder {
			return "is a pattern: name each file, or a folder ending / for everything in it"
		}
		return "is a pattern: name each test file"
	case strings.ContainsFunc(p, func(r rune) bool { return r == '\\' || unicode.IsControl(r) }):
		return "is not a path from the code's root: write it with / between folders"
	case !folder && file != p:
		return "is a folder: name each test file"
	case file == "" || path.IsAbs(file) || path.Clean(file) != file || file == "." || file == ".." || strings.HasPrefix(file, "../"):
		return "is not a path from the code's root, such as custom/server/warehouse.ts"
	case InChanges(p) || file == ChangesDir:
		return "is where the records are kept, which are never a change"
	}
	return ""
}

// Covers reports whether a path a record names covers the file at p: the file itself, or a folder
// it is in.
func Covers(touch, p string) bool {
	return touch == p || strings.HasSuffix(touch, "/") && strings.HasPrefix(p, touch)
}

// Uncovered is each of changed, the paths that differ from the release a fork holds, that is a
// change and that no record names, in order.
func Uncovered(changed []string, changes []Change) []string {
	var out []string
	for _, p := range changed {
		if Exempt(p) || slices.ContainsFunc(changes, func(c Change) bool { return names(c, p) }) {
			continue
		}
		out = append(out, p)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Unchanged is each path c names that covers none of changed: a file as the release has it, or a
// folder with nothing in it that differs.
func Unchanged(changed []string, c Change) []string {
	var out []string
	for _, t := range c.Touches {
		if !slices.ContainsFunc(changed, func(p string) bool { return !Exempt(p) && Covers(t, p) }) {
			out = append(out, t)
		}
	}
	return out
}

func names(c Change, p string) bool {
	return slices.ContainsFunc(c.Touches, func(t string) bool { return Covers(t, p) })
}

// Records reads the records among files, which are the files of custom/changes by path: the
// changes that can be read, in name order, and each problem of the files that are not records or
// cannot be read, by path.
func Records(files map[string][]byte) ([]Change, map[string][]Problem) {
	var out []Change
	problems := map[string][]Problem{}
	for p, src := range files {
		name, ok := RecordName(p)
		if !ok {
			problems[p] = []Problem{{Message: "is not a record: custom/changes holds one file a change, named for it in lower-case letters, digits and dashes and ending .yaml"}}
			continue
		}
		c, ps := ParseChange(name, src)
		if len(ps) > 0 {
			problems[p] = ps
			continue
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Change) int { return strings.Compare(a.Name, b.Name) })
	return out, problems
}

var yamlLine = regexp.MustCompile(`^yaml: line (\d+): `)

// yamlError is the line a YAML error names, 0 when it names none, and what it says.
func yamlError(err error) (int, string) {
	text := err.Error()
	if m := yamlLine.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, strings.TrimPrefix(text, m[0])
	}
	return 0, strings.TrimPrefix(text, "yaml: ")
}
