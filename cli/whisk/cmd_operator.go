package whisk

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// operatorCmd is the platform operator's view (CLI.md §5.13): the platform's own logs, its
// nodes and its deploys across every org, its own error log, demo businesses and comps. It answers for an operator's session or an
// operator read token in WHISK_TOKEN; anyone else is told the routes do not exist.
func operatorCmd(s *session) *cobra.Command {
	root := &cobra.Command{
		Use:   "operator",
		Short: "The platform's own logs, nodes and deploys (operators only)",
	}
	root.AddCommand(operatorLogsCmd(s), operatorUnitsCmd(s), operatorNodesCmd(s), operatorDeploysCmd(s), operatorDemoCmd(s), operatorCompCmd(s), operatorHoldsCmd(s), operatorErrorsCmd(s), operatorResourcesCmd(s))
	return root
}

func operatorLogsCmd(s *session) *cobra.Command {
	var q api.PlatformLogQuery
	var follow bool
	c := &cobra.Command{
		Use:   "logs",
		Short: "What the platform's services printed",
		Long: `Reads the platform's own lines: the control plane, the node agent, Caddy, Postgres and every
other platform service, never a tenant app's output. --unit narrows to one service (whisk operator
units lists them), --host to one machine. --grep takes free text, or field=value for a field of a
JSON line. --since and --until take a duration (30m, 6h, 7d) or an RFC 3339 time.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if !follow {
				lines, err := client.PlatformLogs(s.ctx, q)
				if err != nil {
					return wrap(err)
				}
				s.printer.Result(map[string]any{"lines": lines}, func(w io.Writer) {
					if len(lines) == 0 {
						fmt.Fprintln(w, "Nothing in that window. Widen --since, or check the unit name with whisk operator units.")
						return
					}
					for _, l := range lines {
						fmt.Fprintln(w, formatPlatformLine(l))
					}
				})
				return nil
			}
			ctx, stop := signal.NotifyContext(s.ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			out := s.env.Stdout
			err = client.TailPlatformLogs(ctx, q, func(batch []api.LogLine) error {
				for _, l := range batch {
					fmt.Fprintln(out, formatPlatformLine(l))
				}
				return nil
			})
			if err != nil && ctx.Err() == nil {
				return wrap(err)
			}
			return nil
		},
	}
	f := c.Flags()
	f.BoolVarP(&follow, "follow", "f", false, "keep the stream open and print lines as they arrive")
	f.StringVar(&q.Unit, "unit", "", "one service, such as whiskd.service or whisk-node.service")
	f.StringVar(&q.Host, "host", "", "one machine")
	f.StringVar(&q.Stream, "stream", "", "stdout or stderr")
	f.StringVar(&q.Grep, "grep", "", "free text, or field=value for a field of a JSON line")
	f.StringVar(&q.Since, "since", "1h", "how far back to read: 30m, 6h, 7d, or an RFC 3339 time")
	f.StringVar(&q.Until, "until", "", "where the window ends: a duration ago or an RFC 3339 time")
	f.IntVar(&q.Limit, "limit", 200, "how many lines to read when not following")
	return c
}

// formatPlatformLine is one platform line as a person reads it: the time, the host and unit,
// stderr when it is, and the message.
func formatPlatformLine(l api.LogLine) string {
	prefix := l.At.Local().Format("01-02 15:04:05.000")
	source := strings.TrimSuffix(l.Unit, ".service")
	if l.Host != "" {
		source = l.Host + "/" + source
	}
	if source != "" {
		prefix += " " + source
	}
	if l.Stream == "stderr" {
		prefix += " stderr"
	}
	return prefix + "  " + l.Line
}

func operatorUnitsCmd(s *session) *cobra.Command {
	var since string
	c := &cobra.Command{
		Use:   "units",
		Short: "The platform services and hosts that logged lately",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			units, hosts, err := client.PlatformLogLabels(s.ctx, since)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"units": units, "hosts": hosts}, func(w io.Writer) {
				fmt.Fprintf(w, "Units: %s\nHosts: %s\n", orDash(strings.Join(units, ", ")), orDash(strings.Join(hosts, ", ")))
			})
			return nil
		},
	}
	c.Flags().StringVar(&since, "since", "24h", "how far back to look")
	return c
}

func operatorNodesCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "nodes",
		Short: "Every node: status, agent version, apps, last heartbeat",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			nodes, err := client.OperatorNodes(s.ctx)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"nodes": nodes}, func(w io.Writer) {
				if len(nodes) == 0 {
					fmt.Fprintln(w, "No nodes are registered.")
					return
				}
				rows := make([][]string, len(nodes))
				for i, n := range nodes {
					rows[i] = []string{n.Name, n.Region, string(n.Status), orDash(n.AgentVersion), fmt.Sprint(n.Apps), when(n.LastSeenAt)}
				}
				s.printer.Table(w, []string{"NODE", "REGION", "STATUS", "AGENT", "APPS", "LAST SEEN"}, rows)
				// Each node's free budget, as the control plane words it (usage.free.line).
				for _, n := range nodes {
					if line := freeLine(n.Usage); line != "" {
						fmt.Fprintf(w, "%s: %s\n", n.Name, line)
					}
				}
				// Where each node's databases are copied, and a failover not yet fenced.
				for _, n := range nodes {
					for _, line := range copyLines(n) {
						fmt.Fprintf(w, "%s: %s\n", n.Name, line)
					}
				}
			})
			return nil
		},
	}
}

func operatorDeploysCmd(s *session) *cobra.Command {
	var status, since string
	c := &cobra.Command{
		Use:   "deploys",
		Short: "Deploys across every org: failed ones by default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			deploys, err := client.OperatorDeploys(s.ctx, status, since)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"deploys": deploys}, func(w io.Writer) {
				if len(deploys) == 0 {
					fmt.Fprintln(w, "No deploys match in that window.")
					return
				}
				rows := make([][]string, len(deploys))
				for i, d := range deploys {
					code := "-"
					if d.Error != nil {
						code = d.Error.Code
					}
					rows[i] = []string{d.ID, d.AppID, d.Environment, short(d.CommitSHA), string(d.Status), code, at(d.CreatedAt)}
				}
				s.printer.Table(w, []string{"DEPLOY", "APP", "ENV", "COMMIT", "STATUS", "ERROR", "CREATED"}, rows)
			})
			return nil
		},
	}
	c.Flags().StringVar(&status, "status", "failed", "a deploy status, or all")
	c.Flags().StringVar(&since, "since", "24h", "how far back to read: 6h, 7d, or an RFC 3339 time")
	return c
}

// copyLines are the node's copy line, as the control plane words it (CONTROL-PLANE.md §6.35),
// and, while it has a failover not yet fenced, what moved and what waits.
func copyLines(n api.Node) []string {
	var out []string
	if n.Copy != nil && n.Copy.Line != "" {
		out = append(out, n.Copy.Line)
	}
	if f := n.Failover; f != nil {
		line := "Failed over to " + orDash(f.Holder) + " at " + at(f.StartedAt)
		if len(f.Apps) > 0 {
			line += ": " + strings.Join(f.Apps, ", ") + " moved"
		}
		if len(f.Stranded) > 0 {
			line += "; waiting: " + strings.Join(f.Stranded, ", ") + " (" + f.Reason + ")"
		}
		out = append(out, line+". It is fenced before it takes work again.")
	}
	return out
}

// freeLine is the node's free apps line from its usage, or "" when it keeps no budget.
func freeLine(usage map[string]any) string {
	free, _ := usage["free"].(map[string]any)
	line, _ := free["line"].(string)
	return line
}

// operatorDemoCmd makes and lists demo businesses (CLI.md §5.14, CONTROL-PLANE.md §6.1): a
// business made for another company, built in like any other with --org, and handed over with
// its link to someone with an email at the company's domain.
func operatorDemoCmd(s *session) *cobra.Command {
	root := &cobra.Command{
		Use:   "demo",
		Short: "Make a business for another company and hand it over with a link",
	}
	var domain, slug string
	create := &cobra.Command{
		Use:   "create <company name>",
		Short: "Make a demo business for a company, owned by you until they claim it",
		Long: `Makes a business for another company. Build in it like any other with --org <slug>. Send the
printed link to someone at the company: once they sign in with an email at --domain and accept,
they own it and you and your agents leave it, with apps, data and addresses unchanged.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			d, err := client.CreateDemo(s.ctx, args[0], domain, slug)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"demo": d}, func(w io.Writer) {
				fmt.Fprintf(w, "Made %s (%s) for %s.\n", d.Org.Name, d.Org.Slug, d.Domain)
				fmt.Fprintf(w, "Build in it with --org %s. Handover link: %s\n", d.Org.Slug, d.ClaimURL)
			})
			return nil
		},
	}
	create.Flags().StringVar(&domain, "domain", "", "the company's email domain, such as harbourbuilding.com")
	create.Flags().StringVar(&slug, "slug", "", "the business's slug; from the name when not given")
	_ = create.MarkFlagRequired("domain")
	list := &cobra.Command{
		Use:   "list",
		Short: "Your demo businesses, with their links and who claimed them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			demos, err := client.Demos(s.ctx)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"demos": demos}, func(w io.Writer) {
				if len(demos) == 0 {
					fmt.Fprintln(w, "No demo businesses. Make one with whisk operator demo create.")
					return
				}
				rows := make([][]string, len(demos))
				for i, d := range demos {
					status, link := "waiting", d.ClaimURL
					if d.ClaimedAt != nil {
						status, link = "claimed by "+d.ClaimedBy+" "+when(d.ClaimedAt), "-"
					}
					rows[i] = []string{d.Org.Slug, d.Domain, status, link}
				}
				s.printer.Table(w, []string{"BUSINESS", "FOR", "STATUS", "HANDOVER LINK"}, rows)
			})
			return nil
		},
	}
	root.AddCommand(create, list)
	return root
}

