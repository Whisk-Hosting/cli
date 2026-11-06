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
