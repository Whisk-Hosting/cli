package doctor

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/whisk-run/contract/custom"
	"github.com/whisk-run/contract/run"
)

// Fork is what a checkout of a fork holds against its product's release line (FORKS.md §9).
type Fork struct {
	// Release is the newest release the checkout's branch holds, as its commit on the line names
	// it ("CRM release 7"), and Base that commit; both are "" when the branch holds none.
	Release string `json:"release"`
	Base    string `json:"base"`
	// Changed is every path that differs from that release in the working tree: added, changed or
	// deleted, committed or not, never what git ignores. Added is those the release does not have.
	Changed []string `json:"changed"`
	Added   []string `json:"added"`
}

// upstreamRef is the remote-tracking branch of a fork's release line: the branch upstream of the
// remote whisk clone names whisk.
const upstreamRef = "refs/remotes/whisk/upstream"

// ReadFork reads the checkout at dir against its release line: nil when it follows none, because its
// remote whisk carries no branch upstream or that branch's newest commit is not a release Whisk
// made. A checkout made before the fork had a release line has none until git fetch whisk.
func ReadFork(ctx context.Context, dir string) (*Fork, error) {
	tip, ok, err := gitAnswer(ctx, dir, "rev-parse", "--verify", "--quiet", upstreamRef+"^{commit}")
	if err != nil || !ok {
		return nil, err
	}
	made, _, err := gitAnswer(ctx, dir, "log", "-1", "--format=%cn%x00%s", tip)
	if by, subject, _ := strings.Cut(made, "\x00"); err != nil || by != custom.LineCommitter || !custom.IsLineSubject(subject) {
		return nil, err
	}
	// A branch with no commit yet, or none shared with the line, holds no release.
	if _, ok, err := gitAnswer(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil || !ok {
		return &Fork{}, err
	}
	if _, ok, err := gitAnswer(ctx, dir, "merge-base", "HEAD", tip); err != nil || !ok {
		return &Fork{}, err
	}
	// The release the branch holds is read from its own history, because git pull fetches only
	// the branch it pulls and leaves the remote-tracking upstream where the last fetch did.
	held, _, err := gitAnswer(ctx, dir, "log", "--committer="+custom.LineCommitter, "--format=%H%x00%cn%x00%s", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	f := &Fork{}
	if f.Base, f.Release = newestRelease(held); f.Base == "" {
		return f, nil
	}
	diffed, err := gitLines(ctx, dir, "diff", "--name-status", "--no-renames", "-z", f.Base, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := gitLines(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	f.Changed, f.Added = nameStatus(diffed)
	f.Changed = append(f.Changed, untracked...)
	f.Added = append(f.Added, untracked...)
	return f, nil
}

// newestRelease is the newest release among the commits git log printed as %H%x00%cn%x00%s, one a
// line: the commit Whisk made whose subject names the highest number, and that subject; "" for
// none. A line's numbers go on through a rename of its product. Pure.
func newestRelease(log string) (base, subject string) {
	newest := 0
	for _, line := range strings.Split(log, "\n") {
		sha, rest, _ := strings.Cut(line, "\x00")
		by, s, _ := strings.Cut(rest, "\x00")
		if _, n, ok := custom.LineRelease(s); ok && by == custom.LineCommitter && n > newest {
			newest, base, subject = n, sha, s
		}
	}
	return base, subject
}

// nameStatus reads git diff --name-status -z, a status and a path at a time: every path, and those
// it added.
func nameStatus(fields []string) (changed, added []string) {
	for i := 0; i+1 < len(fields); i += 2 {
		changed = append(changed, fields[i+1])
		if fields[i] == "A" {
			added = append(added, fields[i+1])
		}
	}
	return changed, added
}

// gitAnswer runs git in dir and answers what it printed, trimmed, and false when it exited saying
// nothing: a revision that is not there (rev-parse --verify --quiet), or a HEAD with no commit or
// none shared with the other (merge-base).
func gitAnswer(ctx context.Context, dir string, args ...string) (string, bool, error) {
	cmd := run.Command(ctx, gitQuick, "git", args...)
	cmd.Dir = dir
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		switch {
		case errors.As(err, &ee) && strings.TrimSpace(stderr.String()) == "":
			return "", false, nil
		case errors.As(err, &ee):
			return "", false, errors.New("git " + strings.Join(args, " ") + ": " + strings.TrimSpace(stderr.String()))
		}
		return "", false, err
	}
	return strings.TrimSpace(out.String()), true, nil
}

// Changes is a fork's checkout as whisk changes lists it: the fork, nil when the checkout follows
// no release line; the changes whose records can be read; the problems of each file of
// custom/changes that is not one, by path; and the files that differ from the release and that no
// record names.
type Changes struct {
	Fork      *Fork
	Changes   []custom.Change
	Problems  map[string][]custom.Problem
	Uncovered []string
}

// ReadChanges reads the checkout at dir as doctor does (W170, W171): its fork, and the records of
// custom/changes git would commit. A directory that is not the top of a repository follows no
// release line.
func ReadChanges(ctx context.Context, dir string) (Changes, error) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return Changes{}, nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return Changes{}, errors.New("git is not installed, and a fork's changes are read with it")
	}
	f, err := ReadFork(ctx, dir)
	if err != nil || f == nil || f.Base == "" {
		return Changes{Fork: f}, err
	}
	paths, err := gitLines(ctx, dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", custom.ChangesDir+"/")
	if err != nil {
		return Changes{}, err
	}
	files := map[string][]byte{}
	for _, p := range paths {
		src, ok, err := recordFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return Changes{}, err
		}
		if ok {
			files[p] = src
		}
	}
	changes, problems := custom.Records(files)
	return Changes{Fork: f, Changes: changes, Problems: problems, Uncovered: custom.Uncovered(f.Changed, changes)}, nil
}

// recordFile reads a file of custom/changes as Whisk reads a record, byte for byte: none for one
// deleted and not committed as deleted yet or one that is not a file, and for one too big to be a
// record a stand-in that is too, so it is said to be without being read.
func recordFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case !info.Mode().IsRegular():
		return nil, false, nil
	case info.Size() > custom.RecordMax:
		return make([]byte, custom.RecordMax+1), true, nil
	}
	src, err := os.ReadFile(path)
	return src, err == nil, err
}

// forkRecords reads the records of a fork's checkout: the changes that can be read and the
// problems of each file of custom/changes that is not one, by path.
func forkRecords(r Repo) ([]custom.Change, map[string][]custom.Problem) {
	files := map[string][]byte{}
	for _, f := range r.Files {
		if custom.InChanges(f.Path) {
			files[f.Path] = recordContent(f)
		}
	}
	return custom.Records(files)
}

// recordContent is a file of custom/changes as Whisk reads a record, byte for byte, where Load
// kept only its text: a file too big to be a record stands in as one, and one Load found binary
// or decoded from UTF-16 as one that is not UTF-8 text. Pure.
func recordContent(f File) []byte {
	switch {
	case f.Size > custom.RecordMax:
		return make([]byte, custom.RecordMax+1)
	case f.Binary || f.UTF16:
		return []byte{0}
	}
	return f.Content
}
