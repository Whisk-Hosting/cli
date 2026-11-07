package whisk

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// crawlersCmd is what robots asked the app for (CLI.md §5): each crawler, the pages, the answers
// and the latest journeys, from Whisk's request log and the CDN's.
func crawlersCmd(s *session) *cobra.Command {
	var period, crawler string
	c := &cobra.Command{
		Use:   "crawlers",
		Short: "What crawlers asked the app for: each crawler, its pages, the answers and its journeys",
		Long: `Prints the crawlers that asked for the app's pages over a period (--period 24h, 7d, 30d or 90d):
each crawler's requests, pages, journeys, errors (an answer of 400 or more) and requests the CDN
answered from its copy; then the pages asked for most, the answers the app gave, and the latest
journeys. A journey is one crawler's requests from one network with no long pause; open one with
whisk analytics journey <id>. --crawler narrows the pages, answers and journeys to one crawler.
Single requests are kept as long as the plan keeps analytics, at most 90 days.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			a, err := client.Crawlers(s.ctx, org, app, period, crawler)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "crawlers": a}, func(w io.Writer) { printAppCrawlers(s, w, a) })
			return nil
		},
	}
	c.Flags().StringVar(&period, "period", "7d", "the period: 24h, 7d, 30d or 90d")
	c.Flags().StringVar(&crawler, "crawler", "", "one crawler's pages, answers and journeys, by its name: Googlebot, GPTBot…")
	return c
}

// printAppCrawlers is the crawlers' tables.
func printAppCrawlers(s *session, w io.Writer, a api.AppCrawlers) {
	if len(a.Crawlers) == 0 {
		fmt.Fprintln(w, "No crawlers in that period.")
		return
	}
	rows := make([][]string, len(a.Crawlers))
	for i, c := range a.Crawlers {
		rows[i] = []string{c.Name, crawlerKind(c.Kind), strconv.Itoa(c.Requests), strconv.Itoa(c.Pages), strconv.Itoa(c.Journeys),
			strconv.Itoa(c.Errors), strconv.Itoa(c.Cached), at(c.Last)}
	}
	s.printer.Table(w, []string{"CRAWLER", "KIND", "REQUESTS", "PAGES", "JOURNEYS", "ERRORS", "FROM CDN", "LAST"}, rows)
	if a.Crawler != "" {
		fmt.Fprintf(w, "\nPages, answers and journeys of %s:\n", a.Crawler)
	}
	if len(a.Pages) > 0 {
		fmt.Fprintln(w)
		rows = make([][]string, len(a.Pages))
		for i, p := range a.Pages {
			rows[i] = []string{p.Path, strconv.Itoa(p.Requests), strconv.Itoa(p.Crawlers), at(p.LastAt), strconv.Itoa(p.LastStatus)}
		}
		s.printer.Table(w, []string{"PAGE", "REQUESTS", "CRAWLERS", "LAST", "LAST ANSWER"}, rows)
	}
	if len(a.Statuses) > 0 {
		fmt.Fprintln(w)
		rows = make([][]string, len(a.Statuses))
		for i, st := range a.Statuses {
			rows[i] = []string{strconv.Itoa(st.Status), strconv.Itoa(st.Requests)}
		}
		s.printer.Table(w, []string{"ANSWER", "REQUESTS"}, rows)
	}
	if len(a.Journeys) > 0 {
		fmt.Fprintln(w)
		rows = make([][]string, len(a.Journeys))
		for i, j := range a.Journeys {
			rows[i] = []string{j.ID, j.Name, j.Network, at(j.Start), span(j.End.Sub(j.Start)), strconv.Itoa(j.Requests),
				strconv.Itoa(j.Pages), j.FirstPath + " → " + j.LastPath}
		}
		s.printer.Table(w, []string{"JOURNEY", "CRAWLER", "NETWORK", "STARTED", "LASTED", "REQUESTS", "PAGES", "FIRST → LAST"}, rows)
	}
}

// journeyCmd is one crawler's journey, request by request (CLI.md §5).
func journeyCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "journey <id>",
		Short: "One crawler's journey through the app, request by request",
		Long: `Prints one crawler's journey, its id from whisk analytics crawlers: every request in order, with
the time, the page, the answer, its size, how long the app took and whether the CDN answered from
its copy.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			d, err := client.CrawlerJourney(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "journey": d}, func(w io.Writer) {
				j := d.Journey
				fmt.Fprintf(w, "%s (%s) from %s: %d requests to %d pages over %s, from %s\n\n", j.Name, crawlerKind(j.Kind), j.Network,
					j.Requests, j.Pages, span(j.End.Sub(j.Start)), at(j.Start))
				rows := make([][]string, len(d.Requests))
				for i, r := range d.Requests {
					from := "app"
					if r.Cached {
						from = "CDN"
					}
					rows[i] = []string{r.At.Local().Format("15:04:05"), r.Method, r.Path, strconv.Itoa(r.Status), strconv.FormatInt(r.Bytes, 10),
						strconv.Itoa(r.Millis), from}
				}
				s.printer.Table(w, []string{"TIME", "METHOD", "PAGE", "ANSWER", "BYTES", "MS", "ANSWERED BY"}, rows)
				if d.More {
					fmt.Fprintf(w, "\nThe first %d requests are listed; the journey has more.\n", len(d.Requests))
				}
			})
			return nil
		},
	}
}

// crawlerKind is a crawler kind as the tables show it. Pure.
func crawlerKind(kind string) string {
	if k := crawlerKinds[kind]; k != "" {
		return k
	}
	return kind
}

// span is how long a journey lasted: 0s, 42s, 12m, 2h 5m. Pure.
func span(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(max(d, 0)/time.Second)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d%time.Hour < time.Minute:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	default:
		return fmt.Sprintf("%dh %dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
	}
}
