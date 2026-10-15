package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

func statusCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "The app's state, current deploy, URL and anything waiting on a human",
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
			a, err := client.GetApp(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			var current *api.Deploy
			if a.CurrentDeployID != "" {
				if d, err := client.GetDeploy(s.ctx, org, app, a.CurrentDeployID); err == nil {
					current = &d
				} else if !api.IsCode(err, "NOT_FOUND") {
					return wrap(err)
				}
			}
			unset := nonNilStrings(a.UnsetSecrets)
			result := map[string]any{"org": org, "app": a, "state": a.State, "url": a.URL, "deploy": current, "unset_secrets": unset, "last_deploy_at": a.LastDeployAt, "problem": a.Problem}
			// The newest package check, on a plan with scanning (CONTROL-PLANE.md §6.28). It is
			// a summary; whisk scan lists the findings.
			var packages api.Packages
			if pk, err := client.GetPackages(s.ctx, org, app); err == nil && pk.Included {
				packages = pk
				summary := map[string]any{"checked_at": nil, "counts": nil, "pending": pk.Pending != nil}
				if pk.Scan != nil {
					summary["checked_at"], summary["counts"], summary["commit_sha"] = pk.Scan.FinishedAt, pk.Scan.Counts, pk.Scan.CommitSHA
				}
				result["packages"] = summary
			}
			var block *output.Error
			if len(unset) > 0 {
				block = needsHumanSecrets(unset, secretsURL(s.dashboard(), org, app))
				result["needs_human"] = block.Body()["error"]
			}
			s.printer.Result(result, func(w io.Writer) {
				fmt.Fprintf(w, "%s/%s  %s  %s\n", org, a.Slug, s.printer.Bold(orDash(string(a.State))), a.URL)
				if current == nil {
					fmt.Fprintln(w, "  no deploy yet; run whisk deploy")
				} else {
					fmt.Fprintf(w, "  deploy %s  %s  %s%s  %s  by %s\n", current.ID, current.Status, short(current.CommitSHA), branchSuffix(current.Branch), at(current.CreatedAt), orDash(current.TriggeredByLabel))
					if current.Message != "" {
						fmt.Fprintf(w, "  %s\n", current.Message)
					}
					if current.Error != nil {
						fmt.Fprintf(w, "  %s: %s\n", s.printer.Bad(current.Error.Code), current.Error.Message)
					}
				}
				if a.Problem != nil {
					fmt.Fprintf(w, "  %s %s (%s)\n", s.printer.Bad(a.Problem.Code+":"), a.Problem.Message, at(a.Problem.At))
					for _, l := range a.Problem.Log {
						fmt.Fprintf(w, "    %s\n", l)
					}
				}
				if line := packagesLine(packages); line != "" {
					fmt.Fprintln(w, "  "+line)
				}
				if block != nil {
					s.printer.Block(w, block)
				}
			})
			return nil
		},
	}
}

// openTargets are the pages whisk open knows. The app itself is its URL; everything else is a
// dashboard page for the app.
var openTargets = map[string]string{
	"dashboard": "", "app": "", "deploys": "/deploys", "logs": "/logs", "runs": "/runs",
	"secrets": "/secrets", "domains": "/domains", "access": "/access", "errors": "/errors",
}

func openCmd(s *session) *cobra.Command {
	var noBrowser bool
	c := &cobra.Command{
		Use:   "open [dashboard|app|deploys|logs|runs|secrets|domains|access|errors]",
		Short: "Print the dashboard page or the app's URL and open it in a browser when one exists",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "dashboard"
			if len(args) == 1 {
				target = strings.ToLower(args[0])
			}
			page, ok := openTargets[target]
			if !ok {
				return output.New("INVALID_REQUEST", fmt.Sprintf("%q is not something whisk open knows.", args[0]), "Pass one of: "+strings.Join(output.SortedKeys(openTargets), ", ")+".", nil)
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			var url string
			if target == "app" {
				client, _, err := s.client()
				if err != nil {
					return err
				}
				a, err := client.GetApp(s.ctx, org, app)
				if err != nil {
					return wrap(err)
				}
				url = a.URL
			} else {
				url = appPage(s.dashboard(), org, app, page)
			}
			if !noBrowser && s.env.IsTerminal {
				openBrowser(url)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "target": target, "url": url}, func(w io.Writer) {
				fmt.Fprintln(w, url)
			})
			return nil
		},
	}
	c.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL only")
	return c
}
