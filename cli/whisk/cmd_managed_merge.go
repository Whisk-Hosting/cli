package whisk

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/custom"
)

// whisk managed merge (FORKS.md §4): a checkout of a fork with changes of the business's own takes
// its product's newest release. What the release changed is joined with the records before the
// merge (custom.Meets), so a change the release does not meet carries over with nobody looking,
// one it meets is named with what it asked for and its checks, and a file left in conflict is
// named with the changes it belongs to.

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
conflict leaves the merge for you to settle and names the changes each conflicted file belongs to
(MERGE_CONFLICT); git merge --abort puts the branch back as it was.

Then run the checks it names and the app's tests, and whisk deploy, which tries the release's
migrations on a copy of the live data before they reach it.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return runManagedMerge(s) },
	}
}

// metChange is a change the release meets: what it asked for, the files of it the release changed
// too, and its checks.
type metChange struct {
	Name   string   `json:"name"`
	Asked  string   `json:"asked"`
	Files  []string `json:"files"`
	Checks []string `json:"checks"`
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

// mergePlan joins released, the files the release changed, with the branch's records and the files
// it changed that no record names (doctor.Changes.Uncovered). Pure.
func mergePlan(changes []custom.Change, uncovered, released []string) mergeJoin {
	j := mergeJoin{Met: []metChange{}, Carried: []string{}, Checks: []string{}, Unrecorded: []string{}}
	for _, c := range changes {
		files := custom.Meets(c, released)
		if len(files) == 0 {
			j.Carried = append(j.Carried, c.Name)
			continue
		}
		j.Met = append(j.Met, metChange{Name: c.Name, Asked: c.Asked, Files: files, Checks: c.Checks})
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

// conflict is a file the merge left for a person or agent to settle, and the changes whose
// records name it.
type conflict struct {
	File    string      `json:"file"`
	Changes []metChange `json:"changes"`
}

// conflictsOf names each conflicted file with the changes it belongs to. A file conflicts only when
// the branch changed it too, so one no record names is a change of the branch's without one. Pure.
func conflictsOf(files []string, changes []custom.Change) []conflict {
	out := make([]conflict, 0, len(files))
	for _, f := range files {
		cf := conflict{File: f, Changes: []metChange{}}
		for _, c := range custom.Naming(changes, f) {
			cf.Changes = append(cf.Changes, metChange{Name: c.Name, Asked: c.Asked, Files: []string{f}, Checks: c.Checks})
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
	join := mergePlan(before.Changes, before.Uncovered, released)
	s.printer.Progress("Merging %s into %s, after %s.", subject, branch, before.Fork.Release)
	res, err := s.gitRun(nil, append(s.identityArgs(), "merge", "--no-edit", "-m", mergeMessage(product, number), tip)...)
	if err != nil {
		return err
	}
	if !res.OK() {
		unsettled, err := s.gitNames("diff", "--name-only", "--diff-filter=U", "-z")
		if err != nil || len(unsettled) == 0 {
			return gitFailed("git merge", res)
		}
		return s.mergeConflict(subject, before.Fork.Release, conflictsOf(unsettled, before.Changes))
	}
	head, err := s.gitOut("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	s.printer.Result(map[string]any{"merged": true, "release": subject, "from": before.Fork.Release, "commit": head,
		"met": join.Met, "carried": join.Carried, "checks": join.Checks, "unrecorded": join.Unrecorded},
		func(w io.Writer) { printMerged(s.printer, w, subject, before.Fork.Release, join) })
	return nil
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
// belongs to are said on stderr.
func (s *session) mergeConflict(release, from string, conflicts []conflict) error {
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
	return output.New("MERGE_CONFLICT", fmt.Sprintf("Merging %s left %d file(s) to settle: %s.", release, len(files), listPreview(files, 5)),
		"Settle each file so the release and the change it belongs to both still do what they were for (details.conflicts names each change with what it asked for), git add the files and git commit, run the checks, then whisk deploy. git merge --abort puts the branch back as it was.",
		map[string]any{"release": release, "from": from, "conflicts": conflicts})
}

func printMerged(p output.Printer, w io.Writer, release, from string, j mergeJoin) {
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
		fmt.Fprintf(w, "\n%s\n  %s\n  %s\n  %s\n", p.Bold(c.Name), c.Asked, p.Dim("the release changed "+strings.Join(c.Files, ", ")), p.Dim("checked by "+strings.Join(c.Checks, ", ")))
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

func touchVerb(n int) string {
	if n == 1 {
		return "touches"
	}
	return "touch"
}
