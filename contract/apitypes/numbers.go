package apitypes

import "time"

// The operator's weekly numbers (CONTROL-PLANE.md §6.25): each number as a process behaviour
// chart, where the public site's visitors came from and how they read it, and the Google Search
// Console connection.

// NumberPeriod is what one point of a chart covers.
type NumberPeriod string

const (
	NumbersWeek NumberPeriod = "week"
	NumbersDay  NumberPeriod = "day"
)

// NumberChannel is how visits are counted: page loads in the edge's request log, or the site's
// own beacon.
type NumberChannel string

const (
	ChannelServer  NumberChannel = "server"
	ChannelBrowser NumberChannel = "browser"
)

// NumberInclude is a kind of visit the numbers may count besides people reading the public site.
type NumberInclude string

const (
	IncludeBots       NumberInclude = "bots"
	IncludeMalicious  NumberInclude = "malicious"
	IncludeWhiskUsers NumberInclude = "whisk_users"
	IncludeOtherSites NumberInclude = "other_sites"
	IncludeAppPages   NumberInclude = "app_pages"
	IncludeErrors     NumberInclude = "errors"
)

// NumberGroup is where a chart is shown: on the public site, getting started, on Whisk, or in
// Google's results.
type NumberGroup string

const (
	GroupSite   NumberGroup = "site"
	GroupStart  NumberGroup = "start"
	GroupWhisk  NumberGroup = "whisk"
	GroupSearch NumberGroup = "search"
)

// NumberVerdict is what a chart says as a whole.
type NumberVerdict string

const (
	VerdictEmpty  NumberVerdict = "empty"
	VerdictTooFew NumberVerdict = "too_few"
	VerdictSteady NumberVerdict = "steady"
	VerdictSignal NumberVerdict = "signal"
)

// NumberSignal is a rule one point of a chart breaks.
type NumberSignal string

const (
	SignalOutside   NumberSignal = "outside"
	SignalRun       NumberSignal = "run"
	SignalNearLimit NumberSignal = "near_limit"
	SignalJump      NumberSignal = "jump"
)

// Numbers is GET /operator/numbers. Channel is how visits were counted and Include the kinds of
// visit counted besides people reading the public site. CountingSince is when the first visit
// was counted; absent until then. TopDays is how many days the tallies cover. Countries are
// two-letter codes; networks read "<organisation> (AS<number>)"; an empty name is unknown.
// Actions are the calls to action used (views are clicks); installers and skill readers the user
// agents that downloaded an install script and read the skill (views are fetches). Screens,
// times, scrolls and speeds are bands; exits count visitors who left from a page; sign-up sources
// count sign-ups and the pages they opened first, "(unseen)" when none was seen. BotRules are the
// user agents the operator marked as bots, newest first.
type Numbers struct {
	Period        NumberPeriod    `json:"period"`
	Channel       NumberChannel   `json:"channel"`
	Include       []NumberInclude `json:"include"`
	CountingSince *time.Time      `json:"counting_since,omitempty"`
	Metrics       []NumberMetric  `json:"metrics"`
	TopDays       int             `json:"top_days"`
	Sources       []NumberTally   `json:"sources"`
	Pages         []NumberTally   `json:"pages"`
	Campaigns     []NumberTally   `json:"campaigns"`
	Referrers     []NumberTally   `json:"referrers"`
	Browsers      []NumberTally   `json:"browsers"`
	Countries     []NumberTally   `json:"countries"`
	Networks      []NumberTally   `json:"networks"`
	Actions       []NumberTally   `json:"actions"`
	Installers    []NumberTally   `json:"installers"`
	SkillReaders  []NumberTally   `json:"skill_readers"`
	Devices       []NumberTally   `json:"devices"`
	Screens       []NumberTally   `json:"screens"`
	Languages     []NumberTally   `json:"languages"`
	Times         []NumberTally   `json:"times"`
	Scrolls       []NumberTally   `json:"scrolls"`
	Speeds        []NumberTally   `json:"speeds"`
	Exits         []NumberTally   `json:"exits"`
	Outbound      []NumberTally   `json:"outbound"`
	SignupSources []NumberTally   `json:"signup_sources"`
	BotRules      []NumberBotRule `json:"bot_rules"`
	Search        NumberSearch    `json:"search"`
	GeneratedAt   time.Time       `json:"generated_at"`
}

