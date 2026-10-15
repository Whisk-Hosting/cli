package apitypes

import "time"

// The services Whisk works with on a business's behalf: a CDN in front of its apps
// (CONTROL-PLANE.md §6.29) and an optional copy of an app on GitHub (§6.26).

// CdnStatus is where an app's CDN switch stands.
type CdnStatus string

const (
	CdnOff       CdnStatus = "off"
	CdnStarting  CdnStatus = "starting"
	CdnSwitching CdnStatus = "switching"
	CdnLive      CdnStatus = "live"
	CdnStopping  CdnStatus = "stopping"
)

// CdnDirectReason is why one of an app's names goes direct rather than through the CDN: empty
// when it goes through, else a bare domain, the business's own domain, or a preview.
type CdnDirectReason string

const (
	CdnThrough  CdnDirectReason = ""
	CdnBare     CdnDirectReason = "bare"
	CdnBusiness CdnDirectReason = "business"
	CdnPreview  CdnDirectReason = "preview"
)

// CdnHostname is one of the app's names and whether visitors reach it through the CDN.
type CdnHostname struct {
	Hostname   string          `json:"hostname"`
	ThroughCDN bool            `json:"through_cdn"`
	Reason     CdnDirectReason `json:"reason"`
}

// AppCdn is GET /orgs/:org/apps/:app/cdn: one app's CDN switch and its traffic this month.
type AppCdn struct {
	Included        bool          `json:"included"`
	Available       bool          `json:"available"`
	On              bool          `json:"on"`
	Status          CdnStatus     `json:"status"`
	Error           string        `json:"error"`
	Since           *time.Time    `json:"since,omitempty"`
	ZoneHost        string        `json:"zone_host"`
	Hostnames       []CdnHostname `json:"hostnames"`
	TrafficBytes    int64         `json:"traffic_bytes"`
	OrgTrafficBytes int64         `json:"org_traffic_bytes"`
	AllowanceBytes  int64         `json:"allowance_bytes"`
}

// GitHubRepo is a repository an installation reaches; LinkedApp is the slug of the app linked to
// it.
type GitHubRepo struct {
	ID        int64  `json:"id"`
	FullName  string `json:"full_name"`
	Private   bool   `json:"private"`
	HTMLURL   string `json:"html_url"`
	LinkedApp string `json:"linked_app,omitempty"`
}

// GitHubInstallation is the App installed on one GitHub account, with its repositories;
// SettingsURL is where they are chosen.
type GitHubInstallation struct {
	ID           int64        `json:"id"`
	Account      string       `json:"account"`
	AccountType  string       `json:"account_type"`
	SettingsURL  string       `json:"settings_url"`
	Repositories []GitHubRepo `json:"repositories"`
	Problem      string       `json:"problem,omitempty"`
}

// GitHubOverview is GET /orgs/:org/github.
type GitHubOverview struct {
	SetUp         bool                 `json:"set_up"`
	Installations []GitHubInstallation `json:"installations"`
}

// GitHubBranchState is whether a branch matches on both sides, waits to be copied, or was
// refused.
type GitHubBranchState string

const (
	BranchInSync  GitHubBranchState = "in_sync"
	BranchWaiting GitHubBranchState = "waiting"
	BranchRefused GitHubBranchState = "refused"
)

// GitHubBranch is one branch of a link: its commit on Whisk and on GitHub.
type GitHubBranch struct {
	Name   string            `json:"name"`
	Whisk  string            `json:"whisk,omitempty"`
	GitHub string            `json:"github,omitempty"`
	State  GitHubBranchState `json:"state"`
}

// GitHubLinkStatus is whether an app's link to GitHub works.
type GitHubLinkStatus string

const (
	GitHubLinkActive GitHubLinkStatus = "active"
	GitHubLinkBroken GitHubLinkStatus = "broken"
)

// GitHubLink is GET and PUT /orgs/:org/apps/:app/github.
type GitHubLink struct {
	Repo      string           `json:"repo"`
	HTMLURL   string           `json:"html_url"`
	Status    GitHubLinkStatus `json:"status"`
	Problem   string           `json:"problem,omitempty"`
	SyncedAt  *time.Time       `json:"synced_at,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	Branches  []GitHubBranch   `json:"branches"`
}
