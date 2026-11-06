package whisk

import (
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// analyticsCmd is the app's visits (CLI.md §5): the numbers over a period and the top pages,
// sources and events, from the pages that load /.whisk/analytics.js.
func analyticsCmd(s *session) *cobra.Command {
	var period string
	c := &cobra.Command{
		Use:   "analytics",
		Short: "The app's visits: views, visitors, top pages, sources and events",
		Long: `Prints the app's visits over a period (--period 24h, 7d, 30d or 90d): views, visitors, visits,
the bounce rate and the average visit, then the top pages, where visitors came from and the
events the app sent. A page is counted when it loads <script defer src="/.whisk/analytics.js">.
whisk open analytics opens the full analytics in the dashboard.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			a, err := client.Analytics(s.ctx, org, app, period)
			if err != nil {
				return wrap(err)
			}
			for _, l := range []*[]api.AnalyticsCount{&a.Pages, &a.Sources, &a.Events} {
				if *l == nil {
					*l = []api.AnalyticsCount{}
				}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "analytics": a}, func(w io.Writer) {
				if a.Views == 0 {
					fmt.Fprintln(w, "No visits in that period. Pages are counted once they load <script defer src=\"/.whisk/analytics.js\"></script>.")
					return
				}
				s.printer.Table(w, []string{"VIEWS", "VISITORS", "VISITS", "BOUNCE RATE", "AVERAGE VISIT"}, [][]string{{
					strconv.Itoa(a.Views), strconv.Itoa(a.Visitors), strconv.Itoa(a.Visits),
					strconv.Itoa(int(math.Round(a.BounceRate*100))) + "%", visitLength(a.AverageSeconds),
				}})
				for _, part := range []struct {
					head string
					rows []api.AnalyticsCount
				}{{"PAGE", a.Pages}, {"SOURCE", a.Sources}, {"EVENT", a.Events}} {
					if len(part.rows) == 0 {
						continue
					}
					fmt.Fprintln(w)
					rows := make([][]string, len(part.rows))
					for i, r := range part.rows {
						name := r.Name
						if name == "" {
							name = "(direct)"
						}
						rows[i] = []string{name, strconv.Itoa(r.Count)}
					}
					s.printer.Table(w, []string{part.head, "COUNT"}, rows)
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&period, "period", "7d", "the period: 24h, 7d, 30d or 90d")
	return c
}

// visitLength is an average visit's length: 42s, 2m, 1m 35s. Pure.
func visitLength(seconds int) string {
	if seconds < 60 {
		return strconv.Itoa(max(seconds, 0)) + "s"
	}
	if seconds%60 == 0 {
		return strconv.Itoa(seconds/60) + "m"
	}
	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}
