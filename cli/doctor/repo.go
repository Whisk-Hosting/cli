package doctor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitignore "github.com/sabhiram/go-gitignore"
	"gopkg.in/yaml.v3"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/cli/internal/textenc"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/run"
)

// File is one file doctor may read. Content is nil for binaries and files over the cap.
type File struct {
	Path    string // slash separated, relative to the root
	Content []byte
	Tracked bool
	Size    int64
	Binary  bool
	// UTF16 is a file saved as UTF-16 (Windows PowerShell's >); Content holds it as UTF-8.
	UTF16 bool
}

// Repo is everything the rules look at, loaded once. The rules are pure functions of it.
type Repo struct {
	Dir   string
	Files []File
	IsGit bool // a .git exists at the root
	GitOK bool // the git binary answers, so tracked-ness and size are exact
	Stack stack.Stack

	ManifestSrc  []byte
	ManifestNode *yaml.Node
	Manifest     manifest.Manifest
	ManifestErr  error // nil when the manifest parsed and validated
	HasManifest  bool

	Binding *config.Binding
	// WhiskRemote is the URL of the git remote named whisk, "" when there is none or the
	// directory is not a repository (W006).
	WhiskRemote string
}

const contentCap = 4 << 20

// Load reads a directory into a Repo.
func Load(ctx context.Context, dir string) (Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Repo{}, err
	}
	r := Repo{Dir: abs}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
		r.IsGit = true
	}
	if _, err := exec.LookPath("git"); err == nil {
		r.GitOK = true
	}
	paths, tracked, err := listFiles(ctx, r)
	if err != nil {
		return r, err
	}
	for _, p := range paths {
		f := File{Path: p, Tracked: tracked[p]}
		info, err := os.Lstat(filepath.Join(abs, filepath.FromSlash(p)))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		f.Size = info.Size()
		if f.Size <= contentCap {
			content, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(p)))
			if err == nil {
				// What Windows tools write (a byte-order mark, UTF-16) reads as the text it is.
				decoded := textenc.Decode(content)
				f.UTF16 = len(content) >= 2 && (content[0] == 0xFF && content[1] == 0xFE || content[0] == 0xFE && content[1] == 0xFF)
				content = decoded
				if isBinary(content) {
					f.Binary = true
				} else {
					f.Content = content
				}
			}
		}
		r.Files = append(r.Files, f)
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	r.Stack = stack.Detect(paths, func(p string) []byte { return r.Read(p) })
	if src := r.Read("whisk.yaml"); src != nil {
		r.HasManifest = true
		r.ManifestSrc = src
		var node yaml.Node
		if err := yaml.Unmarshal(src, &node); err == nil {
			r.ManifestNode = &node
		}
		r.Manifest, r.ManifestErr = manifest.Parse(src)
	}
	if b, ok, err := config.LoadBinding(abs); err == nil && ok {
		r.Binding = &b
	}
	if r.IsGit && r.GitOK {
		if out, err := gitOutput(ctx, abs, gitQuick, nil, "remote", "get-url", "whisk"); err == nil {
			r.WhiskRemote = strings.TrimSpace(string(out))
		} else {
			// No remote named whisk, the usual case before a first deploy: W006 has nothing to
			// compare the binding with, so it stays silent rather than guessing.
			r.WhiskRemote = ""
		}
	}
	return r, nil
}

// listFiles returns every path doctor considers part of the app and which of them git tracks.
func listFiles(ctx context.Context, r Repo) ([]string, map[string]bool, error) {
	tracked := map[string]bool{}
	if r.IsGit && r.GitOK {
		all, err := gitLines(ctx, r.Dir, "ls-files", "-co", "--exclude-standard", "-z")
		if err != nil {
			return nil, nil, err
		}
		t, err := gitLines(ctx, r.Dir, "ls-files", "-z")
		if err != nil {
			return nil, nil, err
		}
		for _, p := range t {
			tracked[p] = true
		}
		return all, tracked, nil
	}
	var ign *gitignore.GitIgnore
	if src, err := os.ReadFile(filepath.Join(r.Dir, ".gitignore")); err == nil {
		ign = gitignore.CompileIgnoreLines(strings.Split(string(src), "\n")...)
	}
	var out []string
	err := filepath.WalkDir(r.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(r.Dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || (ign != nil && ign.MatchesPath(rel+"/")) {
				return filepath.SkipDir
			}
			return nil
		}
		if ign != nil && ign.MatchesPath(rel) {
			return nil
		}
		out = append(out, rel)
		tracked[rel] = true
		return nil
	})
	return out, tracked, err
}

func gitLines(ctx context.Context, dir string, args ...string) ([]string, error) {
	raw, err := gitOutput(ctx, dir, gitQuick, nil, args...)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range bytes.Split(raw, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out, nil
}

// gitQuick bounds a git command that reads the index or the object counts; gitHistory bounds one
// that walks every object in the history, which takes minutes in a large repository.
const (
	gitQuick   = 2 * time.Minute
	gitHistory = 10 * time.Minute
)

// gitOutput runs git in dir and answers all of its standard output, which for a large
// repository is more than run's capped Output keeps. A failure carries git's own message.
func gitOutput(ctx context.Context, dir string, timeout time.Duration, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := run.Command(ctx, timeout, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, errors.New("git " + strings.Join(args, " ") + ": " + strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	return out.Bytes(), nil
}

func isBinary(b []byte) bool {
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	return bytes.IndexByte(head, 0) >= 0
}

// Read returns a file's content or nil.
func (r Repo) Read(p string) []byte {
	for _, f := range r.Files {
		if f.Path == p {
			return f.Content
		}
	}
	return nil
}

// Has reports whether a path exists.
func (r Repo) Has(p string) bool {
	for _, f := range r.Files {
		if f.Path == p {
			return true
		}
	}
	return false
}

// Code returns the app's own source files for a language: not tests, not vendored, with text.
func (r Repo) Code(lang stack.Lang) []File {
	var out []File
	for _, f := range r.Files {
		if f.Content == nil || stack.LangOf(f.Path) != lang || stack.IsTest(f.Path) || stack.IsVendored(f.Path) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// AllCode returns source files of every language doctor reads.
func (r Repo) AllCode() []File {
	var out []File
	for _, l := range []stack.Lang{stack.JS, stack.PY, stack.GO} {
		out = append(out, r.Code(l)...)
	}
	return out
}

// lineAt returns the 1-based line of a byte offset.
func lineAt(content []byte, offset int) int {
	if offset > len(content) {
		offset = len(content)
	}
	return bytes.Count(content[:offset], []byte{'\n'}) + 1
}
