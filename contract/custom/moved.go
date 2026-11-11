//whisk:pure

package custom

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// What a release moved (FORKS.md §4). A product's source keeps one note in moved/ for what its
// migrations remove or rename: each name that moved and where it went, what became of the values
// it held, and what code that used it does instead. The source cannot deploy a migration that
// moves a name no note names (SQLMoves, Unexplained), and a merge of the release hands each change
// of a business's own whose files use a name that moved the note and the lines that use it
// (NoteUses).

// MovedDir is the folder of a product's source that keeps its notes on what moved.
const MovedDir = "moved"

// movedNameMax caps a name a note says moved.
const movedNameMax = 200

// Moved is one note: its ID, which is its file's name; each name that moved and where to; what
// became of the values the names held, "" when they held none; and the fix for code that used them.
type Moved struct {
	ID     string `json:"id"`
	Moves  []Move `json:"moved"`
	Values string `json:"values,omitempty"`
	Fix    string `json:"fix"`
}

// Move is one name a release moved and where it went, "" for one that went away. A name is a table,
// a table's column as table.column, a view, a function, a type, a type's value as type.value, or a
// file from the code's root.
type Move struct {
	From string `json:"from"`
	To   string `json:"to,omitempty"`
}

// MovedPath is the file of the note id.
func MovedPath(id string) string { return MovedDir + "/" + id + ".yaml" }

// MovedID is the note the file at p is, and whether it is one: a file directly in moved/, named in
// lower-case letters, digits and dashes and ending .yaml.
func MovedID(p string) (string, bool) {
	dir, file := path.Split(p)
	id, ok := strings.CutSuffix(file, ".yaml")
	return id, ok && dir == MovedDir+"/" && ChangeName(id)
}

// InMoved reports whether p is in moved/, which holds the notes and nothing else.
func InMoved(p string) bool { return strings.HasPrefix(p, MovedDir+"/") }

