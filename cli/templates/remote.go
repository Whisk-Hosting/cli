package templates

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// Remote names the templates that are not carried in the binary: each is its own public
// repository under whisk-hosting, published from the platform's repository at every release
// (the shop and the website, which are large and change with the platform). whisk init
// downloads one when it is asked for.
var Remote = map[string]string{"shop": "shop", "site": "site"}

// RemoteBase is where the template repositories are. WHISK_TEMPLATES_URL overrides it.
const RemoteBase = "https://github.com/whisk-hosting"

// limits on a downloaded template, far above any real one, so a wrong answer cannot fill a disk.
const (
	maxArchive = 64 << 20
	maxFile    = 16 << 20
	maxFiles   = 5000
)

var released = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}-[0-9a-f]{7,40}$`)

// ArchiveURL is the address of a remote template's files for this CLI: the tag of its own
// release, so a CLI always gets the template that was released with it, or the main branch for
// a build that is not a release. Pure.
func ArchiveURL(base, repo, version string) string {
	ref := "refs/heads/main"
	if released.MatchString(version) {
		ref = "refs/tags/" + version
	}
	return strings.TrimRight(base, "/") + "/" + repo + "/archive/" + ref + ".tar.gz"
}

// FromArchive reads a template from a repository's tar.gz archive, whose files all sit in one
// top folder (repo-ref/). Only plain files are taken: a link, a device or a path that would leave
// the folder is refused, as is anything over the limits above. Pure but for reading r.
func FromArchive(r io.Reader) (map[string]File, error) {
	zr, err := gzip.NewReader(io.LimitReader(r, maxArchive+1))
	if err != nil {
		return nil, fmt.Errorf("the template is not a gzip archive: %w", err)
	}
	tr := tar.NewReader(zr)
	out := map[string]File{}
	var top string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the template archive is damaged: %w", err)
		}
		name := strings.TrimPrefix(h.Name, "./")
		first, rest, _ := strings.Cut(name, "/")
		if top == "" {
			top = first
		}
		if first != top {
			return nil, fmt.Errorf("the template archive holds %s outside its folder", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir, tar.TypeXGlobalHeader:
			continue
		case tar.TypeReg:
		default:
			return nil, fmt.Errorf("the template archive holds %s, which is not a plain file", h.Name)
		}
		clean := path.Clean(rest)
		if rest == "" || clean != rest || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("the template archive holds a path that will not do: %s", h.Name)
		}
		if h.Size > maxFile {
			return nil, fmt.Errorf("the template's %s is larger than %d MB", clean, maxFile>>20)
		}
		if len(out) >= maxFiles {
			return nil, fmt.Errorf("the template holds more than %d files", maxFiles)
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxFile))
		if err != nil {
			return nil, fmt.Errorf("the template archive is damaged: %w", err)
		}
		mode := fs.FileMode(0o644)
		if h.Mode&0o111 != 0 {
			mode = 0o755
		}
		out[clean] = File{Mode: mode, Content: string(body)}
	}
	if len(out) == 0 {
		return nil, errors.New("the template archive is empty")
	}
	if _, ok := out["whisk.yaml"]; !ok {
		return nil, errors.New("the template has no whisk.yaml")
	}
	return out, nil
}
