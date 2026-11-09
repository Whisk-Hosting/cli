package whisk

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// emailDomainName is a sending domain as a person typed it: trimmed, lower case, without a
// leading @ or a trailing dot. Pure.
func emailDomainName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.TrimPrefix(name, "@")
	name = strings.TrimSuffix(name, ".")
	return strings.ToLower(strings.TrimSpace(name))
}

// findEmailDomain is the business's sending domain with exactly the name given, never a near
// match: a business holds a name once, so the name alone picks the id to act on. Pure.
func findEmailDomain(all []api.EmailDomain, name string) (api.EmailDomain, bool) {
	want := emailDomainName(name)
	for _, d := range all {
		if want != "" && emailDomainName(d.Domain) == want {
			return d, true
		}
	}
	return api.EmailDomain{}, false
}

// emailDomainRecords is the JSON form of the records a sending domain still needs: the API's
// own list while the domain is pending, nothing once it is verified or paused. Pure.
func emailDomainRecords(d api.EmailDomain) []map[string]string {
	if d.Status != apitypes.EmailDomainPending {
		return []map[string]string{}
	}
	out := make([]map[string]string, len(d.DNSRecords))
	for i, r := range d.DNSRecords {
		out[i] = map[string]string{"type": r.Type, "name": r.Name, "value": r.Value}
		if r.Host != "" {
			out[i]["host"] = r.Host
		}
	}
	return out
}

// emailDomainSays is the sentence a person reads for a sending domain that needs no records
// published. Pure.
func emailDomainSays(d api.EmailDomain) string {
	who := "the business's apps send"
	if d.App != "" {
		who = d.App + " alone sends"
	}
	switch d.Status {
	case apitypes.EmailDomainVerified:
		return fmt.Sprintf("%s is verified: %s from any address at it.", d.Domain, who)
	case apitypes.EmailDomainPending:
		return fmt.Sprintf("%s is pending: its DNS records are not verified yet.", d.Domain)
	case apitypes.EmailDomainPaused:
		return fmt.Sprintf("%s is paused: nothing is sent from it (bounce rate %.1f%%, complaint rate %.2f%%). An owner resumes it in the dashboard under Settings, Email, once the cause is fixed.",
			d.Domain, d.BounceRate*100, d.ComplaintRate*100)
	}
	return fmt.Sprintf("%s is %s.", d.Domain, d.Status)
}

func printEmailDomain(p output.Printer, w io.Writer, d api.EmailDomain) {
	records := emailDomainRecords(d)
	if d.Status != apitypes.EmailDomainPending {
		fmt.Fprintln(w, emailDomainSays(d))
		return
	}
	if len(records) == 0 {
		fmt.Fprintf(w, "%s has no records to publish; run whisk email domains verify %s.\n", d.Domain, d.Domain)
		return
	}
	fmt.Fprintf(w, "Create these DNS records for %s, then run whisk email domains verify %s:\n", d.Domain, d.Domain)
	rows := make([][]string, len(records))
	for i, r := range records {
		rows[i] = []string{r["type"], orDash(r["host"]), orDash(r["name"]), orDash(r["value"])}
	}
	p.Table(w, []string{"TYPE", "HOST", "NAME", "VALUE"}, rows)
}

// emailDomainNotFound is the refusal for a name the business does not send from.
func emailDomainNotFound(org, name, fix string) error {
	return output.New("NOT_FOUND", fmt.Sprintf("%s is not a sending domain of %s.", name, org), fix, map[string]any{"domain": name})
}

