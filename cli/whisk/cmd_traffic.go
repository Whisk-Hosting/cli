package whisk

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// trafficFilters are the flags that narrow a reading of the traffic, each to one value, and the
// query value each sends.
var trafficFilters = []struct{ flag, param, usage string }{
	{"path", "path", "only requests for this page, such as /pricing"},
	{"query", "query", "only requests with exactly this query, without its ?, such as page=2"},
	{"method", "method", "only requests with this method, such as POST"},
	{"host", "host", "only requests to this hostname"},
	{"agent", "agent", "only requests from this agent, such as Googlebot, Chrome or curl"},
	{"kind", "kind", "only requests from this kind of agent: search, ai_assistant, ai_crawler, preview, seo, other, browser, tool or unknown"},
	{"user-agent", "user_agent", "only requests with exactly this user agent"},
	{"referer", "referer", "only requests with this referer, such as https://www.google.com/"},
	{"user-id", "user_id", "only requests from this signed-in person, by the user id the app knows them by"},
	{"address", "address", "only requests from this client address"},
	{"country", "country", "only requests from this country, two letters, such as NZ"},
	{"network", "network", "only requests from this network, such as GOOGLE"},
	{"source", "source", "edge for what Whisk's edge answered, cdn for what the CDN answered from its copy"},
	{"edge-cache", "edge_cache", "hit for what the edge's own cache answered, miss for what it was asked and passed on"},
}

