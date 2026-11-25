package whisk

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/dbtunnel"
	"github.com/whisk-run/cli/internal/output"
)

// dbConnectCmd opens a read-only tunnel to an environment's database (CLI.md §5.9): a login of
// its own that writes nothing, for an hour, reached on 127.0.0.1 by any Postgres tool.
func dbConnectCmd(s *session) *cobra.Command {
	var environment string
	var port int
	c := &cobra.Command{
		Use:   "connect [--env production|preview:<branch>] [--port n]",
		Short: "Open a read-only tunnel to the database for psql or a GUI, for an hour; audited",
		Long: `Makes a read-only login of the environment's database, its own, and listens on 127.0.0.1 for
an hour: point psql, TablePlus or any Postgres tool at the string it prints. The tunnel carries
each connection to the platform over TLS. The login reads what the app reads and writes nothing,
and it and its sessions end when the hour does or when you press Ctrl-C. A database the app
shares with the business's other apps is read with whisk db query instead.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			u, err := client.DatabaseReader(s.ctx, org, app, environment)
			if err != nil {
				return wrap(err)
			}
			if api.PrivateURL(u) {
				return output.New("DB_URL_PRIVATE", "The "+u.Environment+" database's address is inside the platform, so no tunnel reaches it from this machine.",
					"Use whisk db query \"<sql>\" for rows as JSON or whisk db schema for the tables; both run on the platform.",
					map[string]any{"environment": u.Environment})
			}
			proxy, name, err := dbtunnel.Remote(u.URL)
			if err != nil {
				return err
			}
			lis, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				return output.New("INVALID_REQUEST", "Port "+strconv.Itoa(port)+" on 127.0.0.1 is taken.",
					"Pass --port with a free port, or --port 0 to take any.", map[string]any{"port": port})
			}
			local, err := dbtunnel.Local(u.URL, lis.Addr().String())
			if err != nil {
				_ = lis.Close()
				return err
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "environment": u.Environment, "url": local, "read_only": true, "expires_at": u.ExpiresAt.UTC().Format(time.RFC3339)}, func(w io.Writer) {
				fmt.Fprintln(w, local)
			})
			log := s.env.Stderr
			fmt.Fprintf(log, "db connect: read-only on %s until %s; Ctrl-C closes it. Every connection is audited.\n", u.Environment, at(u.ExpiresAt))
			ctx, stop := signal.NotifyContext(s.ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			n, err := dbtunnel.Serve(ctx, dbtunnel.Options{Listener: lis, Proxy: proxy, Name: name, Until: u.ExpiresAt,
				Opened: func(string) { fmt.Fprintln(log, "db connect: a connection opened") }})
			fmt.Fprintf(log, "db connect: closed after %d connections\n", n)
			return err
		},
	}
	c.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	c.Flags().IntVar(&port, "port", 5433, "the port on 127.0.0.1 to listen on; 0 takes any")
	return c
}
