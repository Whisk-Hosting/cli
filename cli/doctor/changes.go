package doctor

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/custom"
)

// The rules of a fork's changes (FORKS.md §9): each change of the business's own has a record in
// custom/changes (W170) that can be read (W171), Whisk's SQL stays as the release has it (W172),
// and a record names only what differs from the release (W173). Each is quiet in a checkout that
// follows no release line.

// forkOf is the fork the checkout is, or what the rule id reports instead: nothing when the
// checkout follows no release line, and a skip when git could not read it or its branch holds
// none of the releases.
func forkOf(r Repo, id string) (*Fork, outcome) {
	switch {
	case r.ForkErr != "":
		return nil, skip(id, "git could not read the checkout against its release line ("+r.ForkErr+")")
	case r.Fork == nil:
		return nil, outcome{}
	case r.Fork.Base == "":
		return nil, skip(id, "the branch holds none of its product's releases yet; git merge whisk/upstream brings the newest")
	}
	return r.Fork, outcome{}
}

func w170(r Repo, _ Context) outcome {
	f, out := forkOf(r, "W170")
	if f == nil {
		return out
	}
	changes, _ := forkRecords(r)
	missing := custom.Uncovered(f.Changed, changes)
	switch len(missing) {
	case 0:
		return outcome{}
	case 1:
		return one("W170", missing[0], 0, fmt.Sprintf("%s differs from %s and no change in %s names it.", missing[0], f.Release, custom.ChangesDir))
	}
	return one("W170", "", 0, fmt.Sprintf("%d files differ from %s and no change in %s names them: %s.", len(missing), f.Release, custom.ChangesDir, few(missing, 10)))
}

func w171(r Repo, _ Context) outcome {
	if f, out := forkOf(r, "W171"); f == nil {
		return out
	}
	changes, problems := forkRecords(r)
	var out outcome
	for _, p := range slices.Sorted(maps.Keys(problems)) {
		for _, pr := range problems[p] {
			out = out.add("W171", p, pr.Line, p+" "+pr.Message+".")
		}
	}
	for _, c := range changes {
		for _, check := range c.Checks {
			switch {
			case !r.Has(check):
				out = out.add("W171", custom.RecordPath(c.Name), 0, fmt.Sprintf("%s names the check %s, which is not in the repository.", custom.RecordPath(c.Name), check))
			case !stack.IsTest(check):
				out = out.add("W171", custom.RecordPath(c.Name), 0, fmt.Sprintf("%s names the check %s, which doctor does not read as a test: a check is a test the app's tests run.", custom.RecordPath(c.Name), check))
			}
		}
	}
	return out
}

func w172(r Repo, _ Context) outcome {
	f, out := forkOf(r, "W172")
	if f == nil {
		return out
	}
	for _, p := range sortedUnique(f.Changed) {
		if !custom.WhiskSQL(p) {
			continue
		}
		switch {
		case !r.Has(p):
			out = out.add("W172", p, 0, fmt.Sprintf("%s is Whisk's SQL and is gone from this copy; git checkout %s -- %s gives it back as %s has it.", p, short(f.Base), p, f.Release))
		case slices.Contains(f.Added, p):
			out = out.add("W172", p, 0, fmt.Sprintf("%s is SQL of the business's own outside custom/, which %s does not have; git rm %s takes it out once what it does is in custom/migrations or custom/views.", p, f.Release, p))
		default:
			out = out.add("W172", p, 0, fmt.Sprintf("%s is Whisk's SQL and differs from %s; git checkout %s -- %s gives it back as the release has it.", p, f.Release, short(f.Base), p))
		}
	}
	return out
}

func w173(r Repo, _ Context) outcome {
	f, out := forkOf(r, "W173")
	if f == nil {
		return out
	}
	changes, _ := forkRecords(r)
	for _, c := range changes {
		same := custom.Unchanged(f.Changed, c)
		switch {
		case len(same) == 0:
			continue
		case len(same) == len(c.Touches):
			out = out.add("W173", custom.RecordPath(c.Name), 0, fmt.Sprintf("Everything %s names is as %s has it, so the release holds the change now or it was undone.", custom.RecordPath(c.Name), f.Release))
		default:
			which := "which is as " + f.Release + " has it"
			if len(same) > 1 {
				which = "which are as " + f.Release + " has them"
			}
			out = out.add("W173", custom.RecordPath(c.Name), 0, fmt.Sprintf("%s names %s, %s.", custom.RecordPath(c.Name), few(same, 10), which))
		}
	}
	return out
}

// few names up to n paths as a sentence does, and how many more there are.
func few(paths []string, n int) string {
	if len(paths) <= n {
		return and(paths)
	}
	return strings.Join(paths[:n], ", ") + fmt.Sprintf(" and %d more", len(paths)-n)
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func sortedUnique(ps []string) []string {
	out := append([]string(nil), ps...)
	sort.Strings(out)
	n := 0
	for i, p := range out {
		if i == 0 || p != out[n-1] {
			out[n] = p
			n++
		}
	}
	return out[:n]
}
