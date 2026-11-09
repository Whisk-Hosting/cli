package whisk

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// readingFlags are the flags that choose a reading of the analytics: the period, or from and to,
// and the zone days start in.
func readingFlags(c *cobra.Command, q *api.AnalyticsQuery) {
	c.Flags().StringVar(&q.Period, "period", "", "the period: 24h, 7d, 30d or 90d (default 7d)")
	c.Flags().StringVar(&q.From, "from", "", "start at a date, 2026-11-01, or a time, 2026-11-01T09:00:00Z, in place of --period")
	c.Flags().StringVar(&q.To, "to", "", "end with a date (its whole day) or a time; default now")
	c.Flags().StringVar(&q.TZ, "tz", "", "the time zone dates and days are in, such as America/New_York (default UTC)")
}

// filterFlags are the flags that narrow a reading to some of its visits.
func filterFlags(c *cobra.Command, q *api.AnalyticsQuery) {
	c.Flags().StringVar(&q.Source, "source", "", "only visits from this site, such as google.com (a comma means any of)")
	c.Flags().StringVar(&q.Page, "page", "", "only visits that viewed this page, such as /pricing")
	c.Flags().StringVar(&q.Event, "event", "", "only visits that sent this event, such as signup")
}

// analyticsCmd is the app's visits (CLI.md §5): the numbers over a period and the top pages,
// sources and events, from the pages that load /.whisk/analytics.js.
func analyticsCmd(s *session) *cobra.Command {
	var q api.AnalyticsQuery
	c := &cobra.Command{
		Use:   "analytics",
		Short: "The app's visits: views, visitors, top pages, sources, events and crawlers",
		Long: `Prints the app's visits over a period (--period 24h, 7d, 30d or 90d, or --from and --to): views,
visitors, visits, the bounce rate and the average visit, then the top pages, where visitors came
from and the events the app sent. A page is counted when it loads
<script defer src="/.whisk/analytics.js">. Last come the crawlers that asked for the app's pages
(search engines, AI assistants and crawlers, link previews and other bots), counted from Whisk's
request log; whisk analytics crawlers shows each one's pages and journeys.

--source, --page and --event narrow every number to the visits that came from a site, viewed a
page or sent an event. --by breaks the reading down instead of listing the top pages:
  hour, day, month   views and visitors for each, every one in the period (--tz sets the zone)
  page               views of each page
  entry, exit        visits that started or ended on each page
  source             visits from each site
  event              times each event was sent
  event-property     each property and value --event's events carried, such as target=https://…
  country, browser, os, device
--limit sets how many rows a breakdown by name lists (default 10, at most 500).

whisk analytics visitors lists the visitors themselves, and whisk analytics visitor <id> what one
of them did. whisk open analytics opens the full analytics in the dashboard.`,
		Example: `  whisk analytics --source google.com --by entry
  whisk analytics --from 2026-11-01 --to 2026-11-07 --by day --tz America/New_York
  whisk analytics --event outbound-click --by event-property --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			a, err := client.Analytics(s.ctx, org, app, q)
			if err != nil {
				return wrap(err)
			}
			for _, l := range []*[]api.AnalyticsCount{&a.Pages, &a.Sources, &a.Events} {
				if *l == nil {
					*l = []api.AnalyticsCount{}
				}
			}
			if a.Crawlers == nil {
				a.Crawlers = []api.AnalyticsCrawler{}
			}
			if a.By != "" && a.Breakdown == nil {
				a.Breakdown = []api.AnalyticsRow{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "analytics": a}, func(w io.Writer) { printAnalytics(s, w, a) })
			return nil
		},
	}
	readingFlags(c, &q)
	filterFlags(c, &q)
	c.Flags().StringVar(&q.By, "by", "", "break the reading down by hour, day, month, page, entry, exit, source, event, event-property, country, browser, os or device")
	c.Flags().IntVar(&q.Limit, "limit", 0, "how many rows a breakdown by name lists (default 10, at most 500)")
	c.AddCommand(crawlersCmd(s), journeyCmd(s), visitorsCmd(s), visitorCmd(s), trafficCmd(s))
	return c
}

// printAnalytics is a reading's tables: the numbers, then a breakdown or the top pages, sources
// and events and the crawlers.
func printAnalytics(s *session, w io.Writer, a api.AppAnalytics) {
	if a.By == "" {
		defer printCrawlers(s, w, a.Crawlers)
	}
	if a.Views == 0 && len(a.Breakdown) == 0 {
		fmt.Fprintln(w, noVisits(a.Filters))
		return
	}
	s.printer.Table(w, []string{"VIEWS", "VISITORS", "VISITS", "BOUNCE RATE", "AVERAGE VISIT"}, [][]string{{
		strconv.Itoa(a.Views), strconv.Itoa(a.Visitors), strconv.Itoa(a.Visits),
		strconv.Itoa(int(math.Round(a.BounceRate*100))) + "%", visitLength(a.AverageSeconds),
	}})
	if a.By != "" {
		fmt.Fprintln(w)
		if len(a.Breakdown) == 0 {
			fmt.Fprintf(w, "Analytics recorded no %s for these visits.\n", strings.ReplaceAll(a.By, "-", " "))
			return
		}
		head, rows := breakdownTable(a.By, a.Breakdown)
		s.printer.Table(w, head, rows)
		return
	}
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
			if name == "" && part.head == "SOURCE" {
				name = unreferred
			} else if name == "" {
				name = "(none)"
			}
			rows[i] = []string{name, strconv.Itoa(r.Count)}
		}
		s.printer.Table(w, []string{part.head, "COUNT"}, rows)
	}
}

// unreferred names the source row of the visitors no other site sent: typed in, bookmarked, or,
// under --page, reaching that page from the app's own pages.
const unreferred = "(direct or own pages)"

// noVisits is what an empty reading says: why it may be empty. Pure.
func noVisits(f api.AnalyticsFilters) string {
	if f.Source != "" || f.Page != "" || f.Event != "" {
		return "No visits in that period match the filters."
	}
	return "No visits in that period. Pages are counted once they load <script defer src=\"/.whisk/analytics.js\"></script>."
}

// breakdownTable is a breakdown's header and rows: views and visitors for a breakdown by time, a
// count for the rest. Pure.
func breakdownTable(by string, rows []api.AnalyticsRow) ([]string, [][]string) {
	out := make([][]string, len(rows))
	if by == "hour" || by == "day" || by == "month" {
		for i, r := range rows {
			out[i] = []string{r.Name, strconv.Itoa(r.Views), strconv.Itoa(r.Visitors)}
		}
		return []string{strings.ToUpper(by), "VIEWS", "VISITORS"}, out
	}
	for i, r := range rows {
		name := r.Name
		if name == "" {
			name = "(none)"
			if by == "source" {
				name = unreferred
			}
		}
		out[i] = []string{name, strconv.Itoa(r.Count)}
	}
	return []string{strings.ToUpper(strings.ReplaceAll(by, "-", " ")), "COUNT"}, out
}

// visitorsCmd is the app's visitors one by one (CLI.md §5).
func visitorsCmd(s *session) *cobra.Command {
	var q api.AnalyticsQuery
	var cursor string
	c := &cobra.Command{
		Use:   "visitors",
		Short: "The app's visitors one by one, latest first",
		Long: `Lists the app's visitors over a period, latest first, 50 at a time: when each was first and last
seen, their visits, page views and events, and where and on what they visited. A visitor is one
browser on one device, told apart without cookies. --source, --page and --event narrow the list
as they do for whisk analytics. whisk analytics visitor <id> shows what one of them did.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			v, err := client.Visitors(s.ctx, org, app, q, cursor)
			if err != nil {
				return wrap(err)
			}
			if v.Visitors == nil {
				v.Visitors = []api.AnalyticsVisitor{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "visitors": v}, func(w io.Writer) {
				if len(v.Visitors) == 0 {
					fmt.Fprintln(w, noVisits(v.Filters))
					return
				}
				rows := make([][]string, len(v.Visitors))
				for i, x := range v.Visitors {
					rows[i] = []string{x.ID, at(x.LastAt), strconv.Itoa(x.Visits), strconv.Itoa(x.Views), strconv.Itoa(x.Events),
						orDash(place(x.Country, x.City)), orDash(strings.TrimSpace(x.Browser + " " + x.OS)), orDash(x.Device)}
				}
				s.printer.Table(w, []string{"VISITOR", "LAST SEEN", "VISITS", "VIEWS", "EVENTS", "WHERE", "BROWSER", "DEVICE"}, rows)
				fmt.Fprintf(w, "%d visitors in all.", v.Total)
				if v.NextCursor != "" {
					fmt.Fprintf(w, " More with --cursor %s.", v.NextCursor)
				}
				fmt.Fprintln(w)
			})
			return nil
		},
	}
	readingFlags(c, &q)
	filterFlags(c, &q)
	c.Flags().StringVar(&cursor, "cursor", "", "the next page, as the last one named it")
	return c
}

