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
	// Crawlers are the robots that asked the app for its pages over the period, most first:
	// search engines, AI assistants and crawlers, link previews, SEO tools and other bots. They
	// run no script, so they are counted from the platform's request log and are in none of the
	// numbers above.
	Crawlers []AnalyticsCrawler `json:"crawlers"`
	// KeptDays is how many days the business's plan keeps; 0 keeps everything.
	KeptDays int `json:"kept_days"`
}

// AnalyticsCount is one row of a breakdown: a page, a referring site or an event, and how many
// times it was seen.
type AnalyticsCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// AnalyticsCrawler is one robot and how many requests it made. Kind is search, ai_assistant (an
// assistant fetching for a person), ai_crawler, preview (a link preview), seo or other; every
// robot no list names is "Other bots".
type AnalyticsCrawler struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Requests int    `json:"requests"`
}

// AppCrawlers is GET /orgs/:org/apps/:app/analytics/crawlers: what robots asked an app for over
// a period, one by one, as the platform's request log and the CDN's log recorded it. Crawler is
// the one robot the answer is narrowed to, "" for every one. Single requests are kept as long as
// the plan keeps analytics and at most 90 days (KeptDays).
type AppCrawlers struct {
	Period   string           `json:"period"`
	From     time.Time        `json:"from"`
	To       time.Time        `json:"to"`
	Crawler  string           `json:"crawler"`
	Crawlers []CrawlerSummary `json:"crawlers"`
	// Pages are the pages asked for most, most first.
	Pages []CrawlerPage `json:"pages"`
	// Statuses are the answers the app gave, most first.
	Statuses []CrawlerStatus `json:"statuses"`
	// Journeys are the latest journeys, latest first: one robot's requests from one network with
	// no pause longer than JourneyGapMinutes.
	Journeys          []CrawlerJourney `json:"journeys"`
	JourneyGapMinutes int              `json:"journey_gap_minutes"`
	KeptDays          int              `json:"kept_days"`
}

// CrawlerSummary is one robot over a period: its requests, the pages it asked for, its journeys,
// the requests answered with an error (400 or over) or from the CDN's copy, and the first and last.
type CrawlerSummary struct {
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Requests int       `json:"requests"`
	Pages    int       `json:"pages"`
	Journeys int       `json:"journeys"`
	Errors   int       `json:"errors"`
	Cached   int       `json:"cached"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
}

// CrawlerPage is one page robots asked for: how often, by how many robots, and the last time and
// answer. The path carries no query.
type CrawlerPage struct {
	Path       string    `json:"path"`
	Requests   int       `json:"requests"`
	Crawlers   int       `json:"crawlers"`
	LastAt     time.Time `json:"last_at"`
	LastStatus int       `json:"last_status"`
}

// CrawlerStatus is how many robot requests the app answered with one HTTP status.
type CrawlerStatus struct {
	Status   int `json:"status"`
	Requests int `json:"requests"`
}

// CrawlerJourney is one robot's run of requests from one network. Network is its address cut
// short (the last part of an IPv4 address, an IPv6 address past 48 bits). ID opens it at
// GET /orgs/:org/apps/:app/analytics/crawlers/journeys/:id.
type CrawlerJourney struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Network   string    `json:"network"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Requests  int       `json:"requests"`
	Pages     int       `json:"pages"`
	Errors    int       `json:"errors"`
	FirstPath string    `json:"first_path"`
	LastPath  string    `json:"last_path"`
	// UserAgent, Address, Country and ASN are the journey's first request's.
	UserAgent string `json:"user_agent"`
	Address   string `json:"address"`
	Country   string `json:"country"`
	ASN       string `json:"asn"`
}

// CrawlerJourneyDetail is GET /orgs/:org/apps/:app/analytics/crawlers/journeys/:id: one journey
// and its requests in order, at most 1000.
type CrawlerJourneyDetail struct {
	Journey  CrawlerJourney   `json:"journey"`
	Requests []CrawlerRequest `json:"requests"`
	// More is true when the journey has more requests than are listed.
	More bool `json:"more"`
}

// CrawlerRequest is one robot request: when, the method and page, the answer, its size and how
// long the app took (0 when the CDN answered from its copy), and where it was answered: edge or
// cdn. UserAgent is the header as sent (cut to 512 bytes); Address is the robot's address, cut
// short when the CDN answered; Country is a two-letter code and ASN the network owner's name.
type CrawlerRequest struct {
	At        time.Time `json:"at"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Bytes     int64     `json:"bytes"`
	Millis    int       `json:"millis"`
	Source    string    `json:"source"`
	Cached    bool      `json:"cached"`
	UserAgent string    `json:"user_agent"`
	Address   string    `json:"address"`
	Host      string    `json:"host"`
	Referer   string    `json:"referer"`
	Country   string    `json:"country"`
	ASN       string    `json:"asn"`
}

// AnalyticsOpen is POST /orgs/:org/apps/:app/analytics/open: the address that opens the app's
// analytics, signed in, for the person who asked. It carries their session there, so it is
// opened at once and never kept.
type AnalyticsOpen struct {
	URL string `json:"url"`
}
