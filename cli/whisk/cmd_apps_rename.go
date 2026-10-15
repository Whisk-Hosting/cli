package whisk

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/cli/scaffold"
)

// renameCmd changes an app's address in place by giving it a new slug (CLI.md §5.2,
// CONTROL-PLANE.md §6.11): the platform keeps the old address sending visitors on and the old
// slug finding the app, and this directory, when it is the app's, is rebound to the new slug.
func renameCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <new-slug>",
		Short: "Change the app's address by giving it a new slug; the old address sends visitors on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			next := strings.ToLower(strings.TrimSpace(args[0]))
			if err := scaffold.CheckSlug(next); err != nil {
				return output.New("INVALID_REQUEST", err.Error(), "Choose 3 to 40 characters of a-z, 0-9 and single hyphens.", nil)
			}
			org, ref, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			before, err := client.GetApp(s.ctx, org, ref)
			if err != nil {
				return wrap(err)
			}
			after, err := client.RenameApp(s.ctx, org, before.ID, next)
			if err != nil {
				return wrap(err)
			}
			rebound, named := false, false
			if after.Slug != before.Slug {
				rebound, named, err = s.rebind(org, before.Slug, before.ID, after.Slug)
				if err != nil {
					return err
				}
			}
			s.printer.Result(map[string]any{"org": org, "app": after, "from": before.Slug, "old_url": "https://" + before.Hostname,
				"renamed": after.Slug != before.Slug, "binding_updated": rebound, "manifest_updated": named}, func(w io.Writer) {
				if after.Slug == before.Slug {
					fmt.Fprintf(w, "%s/%s already has that name: https://%s\n", org, after.Slug, after.Hostname)
					return
				}
				fmt.Fprintf(w, "Renamed %s/%s to %s/%s: https://%s\n", org, before.Slug, org, after.Slug, after.Hostname)
				fmt.Fprintf(w, "https://%s now sends visitors there, and %s still finds the app until another app takes the name.\n", before.Hostname, before.Slug)
				var files []string
				if rebound {
					files = append(files, config.BindingPath)
				}
				if named {
					files = append(files, manifestPath)
				}
				if len(files) > 0 {
					fmt.Fprintf(w, "Updated %s here; commit them.\n", strings.Join(files, " and "))
				}
			})
			return nil
		},
	}
}

// manifestPath is the app's whisk.yaml, relative to the bound directory.
const manifestPath = "whisk.yaml"

// rebind points this directory at the app's new slug when it is bound to the app (by its old
// slug or its ID), and sets whisk.yaml's name to it when it named the old one, so doctor's W003
// and the next push agree. It reports which of the two it changed.
func (s *session) rebind(org, oldSlug, id, slug string) (binding, manifest bool, err error) {
	b, ok, err := config.LoadBinding(s.env.Dir)
	if err != nil || !ok || b.Org != org || (b.App != oldSlug && b.App != id) {
		return false, false, err
	}
	b.App = slug
	if err := config.SaveBinding(s.env.Dir, b); err != nil {
		return false, false, err
	}
	path, err := safefile.Inside(s.env.Dir, manifestPath)
	if err != nil {
		return true, false, err
	}
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}
	out, changed := renameManifest(src, oldSlug, slug)
	if !changed {
		return true, false, nil
	}
	return true, true, safefile.Write(s.env.Dir, manifestPath, out, 0o644)
}

// manifestName is whisk.yaml's top-level name line: the key, the value, and any comment after.
var manifestName = regexp.MustCompile(`(?m)^name:([ \t]*)("[^"\n]*"|'[^'\n]*'|[^#\n]*?)([ \t]*(?:#[^\n]*)?)$`)

// renameManifest sets whisk.yaml's top-level name from slug to next, keeping the rest of the
// file as it is, and reports whether it changed anything: a name that is not slug is left alone.
// Pure.
func renameManifest(src []byte, slug, next string) ([]byte, bool) {
	loc := manifestName.FindSubmatchIndex(src)
	if loc == nil {
		return src, false
	}
	value := strings.Trim(string(src[loc[4]:loc[5]]), `"'`)
	if value != slug {
		return src, false
	}
	out := append([]byte{}, src[:loc[4]]...)
	out = append(out, scaffold.ManifestName(next)...)
	return append(out, src[loc[5]:]...), true
}
