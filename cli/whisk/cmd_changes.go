package whisk

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/doctor"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/custom"
)

// A fork's changes of the business's own (FORKS.md §9): whisk changes lists each with its record in
// custom/changes and the files that differ from the release the code holds that no record names,
// and whisk changes draft writes a record naming them.

func changesCmd(s *session) *cobra.Command {
	c := &cobra.Command{
		Use:   "changes",
		Short: "List a fork's changes of its own, each with its record, and the files no record names",
		Long: `In a checkout of a fork, lists each change the business made to its product's release, from its
record in custom/changes: what the business asked for, the files the change adds, changes or
deletes, and the tests that prove it works. Then the files that differ from the release the code
holds that no record names, which whisk deploy refuses until one does (doctor W170); whisk changes
draft writes the record.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ch, err := s.forkChanges("whisk changes")
			if err != nil {
				return err
			}
			s.printer.Result(changesResult(ch), func(w io.Writer) { printChanges(s.printer, w, ch) })
			return nil
		},
	}
	c.AddCommand(changesDraftCmd(s))
	return c
}

func changesDraftCmd(s *session) *cobra.Command {
	var asked string
	var checks []string
	c := &cobra.Command{
		Use:   "draft <name> [--asked <text>] [--check <test>]...",
		Short: "Write the record of a change, naming every file that differs from the release and no record names",
		Long: `Writes custom/changes/<name>.yaml, the record of one change of the business's own, naming in
touches every file that differs from the release the code holds and no record names yet. --asked is
what the business asked for, in its own words, and --check a test that proves the change works,
once for each; without --check, the tests among those files are its checks. A record already there
keeps what it says and gains the files; --asked replaces what it says was asked.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChangesDraft(s, args[0], asked, checks)
		},
	}
	c.Flags().StringVar(&asked, "asked", "", "what the business asked for, in its own plain words")
	c.Flags().StringArrayVar(&checks, "check", nil, "a test that proves the change works, from the code's root (repeat for more)")
	return c
}

// forkChanges reads the checkout as doctor does for command, refusing one that follows no release
// line or holds none of its releases.
func (s *session) forkChanges(command string) (doctor.Changes, error) {
	ch, err := doctor.ReadChanges(s.ctx, s.env.Dir)
	switch {
	case err != nil:
		return ch, err
	case ch.Fork == nil:
		return ch, output.New("INVALID_REQUEST", "This checkout does not follow one of Whisk's products: its remote whisk has no branch upstream with the product's releases.",
			"Run "+command+" in a checkout of a fork, which whisk clone <org>/<app> makes; in one made before its first release, run git fetch whisk first.", nil)
	case ch.Fork.Base == "":
		return ch, output.New("INVALID_REQUEST", "This branch holds none of its product's releases yet.",
			"Run git fetch whisk and git merge whisk/upstream to bring the newest release, then try again.", nil)
	}
	return ch, nil
}

// unreadable is a file of custom/changes that is not a record that can be read, and why.
type unreadable struct {
	File     string           `json:"file"`
	Problems []custom.Problem `json:"problems"`
}

func changesResult(ch doctor.Changes) map[string]any {
	changes := ch.Changes
	if changes == nil {
		changes = []custom.Change{}
	}
	bad := []unreadable{}
	for _, p := range slices.Sorted(maps.Keys(ch.Problems)) {
		bad = append(bad, unreadable{p, ch.Problems[p]})
	}
	return map[string]any{"release": ch.Fork.Release, "base": ch.Fork.Base, "changes": changes, "unreadable": bad, "uncovered": nonNilStrings(ch.Uncovered)}
}

func printChanges(p output.Printer, w io.Writer, ch doctor.Changes) {
	if len(ch.Changes) == 0 && len(ch.Problems) == 0 && len(ch.Uncovered) == 0 {
		fmt.Fprintf(w, "This copy is %s as Whisk made it, with no changes of its own.\n", ch.Fork.Release)
		return
	}
	if len(ch.Changes) > 0 {
		fmt.Fprintf(w, "%s, with %s of the business's own:\n", ch.Fork.Release, count(len(ch.Changes), "change", "changes"))
	} else {
		fmt.Fprintf(w, "%s, with no record of a change of the business's own yet.\n", ch.Fork.Release)
	}
	for _, c := range ch.Changes {
		fmt.Fprintf(w, "\n%s\n  %s\n  %s\n  %s\n", p.Bold(c.Name), c.Asked, p.Dim("touches "+strings.Join(c.Touches, ", ")), p.Dim("checked by "+strings.Join(c.Checks, ", ")))
	}
	for _, f := range slices.Sorted(maps.Keys(ch.Problems)) {
		fmt.Fprintf(w, "\n%s %s %s\n", p.Bad(f), "cannot be read:", ch.Problems[f][0].Message)
	}
	if len(ch.Uncovered) > 0 {
		fmt.Fprintf(w, "\n%s from %s and no change names %s: %s.\n", count(len(ch.Uncovered), "file differs", "files differ"), ch.Fork.Release, them(len(ch.Uncovered)), sentence(ch.Uncovered, 10))
		fmt.Fprintln(w, p.Dim("whisk changes draft <name> --asked \"<what the business asked for>\" writes a record naming them; whisk deploy refuses the code until one does."))
	}
}

