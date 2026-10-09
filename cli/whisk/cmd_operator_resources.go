package whisk

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/apitypes"
)

// operatorResourcesCmd lists what a business owns outside the platform's database, and confirms
// a leftover a lookup found so it is released (CLI.md §5.14, CONTROL-PLANE.md §6.32).
func operatorResourcesCmd(s *session) *cobra.Command {
	root := &cobra.Command{
		Use:   "resources <business>",
		Short: "Everything a business owns outside Whisk's database, and whether it is released",
		Long: `Lists every repository, database, bucket, sending domain, billing record and other outside
thing the business owns or owned, by its id at its provider, with its state and, for one being
released, its last error. A row in state adopted is a leftover a lookup found: nothing deletes it
until you confirm it with whisk operator resources confirm.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			got, err := client.OrgResources(s.ctx, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"resources": got}, func(w io.Writer) {
				if len(got.Resources) == 0 {
					fmt.Fprintf(w, "%s has nothing recorded.\n", got.Org)
					return
				}
				rows := make([][]string, len(got.Resources))
				for i, r := range got.Resources {
					rows[i] = []string{r.ID, r.Kind, r.Provider, r.ExternalID, string(r.State), ownedNote(r)}
				}
				s.printer.Table(w, []string{"ID", "KIND", "PROVIDER", "EXTERNAL ID", "STATE", "NOTE"}, rows)
				fmt.Fprintln(w, ownedSummary(got))
			})
			return nil
		},
	}
	confirm := &cobra.Command{
		Use:   "confirm <business> <id>",
		Short: "Confirm a leftover a lookup found is the business's, and release it now",
		Long: `Confirms one adopted row, which a lookup found rather than the platform recording it when it was
made, and releases it at once by its recorded id. Check the provider first: the row names the
account and the id. The confirmation is in the business's audit log under your name.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			got, err := client.ConfirmResource(s.ctx, args[0], args[1])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"confirmed": got}, func(w io.Writer) {
				if got.Released {
					fmt.Fprintf(w, "Released %s %s.\n", got.Resource.Kind, got.Resource.ExternalID)
					return
				}
				fmt.Fprintf(w, "Confirmed %s %s; it is not released yet: %s. The hourly sweep tries again.\n", got.Resource.Kind, got.Resource.ExternalID, orNothing(got.Error))
			})
			return nil
		},
	}
	root.AddCommand(confirm)
	return root
}

// ownedNote is what a row needs said beside its state.
func ownedNote(r api.Owned) string {
	switch {
	case r.LastError != "":
		return r.LastError
	case r.State == apitypes.ResourceAdopted && r.ConfirmedAt == nil:
		return "found by a lookup; waits for whisk operator resources confirm"
	case r.State == apitypes.ResourceRetained:
		return r.Retained
	case r.ReleaseAt != nil && r.State != apitypes.ResourceReleased:
		return "released from " + r.ReleaseAt.UTC().Format("2006-01-02 15:04")
	}
	return ""
}

// ownedSummary is the line under the table: done, or how much is still open.
func ownedSummary(o api.OrgOwned) string {
	if o.Done {
		return "Nothing is waiting to be released."
	}
	if len(o.Stuck) > 0 {
		return fmt.Sprintf("Not released yet; %d stuck. Each stuck row's note says why.", len(o.Stuck))
	}
	return "Not released yet."
}

func orNothing(s string) string {
	if s == "" {
		return "no error recorded"
	}
	return s
}
