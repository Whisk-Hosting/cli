package api

import (
	"context"
	"strconv"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// AppAnalytics is an app's visits over a period, from the pages that load the analytics script.
type AppAnalytics = apitypes.AppAnalytics

// AnalyticsCount is one page, source or event and how often it came.
type AnalyticsCount = apitypes.AnalyticsCount

// AnalyticsCrawler is one robot that asked for the app's pages and how often.
type AnalyticsCrawler = apitypes.AnalyticsCrawler

// AnalyticsFilters are what narrowed a reading.
type AnalyticsFilters = apitypes.AnalyticsFilters

// AnalyticsRow is one row of a breakdown.
type AnalyticsRow = apitypes.AnalyticsRow

// AppVisitors is a page of an app's visitors.
type AppVisitors = apitypes.AppVisitors

// AnalyticsVisitor is one visitor of an app.
type AnalyticsVisitor = apitypes.AnalyticsVisitor

// AnalyticsStep is one page a visitor viewed or one event sent for them.
type AnalyticsStep = apitypes.AnalyticsStep

// AnalyticsVisitorSteps is what one visitor did.
type AnalyticsVisitorSteps = apitypes.AnalyticsVisitorSteps

// AnalyticsQuery is one reading of an app's analytics: Period (24h, 7d, 30d or 90d) or From and
// To (a date or a time), TZ for where days start, the filters Source, Page and Event, and By to
// break it down. "" leaves any of them out.
type AnalyticsQuery struct {
	Period, From, To, TZ string
	Source, Page, Event  string
	By                   string
	Limit                int
}

// reading is q with the last 7 days when it names no period, from or to. Pure.
func (q AnalyticsQuery) reading() AnalyticsQuery {
	if q.Period == "" && q.From == "" && q.To == "" {
		q.Period = "7d"
	}
	return q
}

// declared is q as the declared traffic query; a filter the API does not take is left out. Pure.
func (q TrafficQuery) declared() endpoints.TrafficQuery {
	r := q.Reading.reading()
	out := endpoints.TrafficQuery{Period: r.Period, From: r.From, To: r.To, Tz: r.TZ, By: q.By, Cached: q.Cached,
		Status: q.Status, Limit: max(q.Limit, 0)}
	filters := map[string]*string{"address": &out.Address, "agent": &out.Agent, "country": &out.Country,
		"edge_cache": &out.EdgeCache, "host": &out.Host, "kind": &out.Kind, "method": &out.Method, "network": &out.Network,
		"path": &out.Path, "query": &out.Query, "referer": &out.Referer, "source": &out.Source,
		"user_agent": &out.UserAgent, "user_id": &out.UserId}
	for k, v := range q.Filters {
		if f := filters[k]; f != nil {
			*f = v
		}
	}
	return out
}

// Analytics reads an app's visits over a reading, broken down as it asks.
func (c *Client) Analytics(ctx context.Context, org, app string, q AnalyticsQuery) (AppAnalytics, error) {
	r := q.reading()
	return Send(ctx, c, endpoints.GetAnalytics, endpoints.GetAnalyticsQuery{Period: r.Period, From: r.From, To: r.To, Tz: r.TZ,
		Source: r.Source, Page: r.Page, Event: r.Event, By: r.By, Limit: max(r.Limit, 0)}, org, app)
}

// Visitors reads a page of an app's visitors over a reading, latest first, from the page cursor
// names ("" for the first).
func (c *Client) Visitors(ctx context.Context, org, app string, q AnalyticsQuery, cursor string) (AppVisitors, error) {
	r := q.reading()
	at := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil {
			return AppVisitors{}, &Error{Code: "INVALID_REQUEST", Message: "The visitors cursor " + strconv.Quote(cursor) + " is not one the API gave.", Fix: "Send the next_cursor of the page before, or none for the first."}
		}
		at = n
	}
	return Send(ctx, c, endpoints.GetAnalyticsVisitors, endpoints.GetAnalyticsVisitorsQuery{Period: r.Period, From: r.From, To: r.To, Tz: r.TZ,
		Source: r.Source, Page: r.Page, Event: r.Event, Limit: max(r.Limit, 0), Cursor: at}, org, app)
}

// VisitorSteps reads what one visitor did over a reading, earliest first.
func (c *Client) VisitorSteps(ctx context.Context, org, app, id string, q AnalyticsQuery) (AnalyticsVisitorSteps, error) {
	r := q.reading()
	return Send(ctx, c, endpoints.GetAnalyticsVisitor, endpoints.GetAnalyticsVisitorQuery{Period: r.Period, From: r.From, To: r.To, Tz: r.TZ}, org, app, id)
}

// AppCrawlers is what robots asked an app for over a period: each robot, the pages, the answers
// and the latest journeys.
type AppCrawlers = apitypes.AppCrawlers

// CrawlerSummary is one robot over a period.
type CrawlerSummary = apitypes.CrawlerSummary

// CrawlerPage is one page robots asked for.
type CrawlerPage = apitypes.CrawlerPage

// CrawlerJourney is one robot's run of requests from one network.
type CrawlerJourney = apitypes.CrawlerJourney

// CrawlerJourneyDetail is one journey and its requests in order.
type CrawlerJourneyDetail = apitypes.CrawlerJourneyDetail

// Crawlers reads what robots asked an app for over a period (24h, 7d, 30d or 90d, empty for 7d),
// every robot's or the one crawler names, every answer or those status names (404, 5xx).
func (c *Client) Crawlers(ctx context.Context, org, app, period, crawler, status string) (AppCrawlers, error) {
	return Send(ctx, c, endpoints.GetCrawlers, endpoints.GetCrawlersQuery{Period: period, Crawler: crawler, Status: status}, org, app)
}

// AppTraffic is every request to an app, counted and grouped.
type AppTraffic = apitypes.AppTraffic

// TrafficRow is one group of an app's traffic.
type TrafficRow = apitypes.TrafficRow

// AppTrafficRequests are an app's latest requests one by one.
type AppTrafficRequests = apitypes.AppTrafficRequests

// TrafficRequest is one request to an app.
type TrafficRequest = apitypes.TrafficRequest

// TrafficQuery is one reading of an app's traffic: a reading's period, from, to and tz, By as a
// comma list of what to group by, Filters by name (path, agent, country…), and status, cached and
// limit.
type TrafficQuery struct {
	Reading       AnalyticsQuery
	By            string
	Filters       map[string]string
	Cached        string
	Status, Limit int
}

// Traffic reads an app's traffic, grouped and narrowed as q says.
func (c *Client) Traffic(ctx context.Context, org, app string, q TrafficQuery) (AppTraffic, error) {
	return Send(ctx, c, endpoints.GetTraffic, q.declared(), org, app)
}

// TrafficRequests reads an app's latest requests one by one, narrowed as q says; By is ignored.
func (c *Client) TrafficRequests(ctx context.Context, org, app string, q TrafficQuery) (AppTrafficRequests, error) {
	d := q.declared()
	d.By = ""
	return Send(ctx, c, endpoints.GetTrafficRequests, d, org, app)
}