// sentence names up to n items as a sentence does, and how many more there are.
func sentence(items []string, n int) string {
	switch {
	case len(items) > n:
		return strings.Join(items[:n], ", ") + fmt.Sprintf(" and %d more", len(items)-n)
	case len(items) < 2:
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func runChangesDraft(s *session, name, asked string, checks []string) error {
	if !custom.ChangeName(name) {
		return output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a name for a change.", name),
			"Name it in lower-case letters, digits and dashes for what it does, such as tell-the-warehouse.", map[string]any{"name": name})
	}
	asked = strings.TrimSpace(asked)
	if utf8.RuneCountInString(asked) > custom.AskedMax {
		return output.New("INVALID_REQUEST", fmt.Sprintf("--asked is over %d characters.", custom.AskedMax), "Say what the business asked for in a few sentences.", nil)
	}
	for _, c := range checks {
		if why := checkProblem(s.env.Dir, c); why != "" {
			return output.New("INVALID_REQUEST", fmt.Sprintf("--check %s %s.", c, why),
				"Pass a test that proves the change works, from the code's root, once it is written: such as --check custom/server/test/warehouse.test.ts.", map[string]any{"check": c})
		}
	}
	ch, err := s.forkChanges("whisk changes")
	if err != nil {
		return err
	}
	file := custom.RecordPath(name)
	if ps := ch.Problems[file]; len(ps) > 0 {
		return output.New("INVALID_REQUEST", fmt.Sprintf("%s is there and cannot be read: it %s.", file, ps[0].Message),
			"Put the record right by hand (whisk doctor says where, under W171), or delete it and draft it again.", map[string]any{"file": file, "problems": ps})
	}
	i := slices.IndexFunc(ch.Changes, func(c custom.Change) bool { return c.Name == name })
	if i < 0 && len(ch.Uncovered) == 0 {
		return output.New("INVALID_REQUEST", fmt.Sprintf("Nothing differs from %s that no record names, so a record of %s would name nothing.", ch.Fork.Release, name),
			"Make the change first, then draft its record; whisk changes lists the records there are.", map[string]any{"name": name})
	}
	c := custom.Change{Name: name}
	if i >= 0 {
		c = ch.Changes[i]
	}
	c = drafted(c, ch.Uncovered, asked, checks)
	switch {
	case c.Asked == "":
		return output.New("INVALID_REQUEST", "A record says what the business asked for, and --asked is not there.",
			"Pass --asked \"<what the business asked for, in its own plain words>\".", map[string]any{"name": name})
	case len(c.Checks) == 0:
		return output.New("INVALID_REQUEST", "None of the files is a test, and a record names the tests that prove its change works.",
			"Write the test, then pass it with --check <test>, such as --check custom/server/test/warehouse.test.ts.", map[string]any{"name": name, "files": nonNilStrings(c.Touches)})
	}
	src, err := recordYAML(c)
	if err != nil {
		return err
	}
	if err := safefile.Write(s.env.Dir, file, src, 0o644); err != nil {
		return err
	}
	sql := slices.DeleteFunc(slices.Clone(ch.Uncovered), func(p string) bool { return !custom.WhiskSQL(p) })
	s.printer.Result(map[string]any{"file": file, "change": c, "named": nonNilStrings(ch.Uncovered), "whisk_sql": nonNilStrings(sql)}, func(w io.Writer) {
		fmt.Fprintf(w, "Wrote %s: %s, naming %s and checked by %s.\n", file, c.Name, count(len(c.Touches), "file", "files"), sentence(c.Checks, 10))
		if len(sql) > 0 {
			verb := "are"
			if len(sql) == 1 {
				verb = "is"
			}
			fmt.Fprintf(w, "%s %s Whisk's SQL, which whisk deploy refuses changed (doctor W172): tables of your own go in custom/migrations, and views, functions and triggers in custom/views.\n", sentence(sql, 10), verb)
		}
	})
	return nil
}

// drafted is the record c with the files no record names added to what it touches, what was asked
// when --asked says it, and the checks given; a record with no check yet takes the tests among
// those files. Pure.
func drafted(c custom.Change, uncovered []string, asked string, checks []string) custom.Change {
	out := custom.Change{Name: c.Name, Asked: c.Asked, Touches: appendNew(slices.Clone(c.Touches), uncovered), Checks: appendNew(slices.Clone(c.Checks), checks)}
	if asked != "" {
		out.Asked = asked
	}
	if len(out.Checks) == 0 {
		out.Checks = appendNew(nil, slices.DeleteFunc(slices.Clone(uncovered), func(p string) bool { return !stack.IsTest(p) }))
	}
	return out
}

func appendNew(list, more []string) []string {
	for _, m := range more {
		if !slices.Contains(list, m) {
			list = append(list, m)
		}
	}
	return list
}

// checkProblem is why the test at p cannot be a check, "" when it can.
func checkProblem(dir, p string) string {
	if why := custom.PathProblem(p, false); why != "" {
		return why
	}
	if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil || !info.Mode().IsRegular() {
		return "is not a file in this checkout"
	}
	if !stack.IsTest(p) {
		return "is not a test: a check is a test the app's tests run, such as a *.test.ts file"
	}
	return ""
}

// recordYAML is a record as its file says it: asked, touches and checks, in that order.
func recordYAML(c custom.Change) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	err := enc.Encode(struct {
		Asked   string   `yaml:"asked"`
		Touches []string `yaml:"touches"`
		Checks  []string `yaml:"checks"`
	}{c.Asked, nonNilStrings(c.Touches), nonNilStrings(c.Checks)})
	if err == nil {
		err = enc.Close()
	}
	return b.Bytes(), err
}
