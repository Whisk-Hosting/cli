package whisk

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/doctor"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/custom"
)

// whisk managed merge (FORKS.md §4, §10): a checkout of a fork with changes of the business's own
// takes its product's newest release. What the release changed, and its notes on what it moved,
// are joined with the records and the business's own lines before the merge (custom.Meets,
// custom.NoteUses), so a change the release does not meet carries over with nobody looking, one
// it meets is named with what it asked for, its checks and the notes its lines need, and a file
// left in conflict is named with the changes it belongs to. A change Whisk took into the release
// is swapped for Whisk's version, so the branch stops carrying it (returnedOf).

func managedMergeCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "merge",
		Short: "Bring a fork's newest release into this checkout, naming each of the business's changes it meets",
		Long: `In a checkout of an app that follows one of Whisk's products as the business's own code (whisk
clone makes one), on a branch with nothing uncommitted. It fetches the product's newest release
from the remote whisk and merges it into the branch. Before merging it joins the files the release
changed with the business's records in custom/changes: each change whose files the release changed
too is named with what it asked for and the checks that prove it, every other change carries over
as it is, and a file the release changed that differs here with no record naming it is named. A
change whose own lines use a name the release's notes in moved/ say it removed or renamed is named
too, with the note (where it went, what became of the values, the fix) and those lines. A change
Whisk took into the release, or an earlier one, as the business's newest suggestion of it says, is
swapped for Whisk's version when its files are the ones that suggestion sent, as they were sent:
each file only it names is put as the release has it and its record is taken away, so the branch
no longer carries it, and the merge says what Whisk said becomes of its data. A conflict leaves
the merge for you to settle and names the changes each conflicted file belongs to
(MERGE_CONFLICT); git merge --abort puts the branch back as it was.

Then run the checks it names and the app's tests, and whisk deploy, which tries the release's
migrations on a copy of the live data before they reach it.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return runManagedMerge(s) },
	}
}

// metChange is a change the release meets: what it asked for, the files of it the release changed
// too, the release's notes on names it moved that the change's own lines use, and its checks.
type metChange struct {
	Name   string    `json:"name"`
	Asked  string    `json:"asked"`
	Files  []string  `json:"files"`
	Moved  []metNote `json:"moved"`
	Checks []string  `json:"checks"`
}

// metNote is a note of the release on what it moved, and the lines of a change that use a name it
// says moved.
type metNote struct {
	custom.Moved
	Uses []custom.Use `json:"uses"`
}

// mergeJoin is what a release changed joined with the business's changes: those it meets, the
// names of those it does not, the checks of the ones it meets, each once, and the files it changed
// that differ here too with no record naming them, whose purpose nobody can be told.
type mergeJoin struct {
	Met        []metChange `json:"met"`
	Carried    []string    `json:"carried"`
	Checks     []string    `json:"checks"`
	Unrecorded []string    `json:"unrecorded"`
}

// mergePlan joins released, the files the release changed, and notes, its notes on what it moved,
// with the branch's records, the business's own lines in the files they name, and the files the
// branch changed that no record names (doctor.Changes.Uncovered). Pure.
func mergePlan(changes []custom.Change, uncovered, released []string, notes []custom.Moved, lines map[string][]custom.Line) mergeJoin {
	j := mergeJoin{Met: []metChange{}, Carried: []string{}, Checks: []string{}, Unrecorded: []string{}}
	for _, c := range changes {
		files, moved := custom.Meets(c, released), notesMet(c, notes, lines)
		if len(files) == 0 && len(moved) == 0 {
			j.Carried = append(j.Carried, c.Name)
			continue
		}
		j.Met = append(j.Met, metChange{Name: c.Name, Asked: c.Asked, Files: nonEmpty(files), Moved: moved, Checks: c.Checks})
		j.Checks = append(j.Checks, c.Checks...)
	}
	slices.Sort(j.Checks)
	j.Checks = slices.Compact(j.Checks)
	for _, p := range released {
		if slices.Contains(uncovered, p) {
			j.Unrecorded = append(j.Unrecorded, p)
		}
	}
	return j
}

// notesMet is each of notes, with the lines of the change's own files that use a name it says
// moved, when there are any. Pure.
func notesMet(c custom.Change, notes []custom.Moved, lines map[string][]custom.Line) []metNote {
	own := map[string][]custom.Line{}
	for _, p := range custom.Meets(c, slices.Sorted(maps.Keys(lines))) {
		own[p] = lines[p]
	}
	out := []metNote{}
	for _, n := range notes {
		if uses := custom.NoteUses(n, own); len(uses) > 0 {
			out = append(out, metNote{n, uses})
		}
	}
	return out
}

// returnedChange is a change on record that Whisk took into a release no later than the one
// merged, as the business's newest suggestion of it says (FORKS.md §10): the release that brings
// Whisk's version, what Whisk said becomes of the business's data, and what the merge did with it.
// A change whose files are the ones its suggestion sent, as they were sent, is swapped: each file
// of it no other change names (Files) is put as the release has it and its record is taken away,
// so the branch no longer carries it, while one another change names too, in what it touches or
// in its checks (Kept), stays as the merge makes it. One that changed since it was sent is not
// swapped, and the merge meets it as any other change.
type returnedChange struct {
	Name    string   `json:"name"`
	Asked   string   `json:"asked"`
	Release int      `json:"release"`
	Moves   string   `json:"moves"`
	Swapped bool     `json:"swapped"`
	Files   []string `json:"files"`
	Kept    []string `json:"kept"`
}

// returnedOf is each of changes whose newest suggestion, in suggestions newest first, Whisk took
// into a release no later than number, with the files of changed, the paths that differ from the
// release the branch holds, that it names; sentAsIs reports whether those files are the ones the
// suggestion sent, as they were sent. Pure when sentAsIs is.
func returnedOf(changes []custom.Change, changed []string, suggestions []apitypes.ForkSuggestion, number int, sentAsIs func(apitypes.ForkSuggestion, []string) bool) []returnedChange {
	newest := map[string]apitypes.ForkSuggestion{}
	for _, sg := range suggestions {
		if _, ok := newest[sg.Change]; !ok {
			newest[sg.Change] = sg
		}
	}
	out := []returnedChange{}
	for _, c := range changes {
		sg, ok := newest[c.Name]
		if !ok || sg.Status != apitypes.SuggestionTaken || sg.TakenIn > number {
			continue
		}
		files := slices.DeleteFunc(custom.Meets(c, changed), custom.Exempt)
		r := returnedChange{Name: c.Name, Asked: c.Asked, Release: sg.TakenIn, Moves: sg.Moves, Swapped: sentAsIs(sg, files), Files: []string{}, Kept: []string{}}
		for _, p := range files {
			if slices.ContainsFunc(changes, func(o custom.Change) bool {
				return o.Name != c.Name && (len(custom.Meets(o, []string{p})) > 0 || slices.Contains(o.Checks, p))
			}) {
				r.Kept = append(r.Kept, p)
			} else {
				r.Files = append(r.Files, p)
			}
		}
		out = append(out, r)
	}
	return out
}

// swappedOut is changes but those returned swaps for Whisk's version. Pure.
func swappedOut(changes []custom.Change, returned []returnedChange) []custom.Change {
	return slices.DeleteFunc(slices.Clone(changes), func(c custom.Change) bool {
		return slices.ContainsFunc(returned, func(r returnedChange) bool { return r.Swapped && r.Name == c.Name })
	})
}

func nonEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// conflict is a file the merge left for a person or agent to settle, and the changes whose
// records name it.
type conflict struct {
	File    string      `json:"file"`
	Changes []metChange `json:"changes"`
}

// conflictsOf names each conflicted file with the changes it belongs to, each with the notes the
// join found its lines use. A file conflicts only when the branch changed it too, so one no record
// names is a change of the branch's without one. Pure.
func conflictsOf(files []string, changes []custom.Change, j mergeJoin) []conflict {
	out := make([]conflict, 0, len(files))
	for _, f := range files {
		cf := conflict{File: f, Changes: []metChange{}}
		for _, c := range custom.Naming(changes, f) {
			moved := []metNote{}
			if i := slices.IndexFunc(j.Met, func(m metChange) bool { return m.Name == c.Name }); i >= 0 {
				moved = j.Met[i].Moved
			}
			cf.Changes = append(cf.Changes, metChange{Name: c.Name, Asked: c.Asked, Files: []string{f}, Moved: moved, Checks: c.Checks})
		}
		out = append(out, cf)
	}
	return out
}

// mergeMessage is the merge commit's subject, which never reads as a commit of the release line
// (custom.LineRelease), however git is told who made it.
func mergeMessage(product string, number int) string {
	return fmt.Sprintf("Merge release %d of %s", number, product)
}

func runManagedMerge(s *session) error {
	before, err := s.forkChanges("whisk managed merge")
	if err != nil {
		return err
	}
	if err := s.cleanForMerge(); err != nil {
		return err
	}
	branch, err := s.gitOut("branch", "--show-current")
	if err != nil {
		return err
	}
	if branch == "" {
		return output.New("INVALID_REQUEST", "This checkout is on no branch: its HEAD is detached.",
			"Check out the branch to bring the release into, such as git switch main, then run whisk managed merge again.", nil)
	}
	org, app, err := s.target()
	if err != nil {
		return err
	}
	client, cred, err := s.client()
	if err != nil {
		return err
	}
	a, err := client.GetApp(s.ctx, org, app)
	if err != nil {
		return wrap(err)
	}
	if a.Fork == nil {
		return output.New("INVALID_REQUEST", fmt.Sprintf("%s/%s does not follow one of Whisk's products.", org, app),
			"Run whisk managed merge in a checkout of a fork, which whisk clone <org>/<app> makes.", map[string]any{"app": app})
	}
	if err := s.checkBoundRemote(org, app, a.GitURL); err != nil {
		return err
	}
	secret, err := s.gitSecret(client, cred)
	if err != nil {
		return err
	}
	env, cargs, err := gitcmd.Credential(secret, a.GitURL)
	if err != nil {
		return output.New("PLATFORM_UNAVAILABLE", err.Error()+".", "Check WHISK_API points at the Whisk platform you meant, then retry.", map[string]any{"git_url": a.GitURL})
	}
	line := "refs/remotes/whisk/" + a.Fork.Branch
	s.printer.Progress("Fetching %s's releases from %s/%s.", a.Fork.Name, org, app)
	if res, err := s.gitRun(env, gitcmd.Networked(cargs, "fetch", "--quiet", "whisk", "+refs/heads/"+a.Fork.Branch+":"+line)...); err != nil {
		return err
	} else if !res.OK() {
		return gitFailed("git fetch", res)
	}
	tip, err := s.gitOut("rev-parse", "--verify", "--quiet", line+"^{commit}")
	if err != nil {
		return err
	}
	subject, err := s.gitOut("log", "-1", "--format=%s", tip)
	if err != nil {
		return err
	}
	product, number, ok := custom.LineRelease(subject)
	if !ok {
		return output.New("INVALID_REQUEST", fmt.Sprintf("The newest commit of whisk/%s is not a release Whisk made.", a.Fork.Branch),
			fmt.Sprintf("Point the remote whisk at %s/%s's repository (whisk use %s/%s does), then run whisk managed merge again; if it points there already, tell Whisk, which moves that branch only to its releases.", org, app, org, app),
			map[string]any{"commit": tip})
	}
	if res, err := s.gitRun(nil, "merge-base", "--is-ancestor", tip, "HEAD"); err != nil {
		return err
	} else if res.OK() {
		s.printer.Result(map[string]any{"merged": false, "release": subject, "from": before.Fork.Release}, func(w io.Writer) {
			fmt.Fprintf(w, "This branch holds %s, the newest release: there is nothing to merge.\n", subject)
		})
		return nil
	}
	released, err := s.gitNames("diff", "--name-only", "--no-renames", "-z", before.Fork.Base, tip, "--")
	if err != nil {
		return err
	}
	notes, err := s.releaseNotes(tip, released)
	if err != nil {
		return err
	}
	lines, err := s.ownLines(before)
	if err != nil {
		return err
	}
	returned := []returnedChange{}
	if len(before.Changes) > 0 {
		list, err := client.ForkSuggestions(s.ctx, org, app)
		if err != nil {
			return wrap(err)
		}
		returned = returnedOf(before.Changes, before.Fork.Changed, list.Suggestions, number, s.sentAsIs)
	}
	carried := swappedOut(before.Changes, returned)
	join := mergePlan(carried, before.Uncovered, released, notes, lines)
	s.printer.Progress("Merging %s into %s, after %s.", subject, branch, before.Fork.Release)
	res, err := s.mergeRelease(tip, mergeMessage(product, number), returned)
	if err != nil {
		return err
	}
	if !res.OK() {
		unsettled, err := s.gitNames("diff", "--name-only", "--diff-filter=U", "-z")
		if err != nil || len(unsettled) == 0 {
			return gitFailed("git merge", res)
		}
		return s.mergeConflict(subject, before.Fork.Release, conflictsOf(unsettled, carried, join), join, returned)
	}
	head, err := s.gitOut("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	s.printer.Result(map[string]any{"merged": true, "release": subject, "from": before.Fork.Release, "commit": head,
		"met": join.Met, "carried": join.Carried, "checks": join.Checks, "unrecorded": join.Unrecorded, "returned": returned},
		func(w io.Writer) { printMerged(s.printer, w, subject, before.Fork.Release, join, returned) })
	return nil
}

// sentAsIs reports whether files, the files of a change that differ from the release the branch
// holds, are the ones its suggestion sent, as they were sent: none it did not send, and nothing it
// sent, the record among them, changed since the commit it was sent from. A checkout without that
// commit cannot tell, so it is not.
func (s *session) sentAsIs(sg apitypes.ForkSuggestion, files []string) bool {
	if sg.Commit == "" || len(sg.Files) == 0 || slices.ContainsFunc(files, func(p string) bool { return !slices.Contains(sg.Files, p) }) {
		return false
	}
	res, err := s.gitRun(nil, append([]string{"--literal-pathspecs", "diff", "--quiet", "--no-ext-diff", "--no-renames", sg.Commit, "HEAD", "--"}, sg.Files...)...)
	return err == nil && res.OK()
}

// mergeRelease merges the release at tip into the branch. With a change to swap for Whisk's
// version, the merge stops before it commits: each file of the change no other change names is put
// as the release has it, or taken away when the release has no such file, the change's record is
// taken away, and the merge is committed once nothing is left to settle. A file left to settle
// leaves the merge under way, as a conflict does without a swap, so git merge --abort still puts
// the branch back as it was.
func (s *session) mergeRelease(tip, message string, returned []returnedChange) (gitcmd.Result, error) {
	swaps := slices.DeleteFunc(slices.Clone(returned), func(r returnedChange) bool { return !r.Swapped })
	if len(swaps) == 0 {
		return s.gitRun(nil, append(s.identityArgs(), "merge", "--no-edit", "-m", message, tip)...)
	}
	res, err := s.gitRun(nil, append(s.identityArgs(), "merge", "--no-ff", "--no-commit", "-m", message, tip)...)
	if err != nil {
		return res, err
	}
	if under, err := s.gitRun(nil, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err != nil || !under.OK() {
		return res, err
	}
	for _, r := range swaps {
		for _, p := range append(slices.Clone(r.Files), custom.RecordPath(r.Name)) {
			if err := s.takeRelease(tip, p); err != nil {
				return res, err
			}
		}
	}
	if unsettled, err := s.gitNames("diff", "--name-only", "--diff-filter=U", "-z"); err != nil || len(unsettled) > 0 {
		return gitcmd.Result{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: 1}, err
	}
	return s.gitRun(nil, append(s.identityArgs(), "commit", "--quiet", "--no-edit")...)
}

// takeRelease puts the file at p as the release at tip has it, in the merge under way, or takes it
// away when the release has no such file.
func (s *session) takeRelease(tip, p string) error {
	has, err := s.gitRun(nil, "cat-file", "-e", tip+":"+p)
	if err != nil {
		return err
	}
	args := []string{"--literal-pathspecs", "rm", "--quiet", "--force", "--ignore-unmatch", "--", p}
	if has.OK() {
		args = []string{"--literal-pathspecs", "checkout", tip, "--", p}
	}
	res, err := s.gitRun(nil, args...)
	if err == nil && !res.OK() {
		err = gitFailed("git "+args[1], res)
	}
	return err
}

// cleanForMerge refuses a checkout with anything uncommitted or a merge already under way, so a
// merge that stops can always be put back with git merge --abort.
func (s *session) cleanForMerge() error {
	if res, err := s.gitRun(nil, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); err != nil {
		return err
	} else if res.OK() {
		return output.New("INVALID_REQUEST", "A merge is under way in this checkout.",
			"Finish it (settle the files git status names, git add them and git commit) or put it back with git merge --abort, then run whisk managed merge again.", nil)
	}
	res, err := s.gitRun(nil, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	if !res.OK() {
		return gitFailed("git status", res)
	}
	if dirty := gitcmd.ParseStatus(res.Stdout); len(dirty) > 0 {
		return output.New("INVALID_REQUEST", fmt.Sprintf("%d file(s) have uncommitted changes: %s.", len(dirty), listPreview(dirty, 5)),
			"Commit them (or git stash them), then run whisk managed merge again.", map[string]any{"uncommitted": dirty})
	}
	return nil
}

// gitOut runs git in the checkout and answers what it printed, trimmed; a git that fails is an
// error.
func (s *session) gitOut(args ...string) (string, error) {
	res, err := s.gitRun(nil, args...)
	if err != nil {
		return "", err
	}
	if !res.OK() {
		return "", gitFailed("git "+args[0], res)
	}
	return strings.TrimSpace(res.Stdout), nil
}

// releaseNotes reads the release's notes on what moved that it added or changed since the one the
// branch holds, as the release has them; one it took away, or one that cannot be read, says
// nothing, as Whisk's source cannot deploy one (doctor W174).
func (s *session) releaseNotes(tip string, released []string) ([]custom.Moved, error) {
	var out []custom.Moved
	for _, p := range released {
		id, ok := custom.MovedID(p)
		if !ok {
			continue
		}
		res, err := s.gitRun(nil, "cat-file", "blob", tip+":"+p)
		if err != nil {
			return nil, err
		}
		if !res.OK() {
			continue
		}
		if n, ps := custom.ParseMoved(id, []byte(res.Stdout)); len(ps) == 0 {
			out = append(out, n)
		}
	}
	return out, nil
}

// ownLineMax caps a file of the business's own read for the names a release moved, in bytes.
const ownLineMax = 1 << 20

// ownLines is the business's own lines in each file a record names that differs from the release
// the branch holds: every line of a file the business added, and the lines it added to one of the
// release's. A file too big or not text says nothing.
func (s *session) ownLines(ch doctor.Changes) (map[string][]custom.Line, error) {
	out := map[string][]custom.Line{}
	for _, p := range ch.Fork.Changed {
		if !slices.ContainsFunc(ch.Changes, func(c custom.Change) bool { return len(custom.Meets(c, []string{p})) > 0 }) {
			continue
		}
		if slices.Contains(ch.Fork.Added, p) {
			if body, ok := smallText(filepath.Join(s.env.Dir, filepath.FromSlash(p))); ok {
				out[p] = custom.FileLines(body)
			}
			continue
		}
		res, err := s.gitRun(nil, "diff", "--no-color", "--no-ext-diff", "--no-renames", "-U0", ch.Fork.Base, "--", p)
		if err != nil {
			return nil, err
		}
		if res.OK() && len(res.Stdout) <= ownLineMax {
			for _, added := range custom.PatchAdded(res.Stdout) {
				out[p] = append(out[p], added...)
			}
		}
	}
	return out, nil
}

// smallText is the file at path when it is text of at most ownLineMax bytes.
func smallText(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > ownLineMax {
		return nil, false
	}
	body, err := os.ReadFile(path)
	return body, err == nil && utf8.Valid(body) && !slices.Contains(body, 0)
}

// gitNames runs git with -z among args and answers the paths it printed, each as git spelled it.
func (s *session) gitNames(args ...string) ([]string, error) {
	res, err := s.gitRun(nil, args...)
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, gitFailed("git "+args[0], res)
	}
	return strings.FieldsFunc(res.Stdout, func(r rune) bool { return r == 0 }), nil
}

// mergeConflict is MERGE_CONFLICT for a merge left with files to settle, after the changes each
// belongs to are said on stderr; its details carry the join too, with the notes on what moved, and
// the changes Whisk took, swapped in the merge under way or not.
func (s *session) mergeConflict(release, from string, conflicts []conflict, j mergeJoin, returned []returnedChange) error {
	files := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		files = append(files, c.File)
		if len(c.Changes) == 0 {
			s.printer.Progress("%s was changed here, and no record in custom/changes names it.", c.File)
		}
		for _, ch := range c.Changes {
			s.printer.Progress("%s belongs to %s: %s Checked by %s.", c.File, ch.Name, ch.Asked, strings.Join(ch.Checks, ", "))
		}
	}
	if !s.printer.Quiet {
		for _, c := range j.Met {
			if len(c.Moved) > 0 {
				fmt.Fprintf(s.printer.Err, "%s uses what %s moved:\n", c.Name, release)
			}
			for _, n := range c.Moved {
				printNote(s.printer, s.printer.Err, n)
			}
		}
		for _, r := range returned {
			printReturned(s.printer, s.printer.Err, r)
		}
	}
	return output.New("MERGE_CONFLICT", fmt.Sprintf("Merging %s left %d file(s) to settle: %s.", release, len(files), listPreview(files, 5)),
		"Settle each file so the release and the change it belongs to both still do what they were for (details.conflicts names each change with what it asked for), git add the files and git commit, run the checks, then whisk deploy. git merge --abort puts the branch back as it was.",
		map[string]any{"release": release, "from": from, "conflicts": conflicts, "met": j.Met, "unrecorded": j.Unrecorded, "returned": returned})
}

func printMerged(p output.Printer, w io.Writer, release, from string, j mergeJoin, returned []returnedChange) {
	total := len(j.Met) + len(j.Carried)
	switch {
	case total == 0:
		fmt.Fprintf(w, "Merged %s, after %s. The branch has no change of the business's own on record.\n", release, from)
	case len(j.Met) == 0:
		fmt.Fprintf(w, "Merged %s, after %s. The business's %s %s no file the release changed.\n", release, from, count(total, "change", "changes"), touchVerb(total))
	default:
		fmt.Fprintf(w, "Merged %s, after %s. %d of the business's %s %s files the release changed too:\n", release, from, len(j.Met), count(total, "change", "changes"), touchVerb(len(j.Met)))
	}
	for _, c := range j.Met {
		fmt.Fprintf(w, "\n%s\n  %s\n", p.Bold(c.Name), c.Asked)
		if len(c.Files) > 0 {
			fmt.Fprintf(w, "  %s\n", p.Dim("the release changed "+strings.Join(c.Files, ", ")))
		}
		for _, n := range c.Moved {
			printNote(p, w, n)
		}
		fmt.Fprintf(w, "  %s\n", p.Dim("checked by "+strings.Join(c.Checks, ", ")))
	}
	if len(returned) > 0 {
		fmt.Fprintf(w, "\nWhisk took %s of the business's into its releases:\n", count(len(returned), "change", "changes"))
	}
	for _, r := range returned {
		printReturned(p, w, r)
	}
	if len(j.Unrecorded) > 0 {
		fmt.Fprintf(w, "\nNo record names %s, which the release and this branch both changed: whisk changes draft <name> writes one.\n", listPreview(j.Unrecorded, 10))
	}
	fmt.Fprintln(w)
	if len(j.Checks) > 0 {
		fmt.Fprintf(w, "Next: run %s and the app's tests, then whisk deploy, which tries the release's migrations on a copy of the live data first.\n", sentence(j.Checks, 10))
		return
	}
	fmt.Fprintln(w, "Next: run the app's tests, then whisk deploy, which tries the release's migrations on a copy of the live data first.")
}

// noteUsesShown caps the lines of a change printed for one note.
const noteUsesShown = 10

// printNote says what a note says moved, what became of it and the fix, then the change's lines
// that use it.
func printNote(p output.Printer, w io.Writer, n metNote) {
	went := make([]string, 0, len(n.Moves))
	for _, mv := range n.Moves {
		if mv.To == "" {
			went = append(went, mv.From+" went away")
		} else {
			went = append(went, mv.From+" went to "+mv.To)
		}
	}
	said := slices.DeleteFunc([]string{sentence(went, 10) + ".", n.Values, n.Fix}, func(s string) bool { return s == "" })
	fmt.Fprintf(w, "  %s: %s\n", n.ID, strings.Join(said, " "))
	for i, u := range n.Uses {
		if i == noteUsesShown {
			fmt.Fprintf(w, "    %s\n", p.Dim(fmt.Sprintf("and %d more", len(n.Uses)-noteUsesShown)))
			break
		}
		fmt.Fprintf(w, "    %s  %s\n", p.Dim(fmt.Sprintf("%s:%d", u.File, u.Line)), u.Text)
	}
}

// printReturned says what the merge did with a change Whisk took: swapped for Whisk's version,
// with any file another change names too, or carried still because it changed since it was
// suggested; then what Whisk said becomes of its data.
func printReturned(p output.Printer, w io.Writer, r returnedChange) {
	fmt.Fprintf(w, "\n%s %s\n  %s\n", p.Bold(r.Name), p.Dim(fmt.Sprintf("in release %d", r.Release)), r.Asked)
	switch {
	case r.Swapped && len(r.Files) > 0:
		fmt.Fprintf(w, "  The branch no longer carries it: %s %s as release %d has %s, and its record is gone.\n", sentence(r.Files, 10), isVerb(len(r.Files)), r.Release, them(len(r.Files)))
	case r.Swapped:
		fmt.Fprintln(w, "  The branch no longer carries it: its record is gone.")
	default:
		fmt.Fprintf(w, "  It changed after it was suggested, so the branch still carries it: once release %d's version does what it is for, put its files as the release has them and delete %s.\n", r.Release, custom.RecordPath(r.Name))
	}
	if r.Swapped && len(r.Kept) > 0 {
		fmt.Fprintf(w, "  %s %s another change's too, so take %s's lines out of %s by hand.\n", sentence(r.Kept, 10), isVerb(len(r.Kept)), r.Name, them(len(r.Kept)))
	}
	if r.Moves != "" {
		fmt.Fprintf(w, "  %s %s\n", p.Dim("its data:"), r.Moves)
	}
}

func isVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func touchVerb(n int) string {
	if n == 1 {
		return "touches"
	}
	return "touch"
}
