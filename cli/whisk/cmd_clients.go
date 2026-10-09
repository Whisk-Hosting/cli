package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// clientsCmd is an agency's client businesses (CLI.md §5.11): all of them at once, adding one,
// and handing one over to its contact.
func clientsCmd(s *session) *cobra.Command {
	clients := &cobra.Command{
		Use:   "clients",
		Short: "An agency's client businesses: standing, handed over, apps, problems and usage",
		Long: `Lists the client businesses of the org in context (the agency). Build in one with the
agency's own login: whisk init --org <client> in a new folder, or whisk use <client>/<app>, then
whisk deploy as usual.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			items, err := client.ClientsOverview(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "clients": items}, func(w io.Writer) {
				if len(items) == 0 {
					fmt.Fprintln(w, "No client businesses yet. Add one with whisk clients add <name> --contact <email>.")
					return
				}
				s.printer.Table(w, []string{"CLIENT", "STANDING", "APPS", "PROBLEMS", "STORAGE", "RUNS"}, clientRows(items))
				fmt.Fprintln(w, clientsSummary(items))
			})
			return nil
		},
	}

	var contact, slug string
	var yearly, noFreeMonth bool
	add := &cobra.Command{
		Use:   "add <name> [--contact email] [--slug s] [--yearly] [--no-free-month]",
		Short: "Add a client business, billed to the agency; prints the payment page when a card is needed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			in := api.ClientInput{Name: strings.TrimSpace(args[0]), Slug: slug, Contact: strings.TrimSpace(contact), Interval: "month"}
			if yearly {
				in.Interval = "year"
			}
			if noFreeMonth {
				no := false
				in.Trial = &no
			}
			res, err := client.AddClient(s.ctx, org, in)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "client": res.Client, "checkout_url": res.CheckoutURL, "trial": res.Trial}, func(w io.Writer) {
				fmt.Fprintf(w, "Added %s (%s).\n", res.Client.Name, res.Client.Slug)
				if res.CheckoutURL != "" {
					fmt.Fprintf(w, "Open this to pay for it: %s\n", res.CheckoutURL)
				}
				fmt.Fprintf(w, "Build in it with whisk init --org %s.\n", res.Client.Slug)
			})
			return nil
		},
	}
	add.Flags().StringVar(&contact, "contact", "", "the email of the person it will be handed to")
	add.Flags().StringVar(&slug, "slug", "", "its address name; made from the name when left out")
	add.Flags().BoolVar(&yearly, "yearly", false, "bill it yearly")
	add.Flags().BoolVar(&noFreeMonth, "no-free-month", false, "pay from today instead of using the agency's free first month")

	var to string
	handover := &cobra.Command{
		Use:   "handover <client> [--contact email]",
		Short: "Make the link the client's contact accepts the business with",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			h, err := client.HandoverClient(s.ctx, org, strings.TrimSpace(args[0]), strings.TrimSpace(to))
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "client": args[0], "claim_url": h.ClaimURL, "contact": h.Contact}, func(w io.Writer) {
				fmt.Fprintf(w, "Send %s this link: %s\n", h.Contact, h.ClaimURL)
				fmt.Fprintln(w, "Only they can accept it. Your people stay on as developers.")
			})
			return nil
		},
	}
	handover.Flags().StringVar(&to, "contact", "", "who accepts it; the contact set when it was added if left out")

	clients.AddCommand(add, handover)
	return clients
}

var clientStanding = map[string]string{
	"unbilled":  "not paid for yet",
	"trialing":  "free month",
	"active":    "paid",
	"past_due":  "a payment failed",
	"frozen":    "paused",
	"shredding": "being deleted",
}

// clientRows is each client business as a table row.
func clientRows(items []api.ClientOverview) [][]string {
	rows := make([][]string, len(items))
	for i, c := range items {
		standing := clientStanding[string(c.Billing)]
		if standing == "" {
			standing = string(c.Billing)
		}
		if c.HandedOver {
			standing += ", handed over"
		}
		rows[i] = []string{c.Slug, standing, fmt.Sprintf("%d", c.Apps), clientProblems(c),
			billingSize(c.Usage.StorageBytes) + " of " + billingSize(c.Limits.StorageBytes),
			fmt.Sprintf("%d of %d", c.Usage.Runs, c.Limits.Runs)}
	}
	return rows
}

func clientProblems(c api.ClientOverview) string {
	var parts []string
	if c.Problems > 0 {
		parts = append(parts, fmt.Sprintf("%d app%s broken", c.Problems, plural(c.Problems)))
	}
	if c.FailedDeploys > 0 {
		parts = append(parts, fmt.Sprintf("%d failed deploy%s", c.FailedDeploys, plural(c.FailedDeploys)))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

// clientsSummary is the one line under the table: how many clients have problems.
func clientsSummary(items []api.ClientOverview) string {
	n := 0
	for _, c := range items {
		if c.Problems > 0 || c.FailedDeploys > 0 {
			n++
		}
	}
	if n == 0 {
		return "No problems."
	}
	return fmt.Sprintf("%d client%s with problems. whisk status --org <client> --app <app> shows what is wrong.", n, plural(n))
}
