package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The analytics routes.
var (
	GetAnalytics         = New[GetAnalyticsQuery, apitypes.AppAnalytics]("GET /v1/orgs/{org}/apps/{app}/analytics")
	GetAnalyticsVisitors = New[GetAnalyticsVisitorsQuery, apitypes.AppVisitors]("GET /v1/orgs/{org}/apps/{app}/analytics/visitors")
	GetAnalyticsVisitor  = New[GetAnalyticsVisitorQuery, apitypes.AnalyticsVisitorSteps]("GET /v1/orgs/{org}/apps/{app}/analytics/visitors/{visitor}")
	GetAnalyticsChart    = New[GetAnalyticsChartQuery, apitypes.AnalyticsChart]("GET /v1/orgs/{org}/apps/{app}/analytics/chart")
	OpenAnalytics        = New[None, apitypes.AnalyticsOpen]("POST /v1/orgs/{org}/apps/{app}/analytics/open")
	GetTraffic           = New[TrafficQuery, apitypes.AppTraffic]("GET /v1/orgs/{org}/apps/{app}/analytics/traffic")
	GetTrafficRequests   = New[TrafficQuery, apitypes.AppTrafficRequests]("GET /v1/orgs/{org}/apps/{app}/analytics/traffic/requests")

	_ = public.Add(GetAnalytics, GetAnalyticsVisitors, GetAnalyticsVisitor, GetAnalyticsChart, OpenAnalytics, GetTraffic, GetTrafficRequests)
)

// GetAnalyticsQuery is what GetAnalytics reads from the query.
type GetAnalyticsQuery struct {
	By     string `query:"by"`
	Event  string `query:"event"`
	From   string `query:"from"`
	Limit  int    `query:"limit"`
	Page   string `query:"page"`
	Period string `query:"period"`
	Source string `query:"source"`
	To     string `query:"to"`
	Tz     string `query:"tz"`
}

// GetAnalyticsVisitorsQuery is what GetAnalyticsVisitors reads from the query.
type GetAnalyticsVisitorsQuery struct {
	Cursor int    `query:"cursor"`
	Event  string `query:"event"`
	From   string `query:"from"`
	Limit  int    `query:"limit"`
	Page   string `query:"page"`
	Period string `query:"period"`
	Source string `query:"source"`
	To     string `query:"to"`
	Tz     string `query:"tz"`
}

// GetAnalyticsChartQuery is what GetAnalyticsChart reads from the query: the reading, the unit
// (by: hour, day or month) and the measure (visitors or views).
type GetAnalyticsChartQuery struct {
	By      string `query:"by"`
	Event   string `query:"event"`
	From    string `query:"from"`
	Measure string `query:"measure"`
	Page    string `query:"page"`
	Period  string `query:"period"`
	Source  string `query:"source"`
	To      string `query:"to"`
	Tz      string `query:"tz"`
}

// GetAnalyticsVisitorQuery is what GetAnalyticsVisitor reads from the query.
type GetAnalyticsVisitorQuery struct {
	Event  string `query:"event"`
	From   string `query:"from"`
	Page   string `query:"page"`
	Period string `query:"period"`
	Source string `query:"source"`
	To     string `query:"to"`
	Tz     string `query:"tz"`
}

// TrafficQuery is what GetTraffic and GetTrafficRequests read from the query: the window, how to group, and each filter. Cached is true or false, left empty for both.
type TrafficQuery struct {
	Address   string `query:"address"`
	Agent     string `query:"agent"`
	By        string `query:"by"`
	Cached    string `query:"cached"`
	Country   string `query:"country"`
	EdgeCache string `query:"edge_cache"`
	From      string `query:"from"`
	Host      string `query:"host"`
	Kind      string `query:"kind"`
	Limit     int    `query:"limit"`
	Method    string `query:"method"`
	Network   string `query:"network"`
	Path      string `query:"path"`
	Period    string `query:"period"`
	Query     string `query:"query"`
	Referer   string `query:"referer"`
	Source    string `query:"source"`
	Status    int    `query:"status"`
	To        string `query:"to"`
	Tz        string `query:"tz"`
	UserAgent string `query:"user_agent"`
	UserId    string `query:"user_id"`
}
