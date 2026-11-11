package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// cdnPurgeText says what a purge dropped, in plain words. Pure.
func cdnPurgeText(app string, p api.CDNPurge) string {
	switch {
	case !p.On:
		return app + " is not served through the CDN, so nothing was cached there to drop."
	case p.All:
		return "Dropped everything the CDN held for " + app + ". The next visit to each page fetches it from the app."
	}
	out := fmt.Sprintf("Dropped %d address(es) from the CDN for %s:\n", len(p.Urls), app)
	for _, u := range p.Urls {
		out += "  " + u + "\n"
	}
	return out
}

func cdnPurgeCmd(s *session) *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "purge <path>... | --all",
		Short: "Drop pages from the CDN now, before their max-age runs out",
		Long: `Drops each path from the CDN on every name the CDN serves, so the next visit fetches it from
the app. End a path in * to drop everything under it (/entries/*), and give --all to drop
everything. At most 20 paths at a time.

An app can purge on its own when its data changes, with its WHISK_SERVICE_TOKEN: POST
/v1/orgs/<org>/apps/<app>/cdn/purge {"paths": ["/entries/42"]}. That lets a public page carry a
long max-age and still change straight away. An app the CDN does not serve answers "on": false.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return output.New("INVALID_REQUEST", "Give the paths to drop, or --all, and not both.", "Run whisk cdn purge /entries/42 /entries/*, or whisk cdn purge --all.", nil)
			}
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			p, err := client.PurgeCDN(s.ctx, org, app, args)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "purge": p}, func(w io.Writer) { fmt.Fprintln(w, strings.TrimRight(cdnPurgeText(app, p), "\n")) })
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "drop everything the CDN holds for the app")
	return c
}
