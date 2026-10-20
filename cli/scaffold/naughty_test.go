package scaffold

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/hostfiles"
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
			m, err := manifest.Parse([]byte(Manifest(in, stack.Stack{Migrate: "npm run migrate"}, hostfiles.Found{})))
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

// Whatever another host's files hold, the manifest written from them parses and carries each
// value as text: a command, a setting or a note never adds a line or a key of its own.
func TestNaughtyHostFiles(t *testing.T) {
	for _, s := range naughty.Strings() {
		env, _ := json.Marshal(map[string]any{"env": map[string]any{"PLAIN": s, "SECRET_" + "KEY": map[string]any{"required": true}}, "addons": []string{s}})
		files := map[string]string{"Procfile": "web: " + s + "\nrelease: " + s + "\n", "app.json": string(env)}
		h := hostfiles.Read([]string{"Procfile", "app.json"}, func(p string) []byte { return []byte(files[p]) })
		src := Manifest("my-app", stack.Stack{}, h)
		m, err := manifest.Parse([]byte(src))
		if err != nil {
			t.Errorf("the manifest for %q does not parse: %v\n%s", s, err, src)
			continue
		}
		if m.Migrate != strings.TrimSpace(strings.SplitN(s, "\n", 2)[0]) && m.Migrate != "" {
			t.Errorf("migrate %q from %q", m.Migrate, s)
		}
		// The value as JSON carries it, invalid UTF-8 made U+FFFD.
		var want string
		_ = json.Unmarshal(must(json.Marshal(s)), &want)
		got, ok := m.Env["PLAIN"]
		switch {
		case ok && got != want:
			t.Errorf("env PLAIN = %q, want %q", got, want)
		case !ok && !strings.Contains(strings.Join(h.Notes, " "), "PLAIN"):
			t.Errorf("env PLAIN from %q was dropped without a note", s)
		}
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
