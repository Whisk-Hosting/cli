package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// printOrgDomain shows the records to create, or the apps' addresses once verified.
func printOrgDomain(p output.Printer, w io.Writer, d api.OrgDomain) {
	if d.Verified {
		fmt.Fprintf(w, "%s is verified. Every app answers at <app>.%s:\n", d.Domain, d.Domain)
		for _, a := range d.Apps {
			fmt.Fprintf(w, "  %s\n", a.URL)
		}
		return
	}
	fmt.Fprintf(w, "Create these DNS records, then run whisk domains business verify:\n")
	p.Table(w, []string{"TYPE", "NAME", "VALUE"}, [][]string{
		{"TXT", orDash(d.TXTRecord), orDash(d.TXTValue)},
		{"CNAME", orDash(d.CNAMERecord), orDash(d.CNAMETarget)},
	})
	if len(d.Addresses) > 0 {
		fmt.Fprintf(w, "If the DNS provider cannot make a wildcard CNAME, use A (IPv4) or AAAA (IPv6) records for %s to %s instead. On Cloudflare, leave the proxy off.\n", d.CNAMERecord, strings.Join(d.Addresses, ", "))
	}
}

func orgDomainRecords(d api.OrgDomain) []map[string]string {
	if d.Verified {
		return []map[string]string{}
	}
	return []map[string]string{
		{"type": "TXT", "name": d.TXTRecord, "value": d.TXTValue},
		{"type": "CNAME", "name": d.CNAMERecord, "value": d.CNAMETarget},
	}
}

// orgDomainCmd is `whisk domains business`: the business's own domain, which puts every app at
// <app>.<domain> (CLI.md §5).
func orgDomainCmd(s *session) *cobra.Command {
	business := &cobra.Command{Use: "business", Short: "The business's own domain: every app at <app>.<domain>"}

	current := func() (string, *api.Client, api.OrgDomain, bool, error) {
		org, err := s.org()
		if err != nil {
			return "", nil, api.OrgDomain{}, false, err
		}
		client, _, err := s.client()
		if err != nil {
			return "", nil, api.OrgDomain{}, false, err
		}
		all, err := client.OrgDomains(s.ctx, org)
		if err != nil {
			return "", nil, api.OrgDomain{}, false, wrap(err)
		}
		if len(all) == 0 {
			return org, client, api.OrgDomain{}, false, nil
		}
		return org, client, all[0], true, nil
	}
	none := func(org string) error {
		return output.New("NOT_FOUND", org+" has no domain of its own.", "Run whisk domains business add <domain> first.", nil)
	}

	show := &cobra.Command{
		Use:   "show",
		Short: "Show the business's domain and where each app answers on it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, d, ok, err := current()
			if err != nil {
				return err
			}
			if !ok {
				return none(org)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d, "records": orgDomainRecords(d)}, func(w io.Writer) { printOrgDomain(s.printer, w, d) })
			return nil
		},
	}

	add := &cobra.Command{
		Use:   "add <domain>",
		Short: "Use a domain for every app and print the DNS records it needs",
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
			d, err := client.AddOrgDomain(s.ctx, org, strings.ToLower(strings.TrimSpace(args[0])))
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d, "records": orgDomainRecords(d)}, func(w io.Writer) { printOrgDomain(s.printer, w, d) })
			return nil
		},
	}

	verify := &cobra.Command{
		Use:   "verify",
		Short: "Check the DNS records; every app then answers on the domain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, client, d, ok, err := current()
			if err != nil {
				return err
			}
			if !ok {
				return none(org)
			}
			d, err = client.VerifyOrgDomain(s.ctx, org, d.ID)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d}, func(w io.Writer) { printOrgDomain(s.printer, w, d) })
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove",
		Short: "Stop using the domain; the apps keep their Whisk addresses",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, client, d, ok, err := current()
			if err != nil {
				return err
			}
			if !ok {
				return none(org)
			}
			if err := client.DeleteOrgDomain(s.ctx, org, d.ID); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d.Domain, "removed": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Removed %s.\n", d.Domain)
			})
			return nil
		},
	}
	business.AddCommand(show, add, verify, remove)
	return business
}
