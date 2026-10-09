package apitypes

import "time"

// What a Promoted app adds beyond its resources (CONTROL-PLANE.md §6.18, §6.34): hourly backup
// points, Whisk's uptime checks and a status page at a hostname of the business's own.

// BackupPoint is one hourly backup point: when, at which position in the database's history, and
// whether the node saw it reach the backup archive.
type BackupPoint struct {
	At       time.Time `json:"at"`
	LSN      string    `json:"lsn"`
	Archived bool      `json:"archived"`
}

// BackupPoints is GET /orgs/:org/apps/:app/backup-points: whether points are being taken now (a
// Promoted app in force), how many days back a restore reaches, and the points of the last 30
// days, newest first.
type BackupPoints struct {
	Hourly     bool          `json:"hourly"`
	WindowDays int64         `json:"window_days"`
	Points     []BackupPoint `json:"points"`
}

// UptimeStatus is an app's status now.
type UptimeStatus string

const (
	UptimeUp      UptimeStatus = "up"
	UptimeDown    UptimeStatus = "down"
	UptimeUnknown UptimeStatus = "unknown"
)

// UptimeDay is one UTC day: its uptime as a percentage to two places, null for a day with no
// checks, and how long the app was down in it.
type UptimeDay struct {
	Date        string   `json:"date"`
	Uptime      *float64 `json:"uptime"`
	DownSeconds int64    `json:"down_seconds"`
}

// UptimeIncident is one incident: a minute or more of failed checks.
type UptimeIncident struct {
	ID              string     `json:"id"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	DurationSeconds int64      `json:"duration_seconds"`
	Detail          string     `json:"detail"`
}

// Uptime is GET /orgs/:org/apps/:app/uptime: whether the app is checked now, its status, when it
// was last checked, the 90 days' uptime as a percentage (null when nothing was checked), each
// day oldest first, and the incidents newest first.
type Uptime struct {
	Monitored bool             `json:"monitored"`
	Status    UptimeStatus     `json:"status"`
	CheckedAt *time.Time       `json:"checked_at"`
	Uptime90  *float64         `json:"uptime_90d"`
	Days      []UptimeDay      `json:"days"`
	Incidents []UptimeIncident `json:"incidents"`
}

// StatusPage is an app's status page: its hostname and title, whether the hostname is verified
// and its certificate issued, its address, whether it is served now (verified, and the app a
// Promoted app in force), and the records to add with the origin's addresses for a provider that
// has no CNAME.
type StatusPage struct {
	Hostname   string      `json:"hostname"`
	Title      string      `json:"title"`
	Verified   bool        `json:"verified"`
	CertStatus string      `json:"cert_status"`
	URL        string      `json:"url"`
	Live       bool        `json:"live"`
	Records    []DNSRecord `json:"records"`
	Addresses  []string    `json:"addresses"`
}

// StatusPageAnswer is GET /orgs/:org/apps/:app/status-page: the page, or null when there is none.
type StatusPageAnswer struct {
	Page *StatusPage `json:"page"`
}

// StatusPageRequest is PUT /orgs/:org/apps/:app/status-page.
type StatusPageRequest struct {
	Hostname string `json:"hostname"`
	Title    string `json:"title,omitempty"`
}