// NumberMetric is one number read as a process behaviour chart.
type NumberMetric struct {
	Key         string        `json:"key"`
	Group       NumberGroup   `json:"group"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Points      []NumberPoint `json:"points"`
	Limits      *NumberLimits `json:"limits"`
	Verdict     NumberVerdict `json:"verdict"`
	Summary     string        `json:"summary"`
}

// NumberPoint is one period of a chart; Start is the period's first day in New Zealand (YYYY-MM-DD).
type NumberPoint struct {
	Start    string         `json:"start"`
	Value    float64        `json:"value"`
	Complete bool           `json:"complete"`
	Signals  []NumberSignal `json:"signals"`
}

// NumberLimits is a chart's average and natural process limits.
type NumberLimits struct {
	Centre         float64 `json:"centre"`
	Upper          float64 `json:"upper"`
	Lower          float64 `json:"lower"`
	MovingRange    float64 `json:"moving_range"`
	MovingRangeMax float64 `json:"moving_range_upper"`
	BaselinePoints int     `json:"baseline_points"`
}

// NumberTally is how often one source, page or campaign was seen.
type NumberTally struct {
	Name     string `json:"name"`
	Views    int    `json:"views"`
	Visitors int    `json:"visitors"`
}

// NumberBotRule is a user agent the operator marked as a bot: its visits count as bots, past ones
// included.
type NumberBotRule struct {
	UserAgent string    `json:"user_agent"`
	AddedBy   string    `json:"added_by"`
	AddedAt   time.Time `json:"added_at"`
}

// NumberBotRules answers PUT /operator/numbers/bots: every rule, newest first.
type NumberBotRules struct {
	BotRules []NumberBotRule `json:"bot_rules"`
}

// BotMarkRequest is PUT /operator/numbers/bots.
type BotMarkRequest struct {
	UserAgent string `json:"user_agent"`
	Bot       bool   `json:"bot"`
}

// NumberSearch is the Search Console connection on GET /operator/numbers and the answer of PUT
// and DELETE /operator/numbers/search. The service account's key is never answered. Site is the
// property connected, or the default for the public domain before one is. LastImportAt is when
// an import last worked; Error is why the last one failed, if it did. ReportedThrough is the
// last day Google reported (YYYY-MM-DD); the charts end there, and the searches and pages cover
// TopDays days ending on it.
type NumberSearch struct {
	Connected       bool                `json:"connected"`
	Site            string              `json:"site"`
	ClientEmail     string              `json:"client_email,omitempty"`
	ConnectedAt     *time.Time          `json:"connected_at,omitempty"`
	LastImportAt    *time.Time          `json:"last_import_at,omitempty"`
	Error           *SearchProblem      `json:"error"`
	ReportedThrough string              `json:"reported_through,omitempty"`
	TopDays         int                 `json:"top_days"`
	Queries         []NumberSearchTally `json:"queries"`
	Pages           []NumberSearchTally `json:"pages"`
}

// SearchProblem is why the last Search Console import failed.
type SearchProblem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// NumberSearchTally is one search or page in Google's results: clicks, impressions and the
// average position (1 is the top).
type NumberSearchTally struct {
	Name        string  `json:"name"`
	Clicks      int     `json:"clicks"`
	Impressions int     `json:"impressions"`
	Position    float64 `json:"position"`
}

// DailyNumbersMail answers GET /operator/numbers/daily: the operator's daily email for one New
// Zealand day (a date, 2006-01-02), written as it goes out each morning (CONTROL-PLANE.md §6.25).
type DailyNumbersMail struct {
	Day     string `json:"day"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}
