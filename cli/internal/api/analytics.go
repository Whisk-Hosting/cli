package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/whisk-run/contract/apitypes"
)

// AppAnalytics is an app's visits over a period, from the pages that load the analytics script.
type AppAnalytics = apitypes.AppAnalytics

// AnalyticsCount is one page, source or event and how often it came.
type AnalyticsCount = apitypes.AnalyticsCount

// AnalyticsCrawler is one robot that asked for the app's pages and how often.
type AnalyticsCrawler = apitypes.AnalyticsCrawler

// Analytics reads an app's visits over a period: 24h, 7d, 30d or 90d, empty for 7d.
func (c *Client) Analytics(ctx context.Context, org, app, period string) (AppAnalytics, error) {
	var out AppAnalytics
	path := appPath(org, app) + "/analytics"
	if period != "" {
		path += "?" + url.Values{"period": {period}}.Encode()
	}
	return out, c.Do(ctx, http.MethodGet, path, nil, &out)
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
