package whisk

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// restartCmd starts the app's live production build again in a new container, the old one
// stopping once the new one is healthy, and follows that deploy (CLI.md §5.2, CONTROL-PLANE.md
// §5.1). Nothing is built.
func restartCmd(s *session) *cobra.Command {
	var noWait bool
	c := &cobra.Command{
		Use:   "restart [<app>]",
		Short: "Start the live version again in a new container, without building",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				s.appFlag = args[0]
			}
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			if err := client.RestartApp(s.ctx, org, app); err != nil {
				return wrap(err)
			}
			recent, err := client.ListDeploys(s.ctx, org, app, 5)
			if err != nil {
				return wrap(err)
			}
			d, ok := restartDeploy(recent)
			if !ok {
				return output.New("PLATFORM_UNAVAILABLE", "Whisk restarted the app but did not list the restart's deploy.", "Follow it with whisk deploys.", nil)
			}
			s.printer.Progress("Restarting %s/%s (deploy %s).", org, app, d.ID)
			if noWait {
				s.printer.Result(map[string]any{"deploy_id": d.ID, "status": d.Status, "commit_sha": d.CommitSHA, "environment": d.Environment}, func(w io.Writer) {
					fmt.Fprintf(w, "Restart queued as deploy %s. Follow it with: whisk deploys info %s\n", d.ID, d.ID)
				})
				return nil
			}
			return s.waitForDeploy(client, org, app, d)
		},
	}
	c.Flags().BoolVar(&noWait, "no-wait", false, "print the deploy id and exit")
	return c
}

// restartDeploy is the deploy a restart queued: the newest production deploy, the deploys
// listed newest first. Pure.
func restartDeploy(recent []api.Deploy) (api.Deploy, bool) {
	for _, d := range recent {
		if d.Environment == "" || d.Environment == "production" {
			return d, true
		}
	}
	return api.Deploy{}, false
}
