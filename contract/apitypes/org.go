package apitypes

import (
	"encoding/json"
	"time"

	werrors "github.com/whisk-run/contract/errors"
)

// A business's own screens beyond its apps: deleted apps, branches and previews, its own domain,
// settings, trust, exports, demos and handovers, agent identities and a person's own account.

// DeletedApp is an app deleted within its grace period, with when it is released
// (GET /orgs/:org/deleted-apps, CONTROL-PLANE.md §6.5).
type DeletedApp struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	DeletedAt time.Time `json:"deleted_at"`
	ReleaseAt time.Time `json:"release_at"`
}

// Branch is one branch of an app's repository. Preview is the name of the branch's preview
// environment, empty when it has none.
type Branch struct {
	Name    string `json:"name"`
	Commit  string `json:"commit"`
	Default bool   `json:"default"`
	Preview string `json:"preview,omitempty"`
}

// StartPreviewRequest is POST /orgs/:org/apps/:app/previews: the branch to preview.
type StartPreviewRequest struct {
	Branch string `json:"branch"`
}

// StartedPreview is the preview and the deploy following its branch's tip.
type StartedPreview struct {
	Environment Environment `json:"environment"`
	Deploy      Deploy      `json:"deploy"`
}

// OrgDomain is a business's own domain (CONTROL-PLANE.md §6.11): once verified, every app
// answers at <app>.<domain>. While unverified it carries the records to add: a TXT proving
// ownership and a wildcard CNAME (or, where the provider has no wildcard CNAME, wildcard A/AAAA
// records to Addresses); once verified, the apps it gives an address to.
type OrgDomain struct {
	ID          string         `json:"id"`
	Domain      string         `json:"domain"`
	Verified    bool           `json:"verified"`
	TXTRecord   string         `json:"txt_record,omitempty"`
	TXTValue    string         `json:"txt_value,omitempty"`
	CNAMERecord string         `json:"cname_record,omitempty"`
	CNAMETarget string         `json:"cname_target,omitempty"`
	Addresses   []string       `json:"addresses,omitempty"`
	Apps        []OrgDomainApp `json:"apps"`
	CreatedAt   time.Time      `json:"created_at"`
}

// OrgDomainApp is where one app answers on the business domain.
type OrgDomainApp struct {
	App      string `json:"app"`
	Hostname string `json:"hostname"`
	URL      string `json:"url"`
}

// OrgDomainRequest is POST /orgs/:org/domains.
type OrgDomainRequest struct {
	Domain string `json:"domain"`
}

// MintedToken is a new token and its value, which is shown once.
type MintedToken struct {
	Token Token  `json:"token"`
	Value string `json:"value"`
}

// AgentCategory is a category of change an agent identity may hold (CONTROL-PLANE.md §4.4). The
// human category exists on routes, but no identity ever holds it.
type AgentCategory string

const (
	CategoryRead    AgentCategory = "read"
	CategoryOperate AgentCategory = "operate"
	CategoryChange  AgentCategory = "change"
	CategoryDestroy AgentCategory = "destroy"
)

// AgentIdentity is a coding agent's own standing on the platform, acting for the operator: never
// a credential's value.
type AgentIdentity struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Categories  []AgentCategory `json:"categories"`
	Paused      bool            `json:"paused"`
	PausedAt    *time.Time      `json:"paused_at,omitempty"`
	Credentials []Token         `json:"credentials"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// AgentIdentityMinted is a created identity or a new credential, the value shown once. Value is
// absent when the identity adopted a read token the agent already holds.
type AgentIdentityMinted struct {
	Identity AgentIdentity `json:"identity"`
	Value    string        `json:"value,omitempty"`
}

// Demo is a demo business as the operator who made it lists it (CONTROL-PLANE.md §6.1). Domain is
// the company's email domain; only an email there may claim it. ClaimURL is the handover link;
// ClaimedBy the email of the person who claimed it.
type Demo struct {
	Org       Org        `json:"org"`
	Domain    string     `json:"domain"`
	ClaimURL  string     `json:"claim_url"`
	MadeBy    string     `json:"made_by"`
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`
	ClaimedBy string     `json:"claimed_by,omitempty"`
}

// DemoKind is what a handover link hands over: a demo an operator made, or a client business an
// agency is handing over.
type DemoKind string

const (
	HandoverDemo   DemoKind = "demo"
	HandoverClient DemoKind = "client"
)

// DemoInfo is what the handover link shows the person who opens it, signed in; a client
// handover names the agency and the contact. CanClaim says whether the person may claim it;
// Refusal says why not.
type DemoInfo struct {
	Kind      DemoKind        `json:"kind"`
	Agency    string          `json:"agency,omitempty"`
	Contact   string          `json:"contact,omitempty"`
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	Domain    string          `json:"domain"`
	MadeBy    string          `json:"made_by"`
	Apps      []DemoApp       `json:"apps"`
	Claimed   bool            `json:"claimed"`
	ClaimedAt *time.Time      `json:"claimed_at,omitempty"`
	CanClaim  bool            `json:"can_claim"`
	Refusal   *werrors.Detail `json:"refusal,omitempty"`
}

