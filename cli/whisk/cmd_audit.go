package whisk

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/endpoints"
)

// auditCmd reads the org's audit log, newest first (CLI.md §5): who did what to which thing,
// and when. Owners and admins; the platform refuses anyone else.
func auditCmd(s *session) *cobra.Command {
	var since, actor, action, cursor string
	var limit int
	c := &cobra.Command{
		Use:   "audit [--since 7d] [--actor <id>] [--action deploy.created] [--limit 50] [--cursor <next>]",
		Short: "The org's audit log, newest first: who did what, to what, and when",
		Long: `Lists the org's audit log, newest first: when, who acted (and for whom), the action and what
it acted on. --since takes a time (RFC 3339) or an age such as 24h or 7d; --actor and --action
narrow it. One page of --limit entries prints at a time; pass the cursor it ends with to read
on. --json prints {org, events, next_cursor}. Owners and admins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 500 {
				return output.New("INVALID_REQUEST", "--limit must be 1 to 500.", "Pass --limit 50, or fewer.", map[string]any{"limit": limit})
			}
			q := endpoints.ListAuditQuery{Actor: actor, Action: action, Cursor: cursor, Limit: limit}
			if since != "" {
				t, err := parseSince(since, time.Now())
				if err != nil {
					return output.New("INVALID_REQUEST", "--since must be a time or an age: "+since+".", "Pass an RFC 3339 time, or an age such as 24h or 7d.", map[string]any{"since": since})
				}
				q.Since = t.UTC().Format(time.RFC3339)
			}
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			page, err := api.Send(s.ctx, client, endpoints.ListAudit, q, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "events": page.Items, "next_cursor": page.NextCursor}, func(w io.Writer) {
				if len(page.Items) == 0 {
					fmt.Fprintln(w, "Nothing in the audit log matches.")
					return
				}
				rows := make([][]string, len(page.Items))
				for i, e := range page.Items {
					who := orDash(e.ActorLabel)
					if e.OnBehalfOf != "" {
						who += " for " + e.OnBehalfOf
					}
					rows[i] = []string{at(e.At), who, e.Action, orDash(e.TargetKind + " " + e.TargetID)}
				}
				s.printer.Table(w, []string{"AT", "WHO", "ACTION", "TARGET"}, rows)
				if page.NextCursor != "" {
					fmt.Fprintf(w, "More: whisk audit --cursor %s\n", page.NextCursor)
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&since, "since", "", "a time (RFC 3339) or an age such as 24h or 7d")
	c.Flags().StringVar(&actor, "actor", "", "only what this actor did")
	c.Flags().StringVar(&action, "action", "", "only this action, such as deploy.created")
	c.Flags().StringVar(&cursor, "cursor", "", "the next page, from the line a page ends with")
	c.Flags().IntVar(&limit, "limit", 50, "entries in one page, 1 to 500")
	return c
}
