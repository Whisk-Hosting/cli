package whisk

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// flagsCmd is the app's own feature flags (CLI.md §5.15, CONTRACT.md §3.3): the flags its
// whisk.yaml in force declares and who each is on for, changed with no deploy.
func flagsCmd(s *session) *cobra.Command {
	list := func(cmd *cobra.Command, args []string) error {
		client, org, app, err := s.appClient()
		if err != nil {
			return err
		}
		out, err := client.AppFlags(s.ctx, org, app)
		if err != nil {
			return wrap(err)
		}
		s.printer.Result(map[string]any{"org": org, "app": app, "flags": out.Items, "included": out.Included}, func(w io.Writer) {
			printFlags(s, w, out)
		})
		return nil
	}
	flags := &cobra.Command{
		Use:   "flags",
		Short: "The app's feature flags and who each is on for",
		Long: `Lists the flags the app's whisk.yaml in force declares and who each is on for. Declare a flag
under flags in whisk.yaml and read X-Whisk-Flags in the app; who it is on for changes here, on the
app's Flags page, or through the API, with no deploy. Requests carry a change within 30 seconds.
Flags are part of the Starter plan and up.`,
		Args: cobra.NoArgs,
		RunE: list,
	}
	flags.AddCommand(&cobra.Command{Use: "list", Short: "List the app's flags and who each is on for", Args: cobra.NoArgs, RunE: list})

	var people, groups []string
	var team, previews bool
	var percent int
	set := &cobra.Command{
		Use:   "set <name>",
		Short: "Choose who a flag is on for; options left out keep what was set",
		Long: `Sets who the flag is on for. Each option replaces what was set for it and leaves the rest:

  whisk flags set new-checkout --team                 everyone on the business's team
  whisk flags set new-checkout --people a@example.com,b@example.com
  whisk flags set new-checkout --groups beta          everyone in the business's group beta
  whisk flags set new-checkout --percent 10           10% of everyone else signed in
  whisk flags set new-checkout --previews             on in every preview

--people and --groups replace the list; pass an empty value (--people "") to clear it. A person
keeps their side as the percentage grows. whisk flags on turns a flag on for everyone and whisk
flags off turns it off for everyone.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			changed := cmd.Flags().Changed
			if !slices.ContainsFunc([]string{"people", "groups", "team", "percent", "previews"}, changed) {
				return output.New("INVALID_REQUEST", "Say who the flag is on for.", "Pass --team, --people, --groups, --percent or --previews; whisk flags on "+args[0]+" turns it on for everyone.", nil)
			}
			return s.setFlag(args[0], func(in *apitypes.AppFlagRequest) {
				if changed("people") {
					in.People = splitFlagList(people)
				}
				if changed("groups") {
					in.Groups = splitFlagList(groups)
				}
				if changed("team") {
					in.Team = team
				}
				if changed("percent") {
					in.Percent = percent
				}
				if changed("previews") {
					in.Previews = previews
				}
			})
		},
	}
	set.Flags().StringSliceVar(&people, "people", nil, "emails, comma separated")
	set.Flags().StringSliceVar(&groups, "groups", nil, "the business's group names, comma separated")
	set.Flags().BoolVar(&team, "team", false, "everyone on the business's team (--team=false to stop)")
	set.Flags().IntVar(&percent, "percent", 0, "a share of everyone else signed in, 0 to 100")
	set.Flags().BoolVar(&previews, "previews", false, "on in every preview (--previews=false to stop)")

	on := &cobra.Command{
		Use:   "on <name>",
		Short: "Turn a flag on for everyone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.setFlag(args[0], func(in *apitypes.AppFlagRequest) { in.Percent = 100 })
		},
	}
	off := &cobra.Command{
		Use:   "off <name>",
		Short: "Turn a flag off for everyone, previews included",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.setFlag(args[0], func(in *apitypes.AppFlagRequest) {
				*in = apitypes.AppFlagRequest{People: []string{}, Groups: []string{}}
			})
		},
	}
	flags.AddCommand(set, on, off)
	return flags
}

// setFlag reads who the flag is on for now, applies change and stores the result.
func (s *session) setFlag(name string, change func(*apitypes.AppFlagRequest)) error {
	client, org, app, err := s.appClient()
	if err != nil {
		return err
	}
	all, err := client.AppFlags(s.ctx, org, app)
	if err != nil {
		return wrap(err)
	}
	i := slices.IndexFunc(all.Items, func(f apitypes.AppFlag) bool { return f.Name == name })
	if i < 0 {
		return output.New("NOT_FOUND", fmt.Sprintf("%s/%s's whisk.yaml in force declares no flag %s.", org, app, name),
			"Add it under flags in whisk.yaml and deploy; whisk flags lists the declared flags.", map[string]any{"flag": name})
	}
	in := requestOfFlag(all.Items[i])
	change(&in)
	f, err := client.SetAppFlag(s.ctx, org, app, name, in)
	if err != nil {
		return wrap(err)
	}
	s.printer.Result(map[string]any{"org": org, "app": app, "flag": f}, func(w io.Writer) {
		fmt.Fprintf(w, "%s is on for %s. Requests carry it within 30 seconds.\n", f.Name, flagReach(f))
	})
	return nil
}

// requestOfFlag is a flag as it is set, as the body that sets it again. Pure.
func requestOfFlag(f apitypes.AppFlag) apitypes.AppFlagRequest {
	groups := make([]string, 0, len(f.Groups))
	for _, g := range f.Groups {
		groups = append(groups, g.Name)
	}
	return apitypes.AppFlagRequest{People: append([]string{}, f.People...), Groups: groups, Team: f.Team, Percent: f.Percent, Previews: f.Previews}
}

// splitFlagList is a list option's entries, trimmed, without empty ones. Pure.
func splitFlagList(in []string) []string {
	out := []string{}
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// flagReach is who a flag is on for, in a few words. Pure.
func flagReach(f apitypes.AppFlag) string {
	if f.Percent >= 100 {
		return "everyone"
	}
	var parts []string
	if f.Team {
		parts = append(parts, "the team")
	}
	if n := len(f.People); n > 0 {
		parts = append(parts, countOf(n, "person", "people"))
	}
	for _, g := range f.Groups {
		parts = append(parts, "group "+g.Name)
	}
	if f.Percent > 0 {
		parts = append(parts, strconv.Itoa(f.Percent)+"% of everyone else")
	}
	if f.Previews {
		parts = append(parts, "previews")
	}
	if len(parts) == 0 {
		return "nobody"
	}
	return strings.Join(parts, ", ")
}

func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func printFlags(s *session, w io.Writer, out apitypes.AppFlagList) {
	if len(out.Items) == 0 {
		fmt.Fprintln(w, "No flags. Declare one under flags in whisk.yaml and deploy.")
		return
	}
	rows := make([][]string, 0, len(out.Items))
	for _, f := range out.Items {
		removeBy := f.RemoveBy
		if f.Overdue {
			removeBy += " (passed)"
		}
		rows = append(rows, []string{f.Name, string(f.Kind), flagReach(f), removeBy})
	}
	s.printer.Table(w, []string{"NAME", "KIND", "ON FOR", "REMOVE BY"}, rows)
	if !out.Included {
		fmt.Fprintln(w, "The business's plan does not include flags, so every flag is off. Move to Starter or above with whisk billing.")
	}
}