// trafficCmd is every request to the app, one by one or grouped (CLI.md §5).
func trafficCmd(s *session) *cobra.Command {
	var q api.TrafficQuery
	var list bool
	filters := make([]string, len(trafficFilters))
	c := &cobra.Command{
		Use:   "traffic",
		Short: "Every request to the app: pages, answers, the CDN's copy, who asked and from where",
		Long: `Prints every request Whisk's edge or the app's CDN answered for the app over a period (--period
24h, 7d, 30d or 90d, or --from and --to): people, crawlers and scripts alike, including what the
CDN answered from its copy, which never reaches the app. whisk analytics counts only the visits
whose browser ran the analytics script.

Each request is kept whole: its time, method, host, page and query (a token, key, code, password
or signature in it reads REDACTED), answer, bytes sent and received, how long the edge took in all,
how long the app took and how long a wake took, whether the CDN or the edge's own cache answered,
the user agent as sent and who it names, the referer, the signed-in person's user id, the
client's address, and the country and network that address is in.

--by groups the counts, in the order given, by any of:
  day, hour   the day or hour, UTC
  path        the page
  query       the query, secrets masked
  status      the answer, 200, 404…
  cached      whether the CDN answered from its copy
  source      edge or cdn
  method      GET, POST…
  host        the hostname asked for
  agent       a crawler's name (Googlebot, GPTBot…), a browser's (Chrome, Safari…), a tool's
              (curl, Python…), "Other bots", "Other" or "(none)"
  kind        the crawler's kind (search, ai_assistant, ai_crawler, preview, seo, other),
              browser, tool or unknown
  user_agent  the user agent as sent
  referer     the referer
  user_id     the signed-in person's user id, "-" for nobody
  address     the client's address
  country     the client's country, two letters
  network     the client's network
  edge_cache  hit or miss when the edge's own cache was asked
Each group counts its requests and the distinct addresses that made them.
--list prints the latest requests one by one instead; --json has every field of each.
The flags below narrow either. Requests are kept as long as the plan keeps analytics, at most 90
days, and the newest come in a few minutes late.`,
		Example: `  whisk analytics traffic --by day,kind
  whisk analytics traffic --by path,agent --status 404 --period 30d
  whisk analytics traffic --by country,network --kind unknown
  whisk analytics traffic --list --path /pricing --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			q.Filters = map[string]string{}
			for i, f := range trafficFilters {
				if filters[i] != "" {
					q.Filters[f.param] = filters[i]
				}
			}
			if list {
				t, err := client.TrafficRequests(s.ctx, org, app, q)
				if err != nil {
					return wrap(err)
				}
				if t.Requests == nil {
					t.Requests = []api.TrafficRequest{}
				}
				s.printer.Result(map[string]any{"org": org, "app": app, "traffic": t}, func(w io.Writer) { printTrafficRequests(s, w, t) })
				return nil
			}
			t, err := client.Traffic(s.ctx, org, app, q)
			if err != nil {
				return wrap(err)
			}
			if t.Rows == nil {
				t.Rows = []api.TrafficRow{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "traffic": t}, func(w io.Writer) { printTraffic(s, w, t) })
			return nil
		},
	}
	readingFlags(c, &q.Reading)
	c.Flags().StringVar(&q.By, "by", "day", "group by any of day, hour, path, query, status, cached, edge_cache, source, method, host, agent, kind, user_agent, referer, user_id, address, country and network, as a comma list in order")
	c.Flags().BoolVar(&list, "list", false, "list the latest requests one by one instead of counting them")
	for i, f := range trafficFilters {
		c.Flags().StringVar(&filters[i], f.flag, "", f.usage)
	}
	c.Flags().IntVar(&q.Status, "status", 0, "only requests answered with this status, such as 404")
	c.Flags().StringVar(&q.Cached, "cached", "", "true for only what the CDN answered from its copy, false for the rest")
	c.Flags().IntVar(&q.Limit, "limit", 0, "how many rows or requests to list (default 100, at most 1000)")
	return c
}

// printTraffic is a reading of the traffic as a table, then its total.
func printTraffic(s *session, w io.Writer, t api.AppTraffic) {
	if t.Requests == 0 {
		fmt.Fprintln(w, "No requests in that period match.")
		return
	}
	if len(t.By) > 0 {
		head, rows := trafficTable(t.By, t.Rows)
		s.printer.Table(w, head, rows)
	}
	fmt.Fprintf(w, "%d requests in all, from %d addresses.\n", t.Requests, t.Addresses)
}

// trafficTable is a reading's header and rows, a column for each group, then the requests and
// addresses. Pure.
func trafficTable(by []string, rows []api.TrafficRow) ([]string, [][]string) {
	head := make([]string, 0, len(by)+2)
	for _, b := range by {
		head = append(head, strings.ToUpper(strings.ReplaceAll(b, "_", " ")))
	}
	head = append(head, "REQUESTS", "ADDRESSES")
	out := make([][]string, len(rows))
	for i, r := range rows {
		row := make([]string, 0, len(head))
		for _, b := range by {
			row = append(row, trafficValue(b, r.Group[b]))
		}
		out[i] = append(row, strconv.Itoa(r.Requests), strconv.Itoa(r.Addresses))
	}
	return head, out
}

// trafficValue is a group's value as a person reads it. Pure.
func trafficValue(by, v string) string {
	switch {
	case by == "cached" && v == "true":
		return "CDN copy"
	case by == "cached":
		return "app"
	case by == "kind":
		return crawlerKind(v)
	case v == "":
		return "-"
	}
	return v
}

// printTrafficRequests is the latest requests as a table, latest first.
func printTrafficRequests(s *session, w io.Writer, t api.AppTrafficRequests) {
	if len(t.Requests) == 0 {
		fmt.Fprintln(w, "No requests in that period match.")
		return
	}
	s.printer.Table(w, []string{"AT (UTC)", "METHOD", "PATH", "STATUS", "MS", "FROM", "AGENT", "ADDRESS", "COUNTRY", "NETWORK"}, requestRows(t.Requests))
}

// requestRows are requests as table rows. Pure.
func requestRows(reqs []api.TrafficRequest) [][]string {
	out := make([][]string, len(reqs))
	for i, r := range reqs {
		from := "app"
		switch {
		case r.Cached:
			from = "CDN copy"
		case r.EdgeCache == "hit":
			from = "edge cache"
		case r.WakeMillis != nil:
			from = "app, woken"
		}
		page := r.Path
		if r.Query != "" {
			page += "?" + r.Query
		}
		out[i] = []string{r.At.UTC().Format("2006-01-02 15:04:05"), r.Method, page, strconv.Itoa(r.Status), strconv.Itoa(r.Millis), from,
			r.Agent, r.Address, trafficValue("country", r.Country), trafficValue("network", r.Network)}
	}
	return out
}