// DemoApp is one of a demo's apps on its handover page.
type DemoApp struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// NotificationChannel is a way a notification reaches a business (CONTROL-PLANE.md §6.14).
type NotificationChannel string

const (
	ChannelEmail   NotificationChannel = "email"
	ChannelSlack   NotificationChannel = "slack"
	ChannelWebhook NotificationChannel = "webhook"
)

// OrgSettings is GET /orgs/:org/settings (DASHBOARD.md §4.18): which channels carry each kind of
// notification, the webhooks, single sign-on and database access. Secrets are reported as set
// or not, never shown.
type OrgSettings struct {
	Notifications   map[string][]NotificationChannel `json:"notifications"`
	SlackWebhookSet bool                             `json:"slack_webhook_set"`
	WebhookURL      string                           `json:"webhook_url,omitempty"`
	SSO             SSOSettings                      `json:"sso"`
	DBAccess        bool                             `json:"db_access"`
}

// SSOSettings is the org's single sign-on, without its client secret. Verified is true once the
// domain's TXT record was found; Record is that record. CallbackURL is the redirect URI to
// register with the identity provider.
type SSOSettings struct {
	Enabled     bool       `json:"enabled"`
	Issuer      string     `json:"issuer,omitempty"`
	ClientID    string     `json:"client_id,omitempty"`
	Domain      string     `json:"domain,omitempty"`
	Verified    bool       `json:"verified"`
	Record      *DNSRecord `json:"record,omitempty"`
	CallbackURL string     `json:"callback_url"`
}

// DNSRecord is one record an owner adds at their DNS provider.
type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// OrgSettingsRequest is PUT /orgs/:org/settings: every field optional, an absent one unchanged.
type OrgSettingsRequest struct {
	Notifications   map[string][]NotificationChannel `json:"notifications,omitempty"`
	SlackWebhookURL *string                          `json:"slack_webhook_url,omitempty"`
	WebhookURL      *string                          `json:"webhook_url,omitempty"`
	SSO             *SSORequest                      `json:"sso,omitempty"`
	DBAccess        *bool                            `json:"db_access,omitempty"`
}

// SSORequest carries the client secret once, on the way in; without it the kept one stays.
type SSORequest struct {
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Domain       string `json:"domain"`
}

// Preferences are a person's own settings (GET/PUT /me/preferences, DASHBOARD.md §4.19): their
// time zone and, per notification kind, whether the platform's mail about it reaches their
// inbox. A kind absent from Notifications is on.
type Preferences struct {
	Notifications map[string]bool `json:"notifications"`
	TimeZone      string          `json:"time_zone"`
}

// Trust is GET /orgs/:org/trust (DASHBOARD.md §4.16): where the data lives, how it is backed up
// and proven restorable, who can see it, and where the platform's status is.
type Trust struct {
	Statement       string       `json:"statement"`
	StatementPDFURL string       `json:"statement_pdf_url,omitempty"`
	Backups         TrustBackups `json:"backups"`
	Access          TrustAccess  `json:"access"`
	Status          TrustStatus  `json:"status"`
}

// TrustBackups is the backup half of the trust page. LastFullRebuild is the newest kill test
// (INFRA.md §11): a node destroyed and rebuilt from backups, timed, with the full harness run
// against the result.
type TrustBackups struct {
	LastContinuousAt *time.Time `json:"last_continuous_at,omitempty"`
	Drills           []Drill    `json:"drills"`
	LastFullRebuild  *Rebuild   `json:"last_full_rebuild,omitempty"`
}

// TrustAccess is who can see the org's data.
type TrustAccess struct {
	Members int `json:"members"`
	Guests  int `json:"guests"`
}

// TrustStatus is the platform's status page, when there is one.
type TrustStatus struct {
	URL string `json:"url,omitempty"`
}

// DrillStatus is where a restore drill stands. A skipped drill had nothing to restore.
type DrillStatus string

const (
	DrillRunning DrillStatus = "running"
	DrillDone    DrillStatus = "done"
	DrillFailed  DrillStatus = "failed"
	DrillSkipped DrillStatus = "skipped"
)

