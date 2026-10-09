package whisk

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// cdnStatusText is where the app's CDN switch has got to, in plain words.
func cdnStatusText(status string) string {
	switch status {
	case "off":
		return "off: visitors reach the app directly"
	case "starting":
		return "starting: getting certificates while visitors still reach the app directly"
	case "switching":
		return "switching: visitors move over to the CDN within fifteen minutes"
	case "live":
		return "live: served from near your visitors"
	case "stopping":
		return "stopping: visitors move back to the app directly within fifteen minutes"
	}
	return orDash(status)
}

// cdnReasonText is why a hostname reaches the app directly, in plain words.
func cdnReasonText(reason string) string {
	switch reason {
	case "bare":
		return "a bare domain pointed by A records cannot follow the app's address"
	case "business":
		return "an address on the business's own domain"
	case "preview":
		return "a preview"
	}
	return orDash(reason)
}

// cdnHostnameText is one hostname's route.
func cdnHostnameText(h api.CDNHostname) string {
	if h.ThroughCDN {
		return "through the CDN"
	}
	return "direct (" + cdnReasonText(string(h.Reason)) + ")"
}

// cdnTrafficText is this month's traffic against the plan's allowance.
func cdnTrafficText(c api.CDN) string {
	if c.AllowanceBytes <= 0 {
		return fmt.Sprintf("Traffic this month: %s for this app, %s for the business.", billingSize(c.TrafficBytes), billingSize(c.OrgTrafficBytes))
	}
	line := fmt.Sprintf("Traffic this month: %s for this app; the business has used %s of %s included.",
		billingSize(c.TrafficBytes), billingSize(c.OrgTrafficBytes), billingSize(c.AllowanceBytes))
	if c.OrgTrafficBytes > c.AllowanceBytes {
		line += " Traffic past the allowance is charged per started gigabyte on the next invoice."
	}
	return line
}

// printCDN is the human view of an app's CDN.
func printCDN(p output.Printer, w io.Writer, app string, c api.CDN) {
	fmt.Fprintf(w, "CDN for %s: %s\n", app, cdnStatusText(string(c.Status)))
	if c.Since != nil {
		fmt.Fprintf(w, "Since: %s\n", when(c.Since))
	}
	if c.Error != "" {
		fmt.Fprintf(w, "Last problem, still being retried: %s\n", c.Error)
	}
	switch {
	case !c.Included:
		fmt.Fprintln(w, "The CDN is included on the Team and Business plans; an owner upgrades under billing in the dashboard.")
	case !c.Available:
		fmt.Fprintln(w, "The platform's operator has not set the CDN up yet; the app keeps being served directly.")
	case !c.On && c.Status == "off":
		fmt.Fprintln(w, "Turn it on with whisk cdn on. It helps public pages, websites and downloads.")
	}
	if len(c.Hostnames) > 0 {
		rows := make([][]string, len(c.Hostnames))
		for i, h := range c.Hostnames {
			rows[i] = []string{h.Hostname, cdnHostnameText(h)}
		}
		p.Table(w, []string{"HOSTNAME", "ROUTE"}, rows)
	}
	fmt.Fprintln(w, cdnTrafficText(c))
}

// cdnCmd is the app's CDN (CLI.md §5.7, CONTROL-PLANE.md §6.29).
func cdnCmd(s *session) *cobra.Command {
	statusRun := func(cmd *cobra.Command, args []string) error {
		org, app, err := s.target()
		if err != nil {
			return err
		}
		client, _, err := s.client()
		if err != nil {
			return err
		}
		c, err := client.GetCDN(s.ctx, org, app)
		if err != nil {
			return wrap(err)
		}
		s.printer.Result(map[string]any{"org": org, "app": app, "cdn": c}, func(w io.Writer) { printCDN(s.printer, w, app, c) })
		return nil
	}
	set := func(on bool) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			c, err := client.SetCDN(s.ctx, org, app, on)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "cdn": c}, func(w io.Writer) {
				if on {
					fmt.Fprintf(w, "Turning the CDN on for %s. Certificates come first, then visitors move over within fifteen minutes; nothing changes in your DNS.\n", app)
				} else {
					fmt.Fprintf(w, "Turning the CDN off for %s. Visitors move back to the app directly within fifteen minutes.\n", app)
				}
				printCDN(s.printer, w, app, c)
			})
			return nil
		}
	}

	cdn := &cobra.Command{
		Use:   "cdn",
		Short: "Serve the app from near its visitors (Team and Business)",
		Long: `The CDN answers the app's address and its custom domains from the location nearest each
visitor. It helps public pages, websites and downloads; a signed-in tool gains little, since its
pages are private and pass through uncached. A public page is cached only for as long as the
app's Cache-Control says, and a response that sets a cookie is never cached.

With no subcommand, whisk cdn prints the CDN's status.`,
		Args: cobra.NoArgs,
		RunE: statusRun,
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "Whether the CDN is on, each hostname's route, and this month's traffic",
		Args:  cobra.NoArgs,
		RunE:  statusRun,
	}
	on := &cobra.Command{
		Use:   "on",
		Short: "Serve the app through the CDN",
		Args:  cobra.NoArgs,
		RunE:  set(true),
	}
	off := &cobra.Command{
		Use:   "off",
		Short: "Serve the app directly again",
		Args:  cobra.NoArgs,
		RunE:  set(false),
	}
	cdn.AddCommand(status, on, off)
	return cdn
}
