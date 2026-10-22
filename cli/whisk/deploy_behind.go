package whisk

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
)

// behind is what whisk deploy knows about a push refused because the branch on Whisk moved on
// (CLI.md §5.4 step 5, "Catching up with someone else's push").
type behind struct {
	Branch    string   // the branch on Whisk
	Src       string   // the local branch pushed, or HEAD
	Reason    string   // conflict | shared | not_checked_out | rebase_failed | fetch_failed | raced
	Remote    string   // the branch's commit on Whisk; "" when it could not be fetched
	Behind    int      // commits on Whisk this copy lacks
	Ahead     int      // this copy's own commits
	Conflicts []string // files the rebase conflicted in
	Git       []string // git's last lines from the refused push
}

const neverForce = " Never force-push: it deletes the other person's work."

// pushBehind is PUSH_BEHIND for b: what happened in one or two sentences and the git commands
// that bring this copy up to date. Pure.
func pushBehind(b behind) *output.Error {
	msg := fmt.Sprintf("Someone else pushed to %s on Whisk since this copy last pulled", b.Branch)
	if b.Remote != "" {
		msg += fmt.Sprintf(": it is at %s, with %d commit(s) this copy does not have", short(b.Remote), b.Behind)
	}
	msg += "."
	tracking := "whisk/" + b.Branch
	var fix string
	switch b.Reason {
	case "conflict":
		files := listPreview(b.Conflicts, 5)
		msg += fmt.Sprintf(" Rebasing this copy's %d commit(s) onto them conflicts in %s, so nothing here was changed.", b.Ahead, files)
		fix = fmt.Sprintf("Run git rebase %s, resolve the conflicts in %s so both changes still do what they were meant to, git add them and run git rebase --continue, run the tests, then run whisk deploy again.", tracking, files)
	case "shared":
		msg += " This copy's commits are already on another remote, so whisk deploy did not rebase them."
		fix = fmt.Sprintf("Run git merge %s, resolve any conflicts so both changes still do what they were meant to, run the tests, then run whisk deploy again.", tracking)
	case "not_checked_out":
		want := b.Src
		if want == "HEAD" {
			want = "main"
		}
		msg += fmt.Sprintf(" whisk deploy brings only the checked-out branch up to date, and %s is not checked out.", want)
		fix = fmt.Sprintf("Run git switch %s, then whisk deploy again; it brings the branch up to date itself.", want)
	case "raced":
		msg += " It moved again while whisk deploy was catching up."
		fix = "Run whisk deploy again; it brings this copy up to date itself."
	case "fetch_failed":
		fix = fmt.Sprintf("Run git pull --rebase whisk %s, resolve any conflicts so both changes still do what they were meant to, run the tests, then run whisk deploy again.", b.Branch)
	default:
		msg += " Rebasing onto them stopped before it finished, so nothing here was changed."
		fix = fmt.Sprintf("Run git rebase %s and follow what git says, run the tests, then run whisk deploy again.", tracking)
	}
	details := map[string]any{"branch": b.Branch, "reason": b.Reason, "behind": b.Behind, "ahead": b.Ahead}
	if b.Remote != "" {
		details["remote_commit"] = b.Remote
	}
	if len(b.Conflicts) > 0 {
		details["conflicts"] = b.Conflicts
	}
	if len(b.Git) > 0 {
		details["git"] = b.Git
	}
	return output.New("PUSH_BEHIND", msg, fix+neverForce, details)
}

// isBehind is the PUSH_BEHIND a push returned, or nil.
func isBehind(err error) *output.Error {
	var e *output.Error
	if errors.As(err, &e) && e.Code == "PUSH_BEHIND" {
		return e
	}
	return nil
}

// parseLeftRight reads git rev-list --left-right --count A...B: the commits only in A, then
// those only in B. Pure.
func parseLeftRight(out string) (left, right int, ok bool) {
	f := strings.Fields(out)
	if len(f) != 2 {
		return 0, 0, false
	}
	l, err1 := strconv.Atoi(f[0])
	r, err2 := strconv.Atoi(f[1])
	return l, r, err1 == nil && err2 == nil
}