// ParseMoved reads the note id: moved, a list of from and to; fix; and values when the names held
// any, each once and nothing else. A note that cannot be read answers each problem, with its line.
func ParseMoved(id string, src []byte) (Moved, []Problem) {
	m := Moved{ID: id}
	switch {
	case len(src) > RecordMax:
		return m, []Problem{{Message: fmt.Sprintf("is over %d KiB, and a note says what moved in a few lines", RecordMax>>10)}}
	case !utf8.Valid(src) || slices.Contains(src, 0):
		return m, []Problem{{Message: "is not UTF-8 text"}}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		line, text := yamlError(err)
		return m, []Problem{{Line: line, Message: "does not parse as YAML: " + text}}
	}
	if len(doc.Content) == 0 {
		return m, []Problem{{Message: "is empty: name each name that moved in moved, with where it went, and say what code that used it does instead in fix"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return m, []Problem{{Line: root.Line, Message: "is not a mapping of moved, values and fix"}}
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
		case "moved":
			m.Moves, ps = moves(k, v)
		case "values":
			m.Values, ps = noteText("values", k, v)
		case "fix":
			m.Fix, ps = noteText("fix", k, v)
		default:
			ps = []Problem{{k.Line, fmt.Sprintf("has %s, which a note does not take: it takes moved, values and fix", k.Value)}}
		}
		out = append(out, ps...)
	}
	if !seen["moved"] {
		out = append(out, Problem{Message: "names nothing in moved: name each name that moved, with from and, unless it went away, to"})
	}
	if !seen["fix"] {
		out = append(out, Problem{Message: "says nothing in fix: say what code that used what moved does instead"})
	}
	return m, out
}

// noteText is a note's values or fix: one piece of text, not empty, at most AskedMax characters.
func noteText(key string, k, v *yaml.Node) (string, []Problem) {
	if v.Kind != yaml.ScalarNode {
		return "", []Problem{{v.Line, key + " is not text"}}
	}
	text := strings.TrimSpace(v.Value)
	switch {
	case text == "" || v.Tag == "!!null":
		return "", []Problem{{k.Line, "says nothing in " + key}}
	case utf8.RuneCountInString(text) > AskedMax:
		return text, []Problem{{v.Line, fmt.Sprintf("%s is over %d characters: say it in a few sentences", key, AskedMax)}}
	}
	return text, nil
}

// moves is a note's moved: a list, not empty and at most ListMax long, of mappings of from, a name,
// and to, where it went, which may be left out for a name that went away.
func moves(k, v *yaml.Node) ([]Move, []Problem) {
	if v.Kind != yaml.SequenceNode || len(v.Content) == 0 {
		return nil, []Problem{{k.Line, "names nothing in moved: name each name that moved, with from and, unless it went away, to"}}
	}
	if len(v.Content) > ListMax {
		return nil, []Problem{{k.Line, fmt.Sprintf("moved names over %d names: write a note for each change of the migrations", ListMax)}}
	}
	var out []Move
	var ps []Problem
	for _, item := range v.Content {
		if item.Kind != yaml.MappingNode {
			ps = append(ps, Problem{item.Line, "moved has an entry that is not a mapping of from and to"})
			continue
		}
		var mv Move
		keys := map[string]bool{}
		bad := false
		for i := 0; i+1 < len(item.Content); i += 2 {
			ek, ev := item.Content[i], item.Content[i+1]
			if ek.Value != "from" && ek.Value != "to" {
				ps, bad = append(ps, Problem{ek.Line, fmt.Sprintf("moved has %s, which an entry does not take: it takes from and to", ek.Value)}), true
				continue
			}
			if keys[ek.Value] {
				ps, bad = append(ps, Problem{ek.Line, "moved has an entry with " + ek.Value + " twice"}), true
				continue
			}
			keys[ek.Value] = true
			name := strings.TrimSpace(ev.Value)
			if why := movedNameProblem(name, ev); why != "" {
				ps, bad = append(ps, Problem{ev.Line, fmt.Sprintf("moved has a %s that %s", ek.Value, why)}), true
				continue
			}
			if ek.Value == "from" {
				mv.From = name
			} else {
				mv.To = name
			}
		}
		if !keys["from"] {
			ps = append(ps, Problem{item.Line, "moved has an entry with no from: name what moved"})
			continue
		}
		if !bad {
			out = append(out, mv)
		}
	}
	return out, ps
}

// movedNameProblem is why name cannot be a name a note says moved, "" when it can.
func movedNameProblem(name string, v *yaml.Node) string {
	switch {
	case v.Kind != yaml.ScalarNode || v.Tag == "!!null" || name == "":
		return "is not a name"
	case utf8.RuneCountInString(name) > movedNameMax:
		return fmt.Sprintf("is over %d characters", movedNameMax)
	case strings.ContainsAny(name, "\n\r\t"):
		return "is more than one line"
	}
	return ""
}

// SameName reports whether a and b name the same thing in a database: unquoted names in any case,
// with or without the schema public.
func SameName(a, b string) bool { return normalName(a) == normalName(b) }

// normalName is a name as a database knows it: lower-case unless quoted, without the schema public
// and without a function's arguments.
func normalName(n string) string {
	n, _, _ = strings.Cut(strings.TrimSpace(n), "(")
	parts := strings.Split(n, ".")
	for i, p := range parts {
		if unq, ok := strings.CutPrefix(p, `"`); ok {
			parts[i] = strings.ReplaceAll(strings.TrimSuffix(unq, `"`), `""`, `"`)
			continue
		}
		parts[i] = strings.ToLower(p)
	}
	if len(parts) > 1 && parts[0] == "public" {
		parts = parts[1:]
	}
	return strings.Join(parts, ".")
}

// Unexplained is each of moves, named as SQLMoves names them, that no note says moved, in order.
func Unexplained(moves []SQLMove, notes []Moved) []SQLMove {
	return slices.DeleteFunc(slices.Clone(moves), func(m SQLMove) bool {
		return slices.ContainsFunc(notes, func(n Moved) bool {
			return slices.ContainsFunc(n.Moves, func(mv Move) bool { return normalName(mv.From) == m.Name })
		})
	})
}

// Use is a line of a file that uses a name a release moved.
type Use struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Line is one line of a file, by its number from 1.
type Line struct {
	Number int
	Text   string
}

// useTextMax caps the text of a line a Use carries, in characters.
const useTextMax = 200

// NoteUses is each of lines, by the path of its file, that uses a name the note says moved, in
// order of path and line. A file's name is looked for as written, with or without its extension;
// any other name by its last part as a whole word in any case, so a column is found wherever code
// names it, whatever table it is read through.
func NoteUses(n Moved, lines map[string][]Line) []Use {
	words := make([]*regexp.Regexp, 0, len(n.Moves))
	var paths []string
	for _, mv := range n.Moves {
		if strings.Contains(mv.From, "/") {
			paths = append(paths, mv.From, strings.TrimSuffix(mv.From, path.Ext(mv.From)))
			continue
		}
		parts := strings.Split(normalName(mv.From), ".")
		words = append(words, regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_$])`+regexp.QuoteMeta(parts[len(parts)-1])+`($|[^A-Za-z0-9_$])`))
	}
	var out []Use
	for _, p := range slices.Sorted(maps.Keys(lines)) {
		for _, l := range lines[p] {
			if slices.ContainsFunc(words, func(w *regexp.Regexp) bool { return w.MatchString(l.Text) }) ||
				slices.ContainsFunc(paths, func(f string) bool { return strings.Contains(l.Text, f) }) {
				out = append(out, Use{File: p, Line: l.Number, Text: clip(strings.TrimSpace(l.Text), useTextMax)})
			}
		}
	}
	return out
}

// FileLines is the lines of a file's text, numbered from 1.
func FileLines(text []byte) []Line {
	split := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	out := make([]Line, 0, len(split))
	for i, l := range split {
		out = append(out, Line{i + 1, l})
	}
	return out
}

// clip is s cut to at most n characters, marked with … when cut.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
