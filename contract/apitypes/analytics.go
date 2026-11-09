package apitypes

import "time"

// AppAnalytics is GET /orgs/:org/apps/:app/analytics: an app's visits and events over a period,
// or from and to, as its analytics recorded them, narrowed by Filters. Period is "" when the
// reading names from and to.
type AppAnalytics struct {
	Period   string           `json:"period"`
	From     time.Time        `json:"from"`
	To       time.Time        `json:"to"`
	Filters  AnalyticsFilters `json:"filters"`
	Views    int              `json:"views"`
	Visitors int              `json:"visitors"`
	Visits   int              `json:"visits"`
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
	// By is what Breakdown breaks the reading down by, "" for none: page, entry, exit, source,
	// event, country, browser, os, device, hour, day, month or event-property.
	By        string         `json:"by,omitempty"`
	Breakdown []AnalyticsRow `json:"breakdown,omitempty"`
	// TZ is the time zone a breakdown by hour, day or month starts its hours and days in.
	TZ string `json:"tz,omitempty"`
}

// AnalyticsFilters narrow a reading to the visits that came from a site (Source, its domain),
// viewed a page (Page, its path) or sent an event (Event, its name). "" leaves one out, and a
// comma in one means any of its values.
type AnalyticsFilters struct {
	Source string `json:"source,omitempty"`
	Page   string `json:"page,omitempty"`
	Event  string `json:"event,omitempty"`
}

// AnalyticsRow is one row of a breakdown. A breakdown by hour, day or month has the views and
// visitors of each, named 2026-11-06 14:00, 2026-11-06 or 2026-11, every one in the reading
// included. Every other breakdown counts one thing in Count, most first: the views of a page,
// the visits that entered or left by it, came from a source or a country or used a browser, os
// or device, the times an event was sent, or for event-property the times an event carried a
// property's value, named property=value. By source, the row named "" is the visitors no other
// site sent: typed in, bookmarked, or under a page filter reaching that page from the app's own.
type AnalyticsRow struct {
	Name     string `json:"name"`
	Views    int    `json:"views,omitempty"`
	Visitors int    `json:"visitors,omitempty"`
	Count    int    `json:"count,omitempty"`
}

// AppVisitors is GET /orgs/:org/apps/:app/analytics/visitors: a page of the visitors of a
// reading, latest first. A visitor is one browser on one device, as analytics tells them apart
// without cookies; Total is how many the reading has. NextCursor, sent back as cursor, reads
// the next page, and is "" on the last.
type AppVisitors struct {
	Period     string             `json:"period"`
	From       time.Time          `json:"from"`
	To         time.Time          `json:"to"`
	Filters    AnalyticsFilters   `json:"filters"`
	Total      int                `json:"total"`
	Visitors   []AnalyticsVisitor `json:"visitors"`
	NextCursor string             `json:"next_cursor"`
}

// AnalyticsVisitor is one visitor: when they were first and last seen in the reading, how many
// visits, page views and events, and where and on what, as far as analytics knows.
type AnalyticsVisitor struct {
	ID       string    `json:"id"`
	FirstAt  time.Time `json:"first_at"`
	LastAt   time.Time `json:"last_at"`
	Visits   int       `json:"visits"`
	Views    int       `json:"views"`
	Events   int       `json:"events"`
	Country  string    `json:"country,omitempty"`
	Region   string    `json:"region,omitempty"`
	City     string    `json:"city,omitempty"`
	Browser  string    `json:"browser,omitempty"`
	OS       string    `json:"os,omitempty"`
	Device   string    `json:"device,omitempty"`
	Language string    `json:"language,omitempty"`
	Hostname string    `json:"hostname,omitempty"`
}

// AnalyticsVisitorSteps is GET /orgs/:org/apps/:app/analytics/visitors/:visitor: what one
// visitor did over a reading, earliest first, at most the latest 500.
type AnalyticsVisitorSteps struct {
	ID    string          `json:"id"`
	From  time.Time       `json:"from"`
	To    time.Time       `json:"to"`
	Steps []AnalyticsStep `json:"steps"`
}

// AnalyticsStep is one page a visitor viewed (Kind view) or one event the app sent for them
// (Kind event, named in Event). Source is the site the visit came from; Visit tells one visit
// from the next.
type AnalyticsStep struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Path     string    `json:"path"`
	Query    string    `json:"query,omitempty"`
	Event    string    `json:"event,omitempty"`
	Source   string    `json:"source,omitempty"`
	Visit    string    `json:"visit"`
	Hostname string    `json:"hostname,omitempty"`
}

