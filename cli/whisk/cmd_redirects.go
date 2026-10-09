package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/doctor"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/redirects"
)

// redirectsCmd is the app's redirects (CONTRACT.md §3.1): checked and tried locally against
// whisk.yaml and its redirects file, and listed as they are live.
func redirectsCmd(s *session) *cobra.Command {
	cmd := &cobra.Command{Use: "redirects", Short: "Old addresses the edge redirects (whisk.yaml redirects and redirects_file)"}

	// load reads the redirects in the working directory, failing as doctor does on an error.
	load := func() (doctor.RedirectSet, error) {
		set, findings, err := doctor.LoadRedirects(s.ctx, s.env.Dir)
		if err != nil {
			return set, err
		}
		if len(findings) > 0 {
			if !s.printer.JSON {
				printReport(s.printer, s.env.Stdout, doctor.Report{Findings: findings})
			}
			e := output.New("DOCTOR_FAILED", strconv.Itoa(len(findings))+" error(s) in the redirects must be fixed before deploying.",
				"Fix each finding (details.findings, or the list above; the fix says how).", map[string]any{"findings": findings})
			e.Exit = output.ExitValidation
			return set, e
		}
		return set, nil
	}

	check := &cobra.Command{
		Use:   "check",
		Short: "Check the redirects as the push will, and list chains",
		Long: `Reads whisk.yaml's redirects and the file redirects_file names, and checks them together as the
push does: each line, duplicates, loops and the limit of 10,000. Exit 3 when one is wrong. A chain
(a rule whose target another rule redirects again) is listed but does not fail: point the first
rule straight at the final address.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := load()
			if err != nil {
				return err
			}
			chains := set.Chains()
			s.printer.Result(map[string]any{"ok": true, "rules": len(set.Rules), "limit": redirects.MaxRules, "chains": nonNil(chains)}, func(w io.Writer) {
				fmt.Fprintf(w, "%d redirects, at most %d.\n", len(set.Rules), redirects.MaxRules)
				if len(chains) > 0 {
					printReport(s.printer, w, doctor.Report{Findings: chains, Warnings: len(chains)})
				}
			})
			return nil
		},
	}

	var method string
	test := &cobra.Command{
		Use:   "test <path or URL>",
		Short: "Show what the edge answers a request for an old address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := load()
			if err != nil {
				return err
			}
			path, query := splitTarget(args[0])
			a, ok := redirects.Compile(set.Rules).Match(path, query, "")
			if !ok {
				s.printer.Result(map[string]any{"path": path, "query": query, "matched": false}, func(w io.Writer) {
					fmt.Fprintf(w, "No redirect: the app answers %s.\n", args[0])
				})
				return nil
			}
			status := redirects.ForMethod(a.Status, strings.ToUpper(method))
			rule := map[string]any{"from": set.Rules[a.Rule].From, "file": set.File[a.Rule], "line": set.Line[a.Rule]}
			s.printer.Result(map[string]any{"path": path, "query": query, "matched": true, "status": status, "location": a.Location, "rule": rule}, func(w io.Writer) {
				if status == 410 {
					fmt.Fprintf(w, "410 Gone, by %s line %d.\n", set.File[a.Rule], set.Line[a.Rule])
					return
				}
				fmt.Fprintf(w, "%d to %s, by %s line %d.\n", status, a.Location, set.File[a.Rule], set.Line[a.Rule])
			})
			return nil
		},
	}
	test.Flags().StringVar(&method, "method", "GET", "the request's method: a 301 or 302 answers anything but GET and HEAD with 308 or 307")

	list := &cobra.Command{
		Use:   "list",
		Short: "List the redirects live in production, and the domains that redirect",
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
			m, err := client.LiveManifest(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			live := []redirects.Rule{}
			if raw, err := json.Marshal(m.Content["redirects"]); err == nil {
				_ = json.Unmarshal(raw, &live)
			}
			if live == nil {
				live = []redirects.Rule{}
			}
			all, err := client.ListDomains(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			domains := []map[string]string{}
			for _, d := range all {
				if d.RedirectTo != "" {
					domains = append(domains, map[string]string{"hostname": d.Hostname, "redirect_to": d.RedirectTo})
				}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "commit": m.CommitSHA, "rules": live, "domains": domains}, func(w io.Writer) {
				fmt.Fprintf(w, "%d redirects live, from commit %s.\n", len(live), shortSHA(m.CommitSHA))
				rows := make([][]string, len(live))
				for i, r := range live {
					r = r.Normal()
					rows[i] = []string{r.From, orDash(r.To), strconv.Itoa(r.Status), r.Query}
				}
				if len(rows) > 0 {
					s.printer.Table(w, []string{"FROM", "TO", "STATUS", "QUERY"}, rows)
				}
				for _, d := range domains {
					fmt.Fprintf(w, "%s redirects to %s.\n", d["hostname"], d["redirect_to"])
				}
			})
			return nil
		},
	}
	cmd.AddCommand(check, test, list)
	return cmd
}

// splitTarget is the path and raw query of a path or a full URL.
func splitTarget(arg string) (string, string) {
	if u, err := url.Parse(arg); err == nil && u.Scheme != "" {
		p := u.Path
		if p == "" {
			p = "/"
		}
		return p, u.RawQuery
	}
	p, q, _ := strings.Cut(arg, "?")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if u, err := url.PathUnescape(p); err == nil {
		p = u
	}
	return p, q
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return orDash(sha)
}
