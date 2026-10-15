package whisk

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/dev"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/stub"
)

func devCmd(s *session) *cobra.Command {
	var as, name, audience, groups, roles string
	var port int
	var down, wipe, status bool
	c := &cobra.Command{
		Use:   "dev [flags] [-- <app command>]",
		Short: "Run the app locally the way the platform would",
		Long: `Starts Postgres, PgBouncer and the Inngest dev server (plus Valkey and object storage when the
manifest declares kv or storage) with Docker Compose in .whisk/dev/, then runs the contract's stub:
an edge on localhost:<port> that injects the identity headers for --as, honours the manifest's
public routes, and serves the queue, approvals and webhook ingress locally. The app command after
-- is run with the production environment (WHISK_DEV=1 added); without one, the app is expected
on port 8080 and the environment is printed. Secrets come from .whisk/dev/secrets.env. Needs Docker
with the compose plugin.

  whisk dev --as ana@acme.example --roles owner -- npm run dev        (TypeScript template)
  uv run whisk dev --as ana@acme.example --roles owner -- uvicorn app.main:app --port 8080   (Python)
  whisk dev --as ana@acme.example --roles owner -- go run .           (Go)`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(s.env.Dir)
			if err != nil {
				return err
			}
			log := s.env.Stderr
			if down {
				return dev.Down(s.ctx, s.env.Dir, m, wipe, log)
			}
			if err := dev.CheckDocker(s.ctx); err != nil {
				return output.New("LOCAL_DOCKER_UNAVAILABLE", err.Error(), "Install Docker Desktop or Docker Engine with the compose plugin and start it, then run whisk dev again; or run the app under whisk-stub with your own Postgres. Retrying without Docker will not help.", nil)
			}
			o := dev.Options{AppDir: s.env.Dir, Manifest: m, Port: port, Log: log}
			o.Identity.As, o.Identity.Name, o.Identity.Audience = as, name, audience
			o.Identity.Groups, o.Identity.Roles = stub.SplitList(groups), stub.SplitList(roles)
			st, err := dev.Up(s.ctx, o, log)
			if err != nil {
				return output.New("DEV_STACK_FAILED", err.Error(), "Check that Docker is running and the ports in .whisk/dev/compose.yaml are free, then run whisk dev again; whisk dev --down resets the stack.", nil)
			}
			if status {
				s.printer.Result(st.Status(), func(w io.Writer) { fmt.Fprintln(w, dev.JSON(st.Status())) })
				return nil
			}
			fmt.Fprintf(log, "dev: postgres %s\n", st.Plan.DirectURL)
			fmt.Fprintf(log, "dev: DATABASE_URL %s (through pgbouncer, transaction mode)\n", st.Plan.DatabaseURL)
			fmt.Fprintf(log, "dev: inngest ui %s\n", st.Plan.InngestURL)
			if len(args) == 0 {
				fmt.Fprintf(log, "dev: no app command after --; start the app yourself on port 8080 with the environment above, or run it under whisk dev: whisk dev --as %s -- <the app's start command, such as npm run dev>\n", orDash(as))
			}
			ctx, stop := signal.NotifyContext(s.ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := dev.RunStub(ctx, o, st, args); err != nil {
				switch {
				case strings.HasPrefix(err.Error(), "MIGRATE_FAILED"):
					return output.New("MIGRATE_FAILED", err.Error(), "Fix the migration and run whisk dev again; the database keeps its state.", nil)
				case strings.Contains(err.Error(), "address already in use"):
					return output.New("INVALID_REQUEST", err.Error(), fmt.Sprintf("Pass --port with a free port; the api and internal listeners use the next two (%d and %d are in use or taken).", port+1, port+2), map[string]any{"port": port})
				}
				return err
			}
			fmt.Fprintln(log, "dev: stopped; containers keep running for a fast restart (whisk dev --down stops them)")
			return nil
		},
	}
	f := c.Flags()
	f.StringVar(&as, "as", "", "sign in as this email; absent means every request is anonymous")
	f.StringVar(&name, "name", "", "display name for --as")
	f.StringVar(&audience, "audience", "team", "team or customer")
	f.StringVar(&groups, "groups", "", "comma-separated groups for --as")
	f.StringVar(&roles, "roles", "owner", "comma-separated org roles for --as")
	f.IntVar(&port, "port", 3000, "edge port; the api is port+1 and the internal listener port+2")
	f.BoolVar(&down, "down", false, "stop the local stack")
	f.BoolVar(&wipe, "wipe", false, "with --down: also delete the database and storage volumes")
	f.BoolVar(&status, "status", false, "start the stack, print its addresses and exit")
	c.AddCommand(tunnelCmd(s))
	return c
}
