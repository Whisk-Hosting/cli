package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/whisk-run/contract/apitypes"
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

// values is a reading as the API's query values, the last 7 days when it names no period, from
// or to. Pure.
func (q AnalyticsQuery) values() url.Values {
	if q.Period == "" && q.From == "" && q.To == "" {
		q.Period = "7d"
	}
	v := url.Values{}
	for k, s := range map[string]string{"period": q.Period, "from": q.From, "to": q.To, "tz": q.TZ,
		"source": q.Source, "page": q.Page, "event": q.Event, "by": q.By} {
		if s != "" {
			v.Set(k, s)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	return v
}

func withQuery(path string, v url.Values) string {
	if len(v) == 0 {
		return path
	}
	return path + "?" + v.Encode()
}

// Analytics reads an app's visits over a reading, broken down as it asks.
func (c *Client) Analytics(ctx context.Context, org, app string, q AnalyticsQuery) (AppAnalytics, error) {
	var out AppAnalytics
	return out, c.Do(ctx, http.MethodGet, withQuery(appPath(org, app)+"/analytics", q.values()), nil, &out)
}

// Visitors reads a page of an app's visitors over a reading, latest first, from the page cursor
// names ("" for the first).
func (c *Client) Visitors(ctx context.Context, org, app string, q AnalyticsQuery, cursor string) (AppVisitors, error) {
	var out AppVisitors
	v := q.values()
	v.Del("by")
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	return out, c.Do(ctx, http.MethodGet, withQuery(appPath(org, app)+"/analytics/visitors", v), nil, &out)
}

// VisitorSteps reads what one visitor did over a reading, earliest first.
func (c *Client) VisitorSteps(ctx context.Context, org, app, id string, q AnalyticsQuery) (AnalyticsVisitorSteps, error) {
	var out AnalyticsVisitorSteps
	v := url.Values{}
	for _, k := range []string{"period", "from", "to", "tz"} {
		if s := q.values().Get(k); s != "" {
			v.Set(k, s)
		}
	}
	return out, c.Do(ctx, http.MethodGet, withQuery(appPath(org, app)+"/analytics/visitors/"+pathSeg(id), v), nil, &out)
}

// AppCrawlers is what robots asked an app for over a period: each robot, the pages, the answers
// and the latest journeys.
type AppCrawlers = apitypes.AppCrawlers

// CrawlerJourney is one robot's run of requests from one network.
type CrawlerJourney = apitypes.CrawlerJourney

// CrawlerJourneyDetail is one journey and its requests in order.
type CrawlerJourneyDetail = apitypes.CrawlerJourneyDetail

// Crawlers reads what robots asked an app for over a period (24h, 7d, 30d or 90d, empty for 7d),
// every robot's or the one crawler names.
func (c *Client) Crawlers(ctx context.Context, org, app, period, crawler string) (AppCrawlers, error) {
	var out AppCrawlers
	q := url.Values{}
	if period != "" {
		q.Set("period", period)
	}
	if crawler != "" {
		q.Set("crawler", crawler)
	}
	path := appPath(org, app) + "/analytics/crawlers"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return out, c.Do(ctx, http.MethodGet, path, nil, &out)
}

// CrawlerJourney reads one robot's journey and its requests.
func (c *Client) CrawlerJourney(ctx context.Context, org, app, id string) (CrawlerJourneyDetail, error) {
	var out CrawlerJourneyDetail
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/analytics/crawlers/journeys/"+pathSeg(id), nil, &out)
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

// values are q as the API's query values. Pure.
func (q TrafficQuery) values() url.Values {
	v := url.Values{}
	for _, k := range []string{"period", "from", "to", "tz"} {
		if s := q.Reading.values().Get(k); s != "" {
			v.Set(k, s)
		}
	}
	for k, s := range q.Filters {
		if s != "" {
			v.Set(k, s)
		}
	}
	if q.By != "" {
		v.Set("by", q.By)
	}
	if q.Cached != "" {
		v.Set("cached", q.Cached)
	}
	if q.Status != 0 {
		v.Set("status", strconv.Itoa(q.Status))
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	return v
}

// Traffic reads an app's traffic, grouped and narrowed as q says.
func (c *Client) Traffic(ctx context.Context, org, app string, q TrafficQuery) (AppTraffic, error) {
	var out AppTraffic
	return out, c.Do(ctx, http.MethodGet, withQuery(appPath(org, app)+"/analytics/traffic", q.values()), nil, &out)
}

// TrafficRequests reads an app's latest requests one by one, narrowed as q says; By is ignored.
func (c *Client) TrafficRequests(ctx context.Context, org, app string, q TrafficQuery) (AppTrafficRequests, error) {
	v := q.values()
	v.Del("by")
	var out AppTrafficRequests
	return out, c.Do(ctx, http.MethodGet, withQuery(appPath(org, app)+"/analytics/traffic/requests", v), nil, &out)
}
