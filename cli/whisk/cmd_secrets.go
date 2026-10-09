package whisk

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// secretScope resolves where a secrets command acts: the org and, for app scope, the app.
// --scope org names the org's own secrets; the default is the bound app when there is one.
func (s *session) secretScope(scope string) (org, app string, err error) {
	org, err = s.org()
	if err != nil {
		return "", "", err
	}
	switch scope {
	case "org":
		return org, "", nil
	case "", "app":
		app = s.appIfBound()
		if app == "" && scope == "app" {
			return "", "", output.New("INVALID_REQUEST", "No app for --scope app.", "Run from a bound directory or pass --app <slug>.", nil)
		}
		return org, app, nil
	}
	return "", "", output.New("INVALID_REQUEST", fmt.Sprintf("--scope %q is not a scope.", scope), "Pass --scope org or --scope app.", nil)
}

func secretsCmd(s *session) *cobra.Command {
	secrets := &cobra.Command{Use: "secrets", Short: "Secret names, versions and reads; never values"}
	var scope string
	secrets.PersistentFlags().StringVar(&scope, "scope", "", "org for org-scoped secrets, app for the bound app (default: app when bound, else org)")

	list := &cobra.Command{
		Use:   "list",
		Short: "List secrets: name, scope, whether set, version, shared with previews, consumers, last read",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			var all []api.Secret
			if app == "" {
				all, err = client.ListOrgSecrets(s.ctx, org)
			} else {
				all, err = client.ListAppSecrets(s.ctx, org, app)
			}
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "secrets": all, "url": secretsURL(s.dashboard(), org, app)}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintln(w, "No secrets declared. Declare one with whisk secrets declare NAME.")
					return
				}
				rows := make([][]string, len(all))
				for i, sec := range all {
					set := "unset"
					if sec.Set {
						set = "set"
					}
					rows[i] = []string{sec.Name, string(sec.Scope), set, fmt.Sprint(sec.Version), previewsLabel(sec.Previews), fmt.Sprint(len(sec.Consumers)), when(sec.LastReadAt)}
				}
				s.printer.Table(w, []string{"NAME", "SCOPE", "VALUE", "VERSION", "PREVIEWS", "CONSUMERS", "LAST READ"}, rows)
				fmt.Fprintf(w, "Values are set at %s\n", secretsURL(s.dashboard(), org, app))
			})
			return nil
		},
	}

	declare := &cobra.Command{
		Use:   "declare NAME",
		Short: "Create the slot for a secret; a human sets its value in the dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			req := api.DeclareSecret{Name: args[0], Scope: "org"}
			if app != "" {
				req.Scope, req.AppID = "app", app
			}
			sec, err := client.DeclareSecret(s.ctx, org, req)
			if err != nil {
				return wrap(err)
			}
			url := secretsURL(s.dashboard(), org, app)
			s.printer.Result(map[string]any{"org": org, "app": app, "secret": sec, "url": url}, func(w io.Writer) {
				state := "has no value yet; set it at " + url
				if sec.Set {
					state = "already has a value"
				}
				fmt.Fprintf(w, "Declared %s (%s scope); it %s\n", sec.Name, sec.Scope, state)
			})
			return nil
		},
	}

	var previews bool
	set := &cobra.Command{
		Use:   "set NAME [NAME...] [--previews | --previews=false]",
		Short: "Where a human sets the values: prints one link for all of them and exits 2",
		Long: `Secret values are only ever typed in the dashboard. The CLI never takes one, from a prompt or
otherwise, so this prints the NEEDS_HUMAN block with one link for every name given. The link opens
the secrets page with its paste form ready, a NAME= line for each, so an owner or admin pastes
all the values at once.

Previews receive a secret only when it is shared with previews. With --previews (or
--previews=false) the block instead asks an owner or admin to turn "Share with previews" on (or
off) for the names on the secrets page; that is a person's decision too, since a preview runs a
branch's code.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("previews") {
				return previewsNeedsHuman(secretsURL(s.dashboard(), org, app), org, app, args, previews)
			}
			url := pasteURL(secretsURL(s.dashboard(), org, app), args)
			verb, them := "needs a value", "the value"
			if len(args) > 1 {
				verb, them = "need values", "the values"
			}
			return &output.Error{
				Code:    "NEEDS_HUMAN",
				Message: fmt.Sprintf("%s %s, and values are set in the dashboard, never through the CLI.", strings.Join(args, ", "), verb),
				Fix:     "Ask an owner or admin to open " + url + " and paste " + them + " there. Do not ask them for a value and do not put one in a file.",
				Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
				Details: map[string]any{"url": url, "name": args[0], "names": args, "org": org, "app": app},
			}
		},
	}

	set.Flags().BoolVar(&previews, "previews", false, "ask a person to share the secrets with previews (--previews=false to stop sharing)")

	link := &cobra.Command{
		Use:   "link NAME",
		Short: "Print the dashboard URL where a human sets the value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			url := secretsURL(s.dashboard(), org, app)
			s.printer.Result(map[string]any{"org": org, "app": app, "name": args[0], "url": url}, func(w io.Writer) {
				fmt.Fprintf(w, "Set %s at %s\n", args[0], url)
			})
			return nil
		},
	}

	versions := &cobra.Command{
		Use:   "versions NAME",
		Short: "List the stored versions of a secret (lengths and authors, never values)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			all, err := client.SecretVersions(s.ctx, org, args[0], app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "name": args[0], "versions": all}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintf(w, "%s has no value yet.\n", args[0])
					return
				}
				rows := make([][]string, len(all))
				for i, v := range all {
					rows[i] = []string{fmt.Sprint(v.Version), fmt.Sprint(v.Length), orDash(v.CreatedBy), at(v.CreatedAt), v.Note}
				}
				s.printer.Table(w, []string{"VERSION", "LENGTH", "BY", "CREATED", "NOTE"}, rows)
			})
			return nil
		},
	}

	var to int
	rollback := &cobra.Command{
		Use:   "rollback NAME --to N",
		Short: "Make an earlier version current; consumers restart",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if to <= 0 {
				return output.New("INVALID_REQUEST", "--to names the version to make current.", "Run whisk secrets versions "+args[0]+" and pass --to <version>.", nil)
			}
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			sec, err := client.RollbackSecret(s.ctx, org, args[0], to, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "secret": sec}, func(w io.Writer) {
				fmt.Fprintf(w, "%s is now version %d; apps that read it restart.\n", sec.Name, sec.Version)
			})
			return nil
		},
	}
	rollback.Flags().IntVar(&to, "to", 0, "version to make current")

	var from string
	share := &cobra.Command{
		Use:   "share NAME --from <app>",
		Short: "Let every app that names NAME use the value another app already has",
		Long: `Moves the value an app already has under NAME to the business, so every app that names NAME
and has no value of its own reads it; apps waiting for it start. Ask the human first whether the
app may use the same value. No value is sent or shown. An app with its own value keeps it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				return output.New("INVALID_REQUEST", "--from names the app that has the value.", "Run whisk secrets list --scope org to see which app has "+args[0]+", then pass --from <app>.", nil)
			}
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			out, err := client.ShareSecret(s.ctx, org, args[0], from)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "from": from, "secret": out.Secret, "apps": out.Apps}, func(w io.Writer) {
				fmt.Fprintf(w, "%s from %s is now shared by every app that names it.\n", out.Secret.Name, from)
				if len(out.Apps) > 0 {
					fmt.Fprintf(w, "Starting with it: %s\n", strings.Join(out.Apps, ", "))
				}
			})
			return nil
		},
	}
	share.Flags().StringVar(&from, "from", "", "the app that already has the value")

	del := &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a secret and every version of it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if err := client.DeleteSecret(s.ctx, org, args[0], app); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "name": args[0], "deleted": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Deleted %s.\n", args[0])
			})
			return nil
		},
	}

	var since string
	reads := &cobra.Command{
		Use:   "reads NAME [--since 7d]",
		Short: "When and by which container the value was delivered",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := parseSince(since, time.Now())
			if err != nil {
				return sinceFlagError(since)
			}
			org, app, err := s.secretScope(scope)
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			all, err := client.SecretReads(s.ctx, org, args[0], app, from)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "name": args[0], "since": from.UTC(), "reads": all}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintf(w, "%s was not read since %s.\n", args[0], at(from))
					return
				}
				rows := make([][]string, len(all))
				for i, r := range all {
					rows[i] = []string{at(r.ReadAt), fmt.Sprint(r.Version), string(r.ReadByKind), r.ReadByID}
				}
				s.printer.Table(w, []string{"READ AT", "VERSION", "BY", "ID"}, rows)
			})
			return nil
		},
	}
	reads.Flags().StringVar(&since, "since", "30d", "how far back: a duration (6h, 7d) or an RFC 3339 time")

	secrets.AddCommand(list, declare, set, link, versions, rollback, share, del, reads)
	return secrets
}

// previewsLabel is the PREVIEWS column: whether preview environments receive the secret.
func previewsLabel(shared bool) string {
	if shared {
		return "shared"
	}
	return "no"
}

// previewsNeedsHuman is the NEEDS_HUMAN block for sharing secrets with previews, or stopping:
// only an owner or admin, signed in, changes it, on the secrets page at url.
func previewsNeedsHuman(url, org, app string, names []string, on bool) error {
	them, verb, turn := strings.Join(names, ", "), "is", "on"
	if len(names) > 1 {
		verb = "are"
	}
	if !on {
		turn = "off"
	}
	message := fmt.Sprintf("Whether %s %s shared with previews is decided in the dashboard by an owner or admin, never through the CLI.", them, verb)
	return &output.Error{
		Code:    "NEEDS_HUMAN",
		Message: message,
		Fix:     fmt.Sprintf("Ask an owner or admin to open %s and turn Share with previews %s for %s. Previews receive a secret only when it is shared with previews; production is not affected.", url, turn, them),
		Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
		Details: map[string]any{"url": url, "name": names[0], "names": names, "org": org, "app": app, "previews": on},
	}
}
