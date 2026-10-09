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

// findDomain resolves a hostname to the app's domain record.
func findDomain(domains []api.Domain, hostname string) (api.Domain, bool) {
	for _, d := range domains {
		if strings.EqualFold(d.Hostname, hostname) {
			return d, true
		}
	}
	return api.Domain{}, false
}

func printDNS(p output.Printer, w io.Writer, d api.Domain) {
	if d.Verified {
		fmt.Fprintf(w, "%s is verified; certificate %s.\n", d.Hostname, orDash(d.CertStatus))
		return
	}
	fmt.Fprintf(w, "Create these DNS records for %s, then run whisk domains verify %s:\n", d.Hostname, d.Hostname)
	records := dnsRecords(d)
	rows := make([][]string, len(records))
	for i, r := range records {
		rows[i] = []string{r["type"], orDash(r["name"]), orDash(r["value"])}
	}
	p.Table(w, []string{"TYPE", "NAME", "VALUE"}, rows)
	if len(d.Addresses) > 0 && !pointsByAddress(records) {
		fmt.Fprintf(w, "A bare domain such as example.com cannot have a CNAME: point it with A (IPv4) or AAAA (IPv6) records to %s instead.\n", strings.Join(d.Addresses, ", "))
	}
}

func domainsCmd(s *session) *cobra.Command {
	domains := &cobra.Command{Use: "domains", Short: "Custom hostnames of the app"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's hostnames and their verification state",
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
			all, err := client.ListDomains(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "domains": all}, func(w io.Writer) {
				rows := make([][]string, len(all))
				for i, d := range all {
					verified := "pending"
					if d.Verified {
						verified = "verified"
					}
					rows[i] = []string{d.Hostname, string(d.Kind), verified, orDash(d.CertStatus), orDash(string(d.AddedBy)), orDash(d.RedirectTo)}
				}
				s.printer.Table(w, []string{"HOSTNAME", "KIND", "DNS", "CERTIFICATE", "ADDED BY", "REDIRECTS TO"}, rows)
			})
			return nil
		},
	}

	add := &cobra.Command{
		Use:   "add <hostname>",
		Short: "Attach a custom hostname and print the DNS records it needs",
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
			d, err := client.AddDomain(s.ctx, org, app, strings.ToLower(strings.TrimSpace(args[0])))
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "domain": d, "records": dnsRecords(d)}, func(w io.Writer) { printDNS(s.printer, w, d) })
			return nil
		},
	}

	verify := &cobra.Command{
		Use:   "verify <hostname>",
		Short: "Check the DNS records and issue the certificate",
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
			all, err := client.ListDomains(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			d, ok := findDomain(all, args[0])
			if !ok {
				return output.New("NOT_FOUND", fmt.Sprintf("%s is not a hostname of %s/%s.", args[0], org, app), "Run whisk domains add "+args[0]+" first; whisk domains list shows the hostnames.", map[string]any{"hostname": args[0]})
			}
			d, err = client.VerifyDomain(s.ctx, org, app, d.ID)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "domain": d}, func(w io.Writer) { printDNS(s.printer, w, d) })
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove <hostname>",
		Short: "Detach a custom hostname",
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
			all, err := client.ListDomains(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			d, ok := findDomain(all, args[0])
			if !ok {
				return output.New("NOT_FOUND", fmt.Sprintf("%s is not a hostname of %s/%s.", args[0], org, app), "whisk domains list shows the hostnames.", map[string]any{"hostname": args[0]})
			}
			if err := client.DeleteDomain(s.ctx, org, app, d.ID); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "hostname": d.Hostname, "removed": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Removed %s.\n", d.Hostname)
			})
			return nil
		},
	}
	var off bool
	redirect := &cobra.Command{
		Use:   "redirect <hostname> [<to>]",
		Short: "Send every request to a custom domain on to another hostname of the app",
		Long: `The custom domain answers every request with a permanent redirect (301 for GET and HEAD, 308 for
anything else) to the same path and query on <to>, another hostname of the app: its address or
another verified custom domain. A path the app's redirects name goes straight to the rule's
target on <to>, in one hop. Use it for the old domain after a site moves, and to send
www.example.com to example.com. --off serves the app on the domain again.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			to := ""
			switch {
			case off && len(args) == 2:
				return output.New("INVALID_REQUEST", "--off takes no target.", "Run whisk domains redirect "+args[0]+" --off, or name the target without --off.", nil)
			case !off && len(args) == 1:
				return output.New("INVALID_REQUEST", "Name the hostname to redirect to.", "Run whisk domains redirect "+args[0]+" <to>, such as example.com, or --off to stop.", nil)
			case !off:
				to = strings.ToLower(strings.TrimSpace(args[1]))
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			all, err := client.ListDomains(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			d, ok := findDomain(all, args[0])
			if !ok {
				return output.New("NOT_FOUND", fmt.Sprintf("%s is not a hostname of %s/%s.", args[0], org, app), "Run whisk domains add "+args[0]+" first; whisk domains list shows the hostnames.", map[string]any{"hostname": args[0]})
			}
			d, err = client.SetDomainRedirect(s.ctx, org, app, d.ID, to)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "domain": d}, func(w io.Writer) {
				if d.RedirectTo == "" {
					fmt.Fprintf(w, "%s serves the app.\n", d.Hostname)
					return
				}
				fmt.Fprintf(w, "%s redirects every request to the same path on %s.\n", d.Hostname, d.RedirectTo)
			})
			return nil
		},
	}
	redirect.Flags().BoolVar(&off, "off", false, "serve the app on the domain again")
	domains.AddCommand(list, add, verify, remove, redirect, orgDomainCmd(s))
	return domains
}

// dnsRecords is the JSON form of the records a custom domain needs: the API's own list, which
// gives a bare domain A and AAAA records instead of a CNAME, or the TXT and CNAME from an API
// that does not send one.
func dnsRecords(d api.Domain) []map[string]string {
	if d.Verified || d.Kind != "custom" {
		return []map[string]string{}
	}
	if len(d.Records) > 0 {
		out := make([]map[string]string, len(d.Records))
		for i, r := range d.Records {
			out[i] = map[string]string{"type": r.Type, "name": r.Name, "value": r.Value}
		}
		return out
	}
	return []map[string]string{
		{"type": "TXT", "name": d.TXTRecord, "value": d.TXTValue},
		{"type": "CNAME", "name": d.Hostname, "value": d.CNAMETarget},
	}
}

// pointsByAddress reports whether the records already point the name with A or AAAA records.
func pointsByAddress(records []map[string]string) bool {
	for _, r := range records {
		if r["type"] == "A" || r["type"] == "AAAA" {
			return true
		}
	}
	return false
}

func envsCmd(s *session) *cobra.Command {
	envs := &cobra.Command{Use: "envs", Short: "Environments: production and the previews"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List environments with their URLs and expiry",
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
			all, err := client.ListEnvironments(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "environments": all}, func(w io.Writer) {
				rows := make([][]string, len(all))
				for i, e := range all {
					db := "-"
					if e.HasDatabase {
						db = "yes"
					}
					rows[i] = []string{e.Name, e.URL, envState(e.Status), orDash(e.CurrentDeployID), db, when(e.ExpiresAt)}
				}
				s.printer.Table(w, []string{"ENVIRONMENT", "URL", "STATE", "DEPLOY", "DATABASE", "EXPIRES"}, rows)
			})
			return nil
		},
	}

	del := &cobra.Command{
		Use:   "delete preview:<branch>",
		Short: "Delete a preview environment and its database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if name == "production" {
				return output.New("INVALID_REQUEST", "production cannot be deleted.", "Delete the app with whisk apps delete if that is what you mean.", nil)
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if err := client.DeleteEnvironment(s.ctx, org, app, name); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "environment": name, "deleted": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Deleted %s.\n", name)
			})
			return nil
		},
	}
	var noWait bool
	start := &cobra.Command{
		Use:   "start <branch>",
		Short: "Start the preview of a branch already pushed to the app",
		Long: "A push of a branch only stores it. This starts the branch's preview at its own URL, with its own empty database, " +
			"and follows the deploy; from then on every push to the branch updates the preview.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := strings.TrimPrefix(args[0], "preview:")
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			started, err := client.StartPreview(s.ctx, org, app, branch)
			if err != nil {
				return wrap(err)
			}
			d := started.Deploy
			if noWait {
				s.printer.Result(map[string]any{"deploy_id": d.ID, "status": d.Status, "environment": started.Environment.Name, "url": started.Environment.URL}, func(w io.Writer) {
					fmt.Fprintf(w, "Deploy %s queued for %s. It will be at %s. Follow it with: whisk deploys info %s\n", d.ID, started.Environment.Name, started.Environment.URL, d.ID)
				})
				return nil
			}
			return s.waitForDeploy(client, org, app, d)
		},
	}
	start.Flags().BoolVar(&noWait, "no-wait", false, "print the deploy id and exit once the preview is queued")
	envs.AddCommand(list, start, del)
	return envs
}

// envState is an environment's state in words.
func envState(status apitypes.EnvironmentStatus) string {
	if status == apitypes.EnvironmentSleeping {
		return "asleep"
	}
	return "awake"
}
