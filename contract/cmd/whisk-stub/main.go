// whisk-stub runs an app the way the platform would, on one machine. It is the command-line
// wrapper around package stub; see that package and CONTRACT.md §13.
//
//	whisk-stub --as ana@acme.example --database-url postgres://... -- npm start
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/whisk-run/contract/stub"
)

func main() {
	o, describe, appCmd := parseFlags()
	if describe {
		env, err := stub.Env(o)
		if err != nil {
			fail(err)
		}
		for _, kv := range env {
			fmt.Println(kv)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := stub.Run(ctx, o, appCmd); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "stub: "+err.Error())
	os.Exit(1)
}

func parseFlags() (stub.Options, bool, []string) {
	d := stub.Defaults
	var o stub.Options
	var groups, roles string
	var describe bool
	flag.StringVar(&o.ManifestPath, "manifest", d.ManifestPath, "path to the manifest")
	flag.StringVar(&o.Listen, "listen", d.Listen, "edge listener: the app hostname")
	flag.StringVar(&o.API, "api", d.API, "api listener: queue, approvals, webhooks, inngest proxy")
	flag.StringVar(&o.Internal, "internal", d.Internal, "internal listener: platform deliveries with service identity")
	flag.StringVar(&o.Upstream, "upstream", d.Upstream, "where the app listens")
	flag.StringVar(&o.As, "as", "", "sign in as this email (team identity); absent means nobody can sign in")
	flag.StringVar(&o.Name, "name", "", "display name for --as")
	flag.StringVar(&groups, "groups", "", "comma-separated groups for --as")
	flag.StringVar(&roles, "roles", "owner", "comma-separated org roles for --as")
	flag.StringVar(&o.Audience, "audience", d.Audience, "team or customer")
	flag.StringVar(&o.OrgID, "org-id", "", "org ULID (generated when empty)")
	flag.StringVar(&o.AppID, "app-id", "", "app ULID (generated when empty)")
	flag.StringVar(&o.DatabaseURL, "database-url", os.Getenv("DATABASE_URL"), "DATABASE_URL for the app; required unless database: none")
	flag.StringVar(&o.SecretsFile, "secrets-file", d.SecretsFile, "NAME=value lines for declared secrets (git-ignored)")
	flag.StringVar(&o.InngestURL, "inngest", "", "URL of a running Inngest dev server; empty starts one")
	flag.StringVar(&o.InngestPort, "inngest-port", d.InngestPort, "port for the dev server the stub starts")
	flag.StringVar(&o.InngestCmd, "inngest-cmd", d.InngestCmd, "command that runs the Inngest CLI")
	flag.BoolVar(&o.NoInngest, "no-inngest", false, "run without a workflow engine")
	flag.BoolVar(&o.Migrate, "migrate", true, "run the manifest's migrate command before starting the app")
	flag.BoolVar(&describe, "describe", false, "print the environment the app would receive and exit")
	flag.Parse()
	o.Groups = stub.SplitList(groups)
	o.Roles = stub.SplitList(roles)
	return o, describe, flag.Args()
}
