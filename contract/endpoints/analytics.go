package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The analytics routes.
var (
	GetAnalytics         = New[GetAnalyticsQuery, apitypes.AppAnalytics]("GET /v1/orgs/{org}/apps/{app}/analytics")
	GetAnalyticsVisitors = New[GetAnalyticsVisitorsQuery, apitypes.AppVisitors]("GET /v1/orgs/{org}/apps/{app}/analytics/visitors")
	GetAnalyticsVisitor  = New[GetAnalyticsVisitorQuery, apitypes.AnalyticsVisitorSteps]("GET /v1/orgs/{org}/apps/{app}/analytics/visitors/{visitor}")
	OpenAnalytics        = New[None, apitypes.AnalyticsOpen]("POST /v1/orgs/{org}/apps/{app}/analytics/open")

	_ = public.Add(GetAnalytics, GetAnalyticsVisitors, GetAnalyticsVisitor, OpenAnalytics)
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