// operatorCompCmd gives a business a paid plan free of charge and takes it back (CLI.md §5.14,
// CONTROL-PLANE.md §6.15). Only the operator does, never an agent identity or a read token.
func operatorCompCmd(s *session) *cobra.Command {
	root := &cobra.Command{
		Use:   "comp",
		Short: "Give a business a paid plan free of charge",
	}
	var until, reason string
	start := &cobra.Command{
		Use:   "start <business> <plan>",
		Short: "Put a business on a paid plan free of charge, or change its comp",
		Long: `Puts a business on starter, team or business (agency for a client business) with no charge.
With --until it goes back to Free at the end of that day (UTC); without, it stays comped until you
end it. The reason goes in the business's audit log. Its owners are told.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			o, err := client.CompOrg(s.ctx, args[0], args[1], until, reason)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": o}, func(w io.Writer) {
				fmt.Fprintf(w, "%s is on %s free of charge %s.\n", o.Name, o.Plan, compUntil(o.Comp))
			})
			return nil
		},
	}
	start.Flags().StringVar(&until, "until", "", "the last day of the comp, such as 2027-03-31; none for no end")
	start.Flags().StringVar(&reason, "reason", "", "why the business is comped (at least ten characters)")
	_ = start.MarkFlagRequired("reason")
	end := &cobra.Command{
		Use:   "end <business>",
		Short: "End a comp now; the business goes back to Free and keeps its apps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			o, err := client.EndComp(s.ctx, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": o}, func(w io.Writer) {
				fmt.Fprintf(w, "%s is back on %s.\n", o.Name, o.Plan)
			})
			return nil
		},
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "The comped businesses, with their plan, end and reason",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			comps, err := client.Comps(s.ctx)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"orgs": comps}, func(w io.Writer) {
				if len(comps) == 0 {
					fmt.Fprintln(w, "No comped businesses. Comp one with whisk operator comp start.")
					return
				}
				rows := make([][]string, len(comps))
				for i, o := range comps {
					rows[i] = []string{o.Slug, o.Comp.Plan, compUntil(o.Comp), o.Comp.Reason}
				}
				s.printer.Table(w, []string{"BUSINESS", "PLAN", "ENDS", "REASON"}, rows)
			})
			return nil
		},
	}
	root.AddCommand(start, end, list)
	return root
}

// compUntil words a comp's end: "until 2027-03-31", or "with no end".
func compUntil(c *api.Comp) string {
	if c == nil || c.EndsAt == nil {
		return "with no end"
	}
	return "until " + c.EndsAt.UTC().Format("2006-01-02")
}

// operatorHoldsCmd lists the abuse holds (CONTROL-PLANE.md §6.27). Releasing or removing one
// is an operator's own decision, made on the operator page.
func operatorHoldsCmd(s *session) *cobra.Command {
	var status string
	c := &cobra.Command{
		Use:   "holds",
		Short: "Businesses the abuse check held for review",
		Long: `Lists the businesses the abuse check took offline, newest first: what found them, the
person, their sign-up address, and the state. --status narrows to held, released or removed.
Release or remove one on the operator page.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			holds, err := client.Holds(s.ctx, status)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"holds": holds}, func(w io.Writer) {
				if len(holds) == 0 {
					fmt.Fprintln(w, "Nothing held.")
					return
				}
				rows := make([][]string, len(holds))
				for i, h := range holds {
					rows[i] = []string{h.Org, string(h.Status), holdFound(h), h.Person, strings.Join(h.IPs, " "), h.CreatedAt.Format("2006-01-02 15:04")}
				}
				s.printer.Table(w, []string{"BUSINESS", "STATE", "FOUND", "PERSON", "ADDRESS", "WHEN"}, rows)
			})
			return nil
		},
	}
	c.Flags().StringVar(&status, "status", "", "held, released or removed")
	return c
}

// holdFound says what found a held business, in a few words.
func holdFound(h api.Hold) string {
	d := func(k string) string { return fmt.Sprint(h.Detail[k]) }
	switch h.Kind {
	case "phishing_page":
		return "password form naming " + d("brand")
	case "blocklist":
		return "on " + d("list")
	case "web_risk":
		return "Web Risk: " + d("threats")
	case "email_link":
		return "emailed a link to " + d("hostname")
	case "linked":
		return "same " + d("matched") + " as a held business"
	}
	return string(h.Kind)
}
