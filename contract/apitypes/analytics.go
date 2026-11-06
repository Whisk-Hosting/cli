package apitypes

import "time"

// AppAnalytics is GET /orgs/:org/apps/:app/analytics: an app's visits and events over a period,
// as its analytics recorded them.
type AppAnalytics struct {
	Period   string    `json:"period"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Views    int       `json:"views"`
	Visitors int       `json:"visitors"`
	Visits   int       `json:"visits"`
	// BounceRate is the share of visits that saw one page and did nothing else, 0 to 1.
	BounceRate float64 `json:"bounce_rate"`
	// AverageSeconds is how long a visit lasted on average.
	AverageSeconds int              `json:"average_seconds"`
	Pages          []AnalyticsCount `json:"pages"`
	Sources        []AnalyticsCount `json:"sources"`
	Events         []AnalyticsCount `json:"events"`
	// KeptDays is how many days the business's plan keeps; 0 keeps everything.
	KeptDays int `json:"kept_days"`
}

// AnalyticsCount is one row of a breakdown: a page, a referring site or an event, and how many
// times it was seen.
type AnalyticsCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// AnalyticsOpen is POST /orgs/:org/apps/:app/analytics/open: the address that opens the app's
// analytics, signed in, for the person who asked. It carries their session there, so it is
// opened at once and never kept.
type AnalyticsOpen struct {
	URL string `json:"url"`
}
