package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// secretAppsCmd is `whisk secrets apps NAME`: the apps a business secret is granted to
// (CONTROL-PLANE.md §6.7 Grants). Only an owner or admin, signed in to the dashboard, adds or
// removes one, so the CLI reads the list and says where.
func secretAppsCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "apps NAME",
		Short: "The apps a business secret reaches; an owner or admin adds or removes them in the dashboard",
		Long: `A secret the business shares reaches only the apps on its list. A production deploy of an app
that names it and is not on the list waits for an owner or admin to approve it; a preview starts
without it. This lists the apps and prints where an owner or admin changes the list.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			all, err := client.ListOrgSecrets(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			apps, err := client.ListApps(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			url := keyURL(orgPage(s.dashboard(), org, "/keys"), args[0])
			sec, ok := businessSecret(all, args[0])
			if !ok {
				return output.New("NOT_FOUND", fmt.Sprintf("The business shares no secret named %s.", args[0]),
					"Only a business secret has a list of apps; an app's own secret reaches only that app. Run whisk secrets list --scope org to see the business's.", map[string]any{"name": args[0]})
			}
			slugs := appSlugs(sec.Apps, apps)
			s.printer.Result(map[string]any{"org": org, "name": sec.Name, "apps": slugs, "url": url}, func(w io.Writer) {
				fmt.Fprintln(w, secretAppsSentence(sec.Name, slugs))
				fmt.Fprintf(w, "An owner or admin adds or removes apps at %s\n", url)
			})
			return nil
		},
	}
}

// businessSecret is the business's own secret of the name among all.
func businessSecret(all []api.Secret, name string) (api.Secret, bool) {
	for _, sec := range all {
		if sec.Name == name && sec.Scope == apitypes.ScopeOrg {
			return sec, true
		}
	}
	return api.Secret{}, false
}

// appSlugs names the apps by slug, in the order given; an app the list cannot name keeps its id.
func appSlugs(ids []string, apps []api.App) []string {
	bySlug := map[string]string{}
	for _, a := range apps {
		bySlug[a.ID] = a.Slug
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if slug, ok := bySlug[id]; ok {
			out = append(out, slug)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// secretAppsSentence says which apps a business secret reaches.
func secretAppsSentence(name string, slugs []string) string {
	if len(slugs) == 0 {
		return name + " reaches no app yet."
	}
	return name + " reaches " + strings.Join(slugs, ", ") + "."
}
