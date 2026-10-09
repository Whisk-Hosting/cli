package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// findCustomer resolves an email to the customer record, case-insensitively.
func findCustomer(list []api.Customer, email string) (api.Customer, bool) {
	for _, c := range list {
		if strings.EqualFold(c.Email, strings.TrimSpace(email)) {
			return c, true
		}
	}
	return api.Customer{}, false
}

// customersCmd is the people of an app with customer_identity (CLI.md §5.11).
func customersCmd(s *session) *cobra.Command {
	customers := &cobra.Command{Use: "customers", Short: "Customers of an app that declares customer_identity"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's customers and whether strangers may register",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			pool, err := client.ListCustomers(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "customers": pool.Items, "signup": pool.Signup}, func(w io.Writer) {
				if len(pool.Items) == 0 {
					fmt.Fprintln(w, "No customers yet. Invite one with whisk customers invite <email>.")
				} else {
					rows := make([][]string, len(pool.Items))
					for i, c := range pool.Items {
						rows[i] = []string{c.Email, orDash(c.Name), string(c.Status), at(c.LastSeenAt)}
					}
					s.printer.Table(w, []string{"EMAIL", "NAME", "STATUS", "LAST SEEN"}, rows)
				}
				if pool.Signup {
					fmt.Fprintln(w, "Strangers may register themselves; switch that off in the dashboard.")
				} else {
					fmt.Fprintln(w, "Invitation only; the dashboard switch opens self-registration.")
				}
			})
			return nil
		},
	}

	var name string
	invite := &cobra.Command{
		Use:   "invite <email> [--name n]",
		Short: "Invite a customer; prints the invite_url to send them",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			c, err := client.InviteCustomer(s.ctx, org, app, strings.TrimSpace(args[0]), name)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "customer": c, "invite_url": c.InviteURL}, func(w io.Writer) {
				fmt.Fprintf(w, "Invited %s (%s).\n", c.Email, c.Status)
				if c.InviteURL != "" {
					fmt.Fprintf(w, "Send them this link: %s\n", c.InviteURL)
				}
			})
			return nil
		},
	}
	invite.Flags().StringVar(&name, "name", "", "the person's name")

	status := func(use, short, status, verb string) *cobra.Command {
		return &cobra.Command{
			Use:   use + " <email>",
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				org, app, client, c, err := s.customerByEmail(args[0])
				if err != nil {
					return err
				}
				c, err = client.SetCustomerStatus(s.ctx, org, app, c.ID, status)
				if err != nil {
					return wrap(err)
				}
				s.printer.Result(map[string]any{"org": org, "app": app, "customer": c}, func(w io.Writer) {
					fmt.Fprintf(w, "%s %s (%s). It takes effect on their next request.\n", verb, c.Email, c.Status)
				})
				return nil
			},
		}
	}

	remove := &cobra.Command{
		Use:   "remove <email>",
		Short: "Remove a customer and their access to the app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, client, c, err := s.customerByEmail(args[0])
			if err != nil {
				return err
			}
			if err := client.RemoveCustomer(s.ctx, org, app, c.ID); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "email": c.Email, "removed": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Removed %s from %s.\n", c.Email, app)
			})
			return nil
		},
	}

	customers.AddCommand(list, invite,
		status("block", "Block a customer; they are signed out on their next request", "blocked", "Blocked"),
		status("unblock", "Let a blocked customer in again", "active", "Unblocked"),
		remove)
	return customers
}

// customerByEmail resolves the target, the client and the customer an email names.
func (s *session) customerByEmail(email string) (org, app string, client *api.Client, c api.Customer, err error) {
	org, app, err = s.target()
	if err != nil {
		return
	}
	client, _, err = s.client()
	if err != nil {
		return
	}
	pool, err := client.ListCustomers(s.ctx, org, app)
	if err != nil {
		err = wrap(err)
		return
	}
	c, ok := findCustomer(pool.Items, email)
	if !ok {
		err = output.New("NOT_FOUND", fmt.Sprintf("%s is not a customer of %s.", email, app), "whisk customers list shows who is.", map[string]any{"email": email, "app": app})
	}
	return
}