// AppTraffic is GET /orgs/:org/apps/:app/analytics/traffic: every request the edge or the CDN
// answered for the app over the period, or from and to, people, robots and scripts alike, and
// what the CDN answered from its copy, which never reached the app. By is what the rows are
// grouped by, in order, from day, hour (both UTC), path, status, cached, source, method, host,
// agent, kind, user_agent, referer, country and network; each row's Group names its value for
// each. Agent is a crawler's, browser's or tool's name, "Other bots", "Other" or "(none)"; Kind
// is the crawler's kind (search, ai_assistant, ai_crawler, preview, seo, other), browser, tool or
// unknown. Filters are what narrowed it, each by name. Requests are kept as long as the plan keeps
// analytics, at most 90 days (KeptDays), and the newest arrive a few minutes late.
type AppTraffic struct {
	Period    string            `json:"period"`
	From      time.Time         `json:"from"`
	To        time.Time         `json:"to"`
	By        []string          `json:"by"`
	Filters   map[string]string `json:"filters"`
	Requests  int               `json:"requests"`
	Addresses int               `json:"addresses"`
	Rows      []TrafficRow      `json:"rows"`
	KeptDays  int               `json:"kept_days"`
}

// TrafficRow is one group of the traffic: its value for each part grouped by, its requests and
// how many client addresses made them.
type TrafficRow struct {
	Group     map[string]string `json:"group"`
	Requests  int               `json:"requests"`
	Addresses int               `json:"addresses"`
}

// AppTrafficRequests is GET /orgs/:org/apps/:app/analytics/traffic/requests: the app's latest
// requests one by one, narrowed as for AppTraffic, latest first.
type AppTrafficRequests struct {
	From     time.Time         `json:"from"`
	To       time.Time         `json:"to"`
	Filters  map[string]string `json:"filters"`
	Requests []TrafficRequest  `json:"requests"`
}

// TrafficRequest is one request to an app: its time, method, host, path and query (without its ?,
// its secrets masked: a token, key, code, password or signature reads REDACTED), answer, bytes
// sent and received, how long the edge took in all (0 for the CDN's copy), how long the app took
// to send its headers and how long the request waited for the app to wake (each left out when
// that did not happen), Source (edge or cdn), whether the CDN answered from its copy and what
// the edge's own cache said (hit or miss, left out when it was not asked), the user agent as
// sent (cut to 512 bytes) and the agent and kind it names, the referer (secrets masked), the
// signed-in person's user id as the app knows it, the client's address (the CDN's log cuts it
// short), and the country (two letters) and network it is in.
type TrafficRequest struct {
	At         time.Time `json:"at"`
	Method     string    `json:"method"`
	Host       string    `json:"host"`
	Path       string    `json:"path"`
	Query      string    `json:"query,omitempty"`
	Status     int       `json:"status"`
	Bytes      int64     `json:"bytes"`
	Received   int64     `json:"received"`
	Millis     int       `json:"millis"`
	AppMillis  *int      `json:"app_millis,omitempty"`
	WakeMillis *int      `json:"wake_millis,omitempty"`
	Source     string    `json:"source"`
	Cached     bool      `json:"cached"`
	EdgeCache  string    `json:"edge_cache,omitempty"`
	UserAgent  string    `json:"user_agent"`
	Agent      string    `json:"agent"`
	Kind       string    `json:"kind"`
	Referer    string    `json:"referer,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
	Address    string    `json:"address"`
	Country    string    `json:"country,omitempty"`
	ASN        int       `json:"asn,omitempty"`
	Network    string    `json:"network,omitempty"`
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
// the one robot Pages, Statuses and Journeys are narrowed to, "" for every one, and Status the
// answers they are narrowed to: one (404) or a class (5xx), omitted for every answer. Crawlers
// is every robot whatever either narrows. Single requests are kept as long as the plan keeps
// analytics and at most 90 days (KeptDays).
type AppCrawlers struct {
	Period   string           `json:"period"`
	From     time.Time        `json:"from"`
	To       time.Time        `json:"to"`
	Crawler  string           `json:"crawler"`
	Status   string           `json:"status,omitempty"`
	Crawlers []CrawlerSummary `json:"crawlers"`
	// Pages are the pages asked for most, most first; with Status, the pages that got those
	// answers, counting only those requests.
	Pages []CrawlerPage `json:"pages"`
	// Statuses are the answers the app gave, most first.
	Statuses []CrawlerStatus `json:"statuses"`
	// Journeys are the latest journeys, latest first: one robot's requests from one network with
	// no pause longer than JourneyGapMinutes. With Status, the journeys that got such an answer.
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