// Drill is one restore drill as the trust page and the operator see it. Machine and Error stay
// with the operator; ReportMD is carried by a drill's own route and the operator listing, not by
// the trust page's rows.
type Drill struct {
	ID             string      `json:"id"`
	At             time.Time   `json:"at"`
	Machine        string      `json:"machine,omitempty"`
	Status         DrillStatus `json:"status"`
	RestoreSeconds int         `json:"restore_seconds"`
	ChecksPassed   int         `json:"checks_passed"`
	ChecksTotal    int         `json:"checks_total"`
	Passed         bool        `json:"passed"`
	ReportURL      string      `json:"report_url,omitempty"`
	ReportMD       string      `json:"report_md,omitempty"`
	Error          string      `json:"error,omitempty"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
}

// Rebuild is one full rebuild as the trust page and the operator see it.
type Rebuild struct {
	At             time.Time `json:"at"`
	RebuildSeconds int       `json:"rebuild_seconds"`
	Passed         bool      `json:"passed"`
	Env            string    `json:"env,omitempty"`
	Node           string    `json:"node,omitempty"`
}

// ExportStatus is where an export of the whole org stands.
type ExportStatus string

const (
	ExportQueued  ExportStatus = "queued"
	ExportRunning ExportStatus = "running"
	ExportDone    ExportStatus = "done"
	ExportFailed  ExportStatus = "failed"
	ExportExpired ExportStatus = "expired"
)

// Export is one export of the whole org (CONTROL-PLANE.md §6.19). Its link comes only from
// reading that one export, because signing a link is audited and a list is read far more often
// than anyone downloads.
type Export struct {
	ID          string          `json:"id"`
	Status      ExportStatus    `json:"status"`
	RequestedBy string          `json:"requested_by"`
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   time.Time       `json:"started_at,omitzero"`
	FinishedAt  time.Time       `json:"finished_at,omitzero"`
	ExpiresAt   time.Time       `json:"expires_at,omitzero"`
	Bytes       int64           `json:"bytes"`
	Contents    map[string]any  `json:"contents"`
	Error       *werrors.Detail `json:"error,omitempty"`
	URL         string          `json:"url,omitempty"`
}

// AccountExport is GET /me/export: everything the platform holds about one person as themselves
// (CONTROL-PLANE.md §4.8). It holds no credential: passkeys are listed by name and dates, never
// their keys; tokens by label and dates, never their values or hashes. Every list is empty
// rather than null.
type AccountExport struct {
	ExportedAt  time.Time          `json:"exported_at"`
	Profile     ExportProfile      `json:"profile"`
	Memberships []ExportMembership `json:"memberships"`
	Sessions    []ExportSession    `json:"sessions"`
	Passkeys    []ExportPasskey    `json:"passkeys"`
	Tokens      []ExportToken      `json:"tokens"`
	Audit       []ExportAuditEvent `json:"audit"`
	Feedback    []ExportFeedback   `json:"feedback"`
}

// ExportProfile is the person's own row.
type ExportProfile struct {
	ID            string          `json:"id"`
	Email         string          `json:"email"`
	Name          string          `json:"name"`
	IsOperator    bool            `json:"is_operator"`
	HasPassword   bool            `json:"has_password"`
	Preferences   json.RawMessage `json:"preferences"`
	CreatedAt     time.Time       `json:"created_at"`
	FirstSignInAt *time.Time      `json:"first_sign_in_at"`
	LastSignInAt  *time.Time      `json:"last_sign_in_at"`
}

// ExportMembership is one business they belong or belonged to.
type ExportMembership struct {
	OrgID     string       `json:"org_id"`
	OrgSlug   string       `json:"org_slug"`
	OrgName   string       `json:"org_name"`
	Role      Role         `json:"role"`
	Kind      MemberKind   `json:"kind"`
	Status    MemberStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
}

// ExportSession is one whisk.run sign-in still kept.
type ExportSession struct {
	ID         string       `json:"id"`
	Method     SignInMethod `json:"method"`
	IP         string       `json:"ip"`
	UserAgent  string       `json:"user_agent"`
	CreatedAt  time.Time    `json:"created_at"`
	LastSeenAt time.Time    `json:"last_seen_at"`
	ExpiresAt  time.Time    `json:"expires_at"`
	RevokedAt  *time.Time   `json:"revoked_at"`
}

// ExportPasskey is one passkey, without its key.
type ExportPasskey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// ExportToken is one token acting for them, without its value.
type ExportToken struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	OrgID      string     `json:"org_id"`
	Label      string     `json:"label"`
	DeviceName string     `json:"device_name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// ExportAuditEvent is one audit entry they made, or one about them.
type ExportAuditEvent struct {
	ID         string          `json:"id"`
	OrgID      string          `json:"org_id"`
	ActorKind  ActorKind       `json:"actor_kind"`
	ActorID    string          `json:"actor_id"`
	Action     string          `json:"action"`
	TargetKind string          `json:"target_kind"`
	TargetID   string          `json:"target_id"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	IP         string          `json:"ip"`
	At         time.Time       `json:"at"`
}

// ExportFeedback is one piece of feedback they sent.
type ExportFeedback struct {
	ID        string         `json:"id"`
	Kind      FeedbackKind   `json:"kind"`
	Message   string         `json:"message"`
	Status    FeedbackStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
}

// LastOwnerOrg is one business named by ACCOUNT_LAST_OWNER, in its details' orgs: the person is
// its last owner, so deleting their account waits until they hand it on or delete it.
type LastOwnerOrg struct {
	Org         string `json:"org"`
	Name        string `json:"name"`
	PeopleURL   string `json:"people_url"`
	SettingsURL string `json:"settings_url"`
}