// catchUp brings this copy up to date after a push refused as behind: it fetches the branch
// from Whisk and rebases the pushed branch's commits onto it, and returns the commit to push.
// Anything it will not or cannot do is PUSH_BEHIND, with this copy left as it was.
func (s *session) catchUp(secret, gitURL string, plan pushPlan, current string, refused *output.Error) (string, error) {
	b := behind{Branch: plan.Dst, Src: plan.Src, Reason: "fetch_failed"}
	if lines, ok := refused.Details["git"].([]string); ok {
		b.Git = lines
	}
	env, cargs, err := gitcmd.Credential(secret, gitURL)
	if err != nil {
		return "", pushBehind(b)
	}
	tracking := "refs/remotes/whisk/" + plan.Dst
	s.printer.Progress("%s on Whisk has commits this copy does not have; fetching them.", plan.Dst)
	fetched, err := s.gitRun(env, gitcmd.Networked(cargs, "fetch", "--quiet", "whisk", "+refs/heads/"+plan.Dst+":"+tracking)...)
	if err != nil {
		return "", err
	}
	if !fetched.OK() {
		b.Git = gitcmd.LastLines(fetched.Stderr, 10)
		return "", pushBehind(b)
	}
	if res, err := s.gitRun(nil, "rev-parse", "--verify", "--quiet", tracking); err != nil {
		return "", err
	} else if res.OK() {
		b.Remote = res.Out()
	}
	if res, err := s.gitRun(nil, "rev-list", "--left-right", "--count", tracking+"..."+plan.Src); err != nil {
		return "", err
	} else if l, r, ok := parseLeftRight(res.Out()); ok {
		b.Behind, b.Ahead = l, r
	}
	if current == "" || plan.Src != current {
		b.Reason = "not_checked_out"
		return "", pushBehind(b)
	}
	// Commits another remote already holds are someone's history; rebasing them would fork it.
	unshared, err := s.gitRun(nil, "rev-list", "--count", plan.Src, "--not", "--remotes")
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(unshared.Out()); !unshared.OK() || err != nil || n < b.Ahead {
		b.Reason = "shared"
		return "", pushBehind(b)
	}
	quiet := []string{"GIT_EDITOR=true"}
	rebased, err := s.gitRun(quiet, append(s.identityArgs(), "rebase", "--quiet", tracking)...)
	if err != nil {
		return "", err
	}
	if !rebased.OK() {
		b.Reason = "rebase_failed"
		if res, err := s.gitRun(nil, "diff", "--name-only", "--diff-filter=U"); err == nil && res.OK() && res.Out() != "" {
			b.Reason, b.Conflicts = "conflict", strings.Split(res.Out(), "\n")
		}
		if _, err := s.gitRun(quiet, "rebase", "--abort"); err != nil {
			return "", err
		}
		if b.Reason == "rebase_failed" {
			b.Git = gitcmd.LastLines(rebased.Stderr+rebased.Stdout, 10)
		}
		return "", pushBehind(b)
	}
	head, err := s.gitRun(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return "", err
	}
	if !head.OK() {
		return "", gitFailed("git rev-parse HEAD", head)
	}
	s.caughtUp = map[string]any{"branch": plan.Dst, "remote_commit": b.Remote, "behind": b.Behind, "rebased": b.Ahead, "commit_sha": head.Out()}
	if b.Ahead == 0 {
		s.printer.Progress("%s on Whisk had %d new commit(s) from someone else; this copy had none of its own and now matches it, at %s.", plan.Dst, b.Behind, short(head.Out()))
	} else {
		s.printer.Progress("%s on Whisk had %d new commit(s) from someone else; rebased this copy's %d commit(s) onto them, now %s.", plan.Dst, b.Behind, b.Ahead, short(head.Out()))
	}
	return head.Out(), nil
}
