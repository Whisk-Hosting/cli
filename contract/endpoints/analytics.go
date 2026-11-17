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
	GetAnalyticsFunnel   = New[GetAnalyticsFunnelQuery, apitypes.AnalyticsFunnel]("GET /v1/orgs/{org}/apps/{app}/analytics/funnel")
	GetAnalyticsTable    = New[GetAnalyticsTableQuery, apitypes.AnalyticsTable]("GET /v1/orgs/{org}/apps/{app}/analytics/table")
	GetAnalyticsChanges  = New[GetAnalyticsChangesQuery, apitypes.AnalyticsChanges]("GET /v1/orgs/{org}/apps/{app}/analytics/changes")
	OpenAnalytics        = New[None, apitypes.AnalyticsOpen]("POST /v1/orgs/{org}/apps/{app}/analytics/open")
	GetTraffic           = New[TrafficQuery, apitypes.AppTraffic]("GET /v1/orgs/{org}/apps/{app}/analytics/traffic")
	GetTrafficRequests   = New[TrafficQuery, apitypes.AppTrafficRequests]("GET /v1/orgs/{org}/apps/{app}/analytics/traffic/requests")

	_ = public.Add(GetAnalytics, GetAnalyticsVisitors, GetAnalyticsVisitor, GetAnalyticsChart, GetAnalyticsFunnel, GetAnalyticsTable, GetAnalyticsChanges, OpenAnalytics, GetTraffic, GetTrafficRequests)
)

// GetAnalyticsQuery is what GetAnalytics reads from the query.
type GetAnalyticsQuery struct {
	Browser string `query:"browser"`
	City    string `query:"city"`
	Country string `query:"country"`
	Device  string `query:"device"`
	OS      string `query:"os"`
	Region  string `query:"region"`
	Compare string `query:"compare"`
	By      string `query:"by"`
	Event   string `query:"event"`
	From    string `query:"from"`
	Limit   int    `query:"limit"`
	Page    string `query:"page"`
	Period  string `query:"period"`
	Source  string `query:"source"`
	To      string `query:"to"`
	Tz      string `query:"tz"`
}

// GetAnalyticsVisitorsQuery is what GetAnalyticsVisitors reads from the query.
type GetAnalyticsVisitorsQuery struct {
	Browser string `query:"browser"`
	City    string `query:"city"`
	Country string `query:"country"`
	Device  string `query:"device"`
	OS      string `query:"os"`
	Region  string `query:"region"`
	Cursor  int    `query:"cursor"`
	Event   string `query:"event"`
	From    string `query:"from"`
	Limit   int    `query:"limit"`
	Page    string `query:"page"`
	Period  string `query:"period"`
	Source  string `query:"source"`
	To      string `query:"to"`
	Tz      string `query:"tz"`
}

// GetAnalyticsChartQuery is what GetAnalyticsChart reads from the query: the reading, the unit
// (by: hour, day, week or month), the measure (visitors, visits, views, bounce_rate or
// average_seconds) and what to compare with (previous or year).
type GetAnalyticsChartQuery struct {
	Browser string `query:"browser"`
	City    string `query:"city"`
	Country string `query:"country"`
	Device  string `query:"device"`
	OS      string `query:"os"`
	Region  string `query:"region"`
	Compare string `query:"compare"`
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

// GetAnalyticsFunnelQuery is what GetAnalyticsFunnel reads from the query: the span and the
// steps in order, each a path (/pricing) or an event (event:signup), sent as step once each. A
// funnel reads every visitor of the span: a filter (source, page, event) is refused.
type GetAnalyticsFunnelQuery struct {
	Event  string   `query:"event"`
	From   string   `query:"from"`
	Page   string   `query:"page"`
	Period string   `query:"period"`
	Source string   `query:"source"`
	Step   []string `query:"step"`
	To     string   `query:"to"`
	Tz     string   `query:"tz"`
}

// GetAnalyticsVisitorQuery is what GetAnalyticsVisitor reads from the query.
type GetAnalyticsVisitorQuery struct {
	Browser string `query:"browser"`
	City    string `query:"city"`
	Country string `query:"country"`
	Device  string `query:"device"`
	OS      string `query:"os"`
	Region  string `query:"region"`
	Event   string `query:"event"`
	From    string `query:"from"`
	Page    string `query:"page"`
	Period  string `query:"period"`
	Source  string `query:"source"`
	To      string `query:"to"`
	Tz      string `query:"tz"`
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

// GetAnalyticsTableQuery is what GetAnalyticsTable reads from the query: the reading, what each
// row is (by), how many rows by name (limit), what to compare with (previous or year), and format
// csv for the table as a CSV file.
type GetAnalyticsTableQuery struct {
	Browser string `query:"browser"`
	By      string `query:"by"`
	City    string `query:"city"`
	Compare string `query:"compare"`
	Country string `query:"country"`
	Device  string `query:"device"`
	Event   string `query:"event"`
	Format  string `query:"format"`
	From    string `query:"from"`
	Limit   int    `query:"limit"`
	OS      string `query:"os"`
	Page    string `query:"page"`
	Period  string `query:"period"`
	Region  string `query:"region"`
	Source  string `query:"source"`
	To      string `query:"to"`
	Tz      string `query:"tz"`
}

// GetAnalyticsChangesQuery is what GetAnalyticsChanges reads from the query: the reading and what
// to compare with (previous, the default, or year).
type GetAnalyticsChangesQuery struct {
	Browser string `query:"browser"`
	City    string `query:"city"`
	Compare string `query:"compare"`
	Country string `query:"country"`
	Device  string `query:"device"`
	Event   string `query:"event"`
	From    string `query:"from"`
	OS      string `query:"os"`
	Page    string `query:"page"`
	Period  string `query:"period"`
	Region  string `query:"region"`
	Source  string `query:"source"`
	To      string `query:"to"`
	Tz      string `query:"tz"`
}