// emailDomainsCmd is whisk email domains: the business's own sending domains, the same ones its
// dashboard shows under Settings, Email (CONTROL-PLANE.md §6.12).
func emailDomainsCmd(s *session) *cobra.Command {
	domains := &cobra.Command{
		Use:   "domains",
		Short: "The business's own domains its apps send mail from",
		Long: `A sending domain lets the business's apps send from an address of its own, such as
orders@mail.yourbusiness.com, instead of the business's address on Whisk's domain. Add it, create
the DNS records it prints at the domain's DNS host, then verify it; an app's mail then names a
from address at it. Adding, verifying and removing need an owner or admin.`,
	}
	domains.AddCommand(emailDomainsListCmd(s), emailDomainsAddCmd(s), emailDomainsVerifyCmd(s), emailDomainsRemoveCmd(s))
	return domains
}

// emailDomainNamed is the business's sending domain with exactly the name given, read from the
// list, or NOT_FOUND with fix.
func (s *session) emailDomainNamed(client *api.Client, org, name, fix string) (api.EmailDomain, error) {
	all, err := client.EmailDomains(s.ctx, org)
	if err != nil {
		return api.EmailDomain{}, wrap(err)
	}
	d, ok := findEmailDomain(all, name)
	if !ok {
		return api.EmailDomain{}, emailDomainNotFound(org, emailDomainName(name), fix)
	}
	return d, nil
}

func emailDomainsListCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the business's sending domains and where each stands",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			all, err := client.EmailDomains(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domains": all}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintf(w, "%s has no sending domains; add one with whisk email domains add <domain>.\n", org)
					return
				}
				rows := make([][]string, len(all))
				for i, d := range all {
					verified := "-"
					if !d.VerifiedAt.IsZero() {
						verified = d.VerifiedAt.Local().Format(time.DateTime)
					}
					rows[i] = []string{d.Domain, string(d.Status), orDash(d.App), verified}
				}
				s.printer.Table(w, []string{"DOMAIN", "STATUS", "APP", "VERIFIED"}, rows)
			})
			return nil
		},
	}
}

func emailDomainsAddCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "add <domain>",
		Short: "Send the business's mail from a domain of its own and print the DNS records it needs",
		Long: `Registers the domain for sending and prints the DNS records to create at its DNS host: DKIM,
SPF and the return path's MX, each with its host as a DNS provider's form takes it and its full
name. Use a subdomain such as mail.yourbusiness.com, so the records leave the domain's own mail
alone. Run whisk email domains verify <domain> once they are published.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			d, err := client.AddEmailDomain(s.ctx, org, emailDomainName(args[0]))
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d, "records": emailDomainRecords(d)}, func(w io.Writer) { printEmailDomain(s.printer, w, d) })
			return nil
		},
	}
}

func emailDomainsVerifyCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <domain>",
		Short: "Check a sending domain's DNS records; it is verified once they are published",
		Long: `Asks the mail provider whether the domain's records are in place. Until they are, it answers
DOMAIN_UNVERIFIED with the records in details; DNS can take up to an hour to appear.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			name := emailDomainName(args[0])
			d, err := s.emailDomainNamed(client, org, name, "Run whisk email domains add "+name+" first; whisk email domains list shows the sending domains.")
			if err != nil {
				return err
			}
			d, err = client.VerifyEmailDomain(s.ctx, org, d.ID)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d, "records": emailDomainRecords(d)}, func(w io.Writer) { printEmailDomain(s.printer, w, d) })
			return nil
		},
	}
}

func emailDomainsRemoveCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <domain>",
		Short: "Stop sending from a domain; apps naming a from address at it are refused",
		Long: `Removes the sending domain with exactly the name given. Mail whose from address is at it is
refused from then on, so change the apps first. It cannot be undone, so only a person may do it:
an agent identity is refused with AGENT_CATEGORY. The DNS records can be deleted afterwards.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			d, err := s.emailDomainNamed(client, org, args[0], "whisk email domains list shows the sending domains.")
			if err != nil {
				return err
			}
			if err := client.DeleteEmailDomain(s.ctx, org, d.ID); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "domain": d.Domain, "removed": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Removed %s. Nothing is sent from it now; its DNS records can be deleted.\n", d.Domain)
			})
			return nil
		},
	}
}