// place is a visitor's country and city as one. Pure.
func place(country, city string) string {
	if city == "" {
		return country
	}
	if country == "" {
		return city
	}
	return city + ", " + country
}

// visitorCmd is what one visitor did (CLI.md §5).
func visitorCmd(s *session) *cobra.Command {
	var q api.AnalyticsQuery
	c := &cobra.Command{
		Use:   "visitor <id>",
		Short: "What one visitor did, page by page and event by event",
		Long: `Prints what one visitor did over a period, earliest first: each page they viewed and each event
the app sent, with the time, the site the visit came from and which visit it was. The id comes
from whisk analytics visitors. At most the latest 500 steps are listed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			d, err := client.VisitorSteps(s.ctx, org, app, args[0], q)
			if err != nil {
				return wrap(err)
			}
			if d.Steps == nil {
				d.Steps = []api.AnalyticsStep{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "visitor": d}, func(w io.Writer) {
				if len(d.Steps) == 0 {
					fmt.Fprintln(w, "This visitor did nothing in that period. Try a longer --period or --from.")
					return
				}
				s.printer.Table(w, []string{"TIME", "VISIT", "DID", "PAGE", "FROM"}, stepRows(d.Steps))
			})
			return nil
		},
	}
	readingFlags(c, &q)
	return c
}

// stepRows are a visitor's steps as a table, each visit numbered from 1 in order. Pure.
func stepRows(steps []api.AnalyticsStep) [][]string {
	visits := map[string]int{}
	rows := make([][]string, len(steps))
	for i, st := range steps {
		if _, ok := visits[st.Visit]; !ok {
			visits[st.Visit] = len(visits) + 1
		}
		did := "viewed"
		if st.Kind == "event" {
			did = "sent " + st.Event
		}
		page := st.Path
		if st.Query != "" {
			page += "?" + strings.TrimPrefix(st.Query, "?")
		}
		rows[i] = []string{st.At.Local().Format("2006-01-02 15:04:05"), strconv.Itoa(visits[st.Visit]), did, page, orDash(st.Source)}
	}
	return rows
}

// crawlerKinds are the API's crawler kinds as the table shows them.
var crawlerKinds = map[string]string{
	"search": "search engine", "ai_assistant": "AI assistant", "ai_crawler": "AI crawler",
	"preview": "link preview", "seo": "SEO tool", "other": "other",
}

// printCrawlers is the robots that asked for the app's pages, which no visit count includes.
func printCrawlers(s *session, w io.Writer, crawlers []api.AnalyticsCrawler) {
	if len(crawlers) == 0 {
		return
	}
	fmt.Fprintln(w)
	rows := make([][]string, len(crawlers))
	for i, c := range crawlers {
		rows[i] = []string{c.Name, crawlerKind(c.Kind), strconv.Itoa(c.Requests)}
	}
	s.printer.Table(w, []string{"CRAWLER", "KIND", "REQUESTS"}, rows)
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
