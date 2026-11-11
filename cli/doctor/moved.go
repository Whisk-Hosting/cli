package doctor

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/whisk-run/contract/custom"
)

// The notes on what a product's releases moved (FORKS.md §4): in the source of a product, each
// table, column, view, function or type its migrations remove or rename is named by a note in
// moved/ that can be read (W174), so a merge of a release into a business's changed code can
// hand each change the note on a name it uses. Quiet in any other app, a fork's checkout among
// them, where the release's notes and migrations came as Whisk made them.

func w174(r Repo, _ Context) outcome {
	if r.Manifest.Managed == nil || r.Fork != nil || r.ForkErr != "" {
		return outcome{}
	}
	notes, problems := movedNotes(r)
	var out outcome
	for _, p := range slices.Sorted(maps.Keys(problems)) {
		for _, pr := range problems[p] {
			out = out.add("W174", p, pr.Line, p+" "+pr.Message+".")
		}
	}
	files := slices.Clone(r.Files)
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	for _, f := range files {
		if !custom.WhiskSQL(f.Path) || strings.HasSuffix(strings.ToLower(f.Path), ".down.sql") || f.Binary {
			continue
		}
		said := map[string]bool{}
		for _, m := range custom.Unexplained(custom.SQLMoves(string(f.Content)), notes) {
			if said[m.Name] {
				continue
			}
			said[m.Name] = true
			what := "removes " + m.Name
			if m.To != "" {
				what = "renames " + m.Name + " to " + m.To
			}
			out = out.add("W174", f.Path, m.Line, fmt.Sprintf("%s %s, and no note in %s/ says where it went.", f.Path, what, custom.MovedDir))
		}
	}
	return out
}

// movedNotes reads the notes in moved/, and why each file there that is not one cannot be read.
func movedNotes(r Repo) ([]custom.Moved, map[string][]custom.Problem) {
	var notes []custom.Moved
	problems := map[string][]custom.Problem{}
	for _, f := range r.Files {
		if !custom.InMoved(f.Path) {
			continue
		}
		id, ok := custom.MovedID(f.Path)
		if !ok {
			problems[f.Path] = []custom.Problem{{Message: "is not a note: " + custom.MovedDir + "/ holds one file for each note, named in lower-case letters, digits and dashes and ending .yaml"}}
			continue
		}
		n, ps := custom.ParseMoved(id, recordContent(f))
		if len(ps) > 0 {
			problems[f.Path] = ps
			continue
		}
		notes = append(notes, n)
	}
	return notes, problems
}
