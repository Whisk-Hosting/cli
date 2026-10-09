package scaffold

import (
	"regexp"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/naughty"
)

// An app slug is lowercase letters, digits and single hyphens, starting and ending with a
// letter or digit (DESIGN.md §5): the hostname <app>--<org> depends on it.
var reNaughtySlug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Slug and CheckSlug answer only slugs of that shape, Slug's answers pass CheckSlug, and the
// manifest written for one parses with that name.
func TestNaughtySlug(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, in := range []string{s, "my " + s + " app", strings.ToUpper(s)} {
			if slug, err := Slug(in); err == nil {
				if !reNaughtySlug.MatchString(slug) || len(slug) < 3 || len(slug) > MaxSlug {
					t.Errorf("Slug(%q) = %q", in, slug)
				}
				if err := CheckSlug(slug); err != nil {
					t.Errorf("Slug(%q) = %q, which CheckSlug refuses: %v", in, slug, err)
				}
			}
			if CheckSlug(in) != nil {
				continue
			}
			if !reNaughtySlug.MatchString(in) || len(in) < 3 || len(in) > MaxSlug {
				t.Errorf("CheckSlug accepted %q", in)
			}
			m, err := manifest.Parse([]byte(Manifest(in, stack.Stack{Migrate: "npm run migrate"})))
			if err != nil || m.Name != in {
				t.Errorf("the manifest for %q does not parse as that app: %v", in, err)
			}
		}
	}
}

// Merging the .gitignore entries keeps what was there, adds each missing entry once, and a
// second merge adds nothing.
func TestNaughtyGitignore(t *testing.T) {
	for _, s := range naughty.Strings() {
		out, _ := MergeGitignore(s)
		if !strings.HasPrefix(out, s) {
			t.Errorf("MergeGitignore(%q) lost the existing text", s)
		}
		again, added := MergeGitignore(out)
		if again != out || len(added) != 0 {
			t.Errorf("MergeGitignore(%q) is not idempotent: added %q", s, added)
		}
	}
}
