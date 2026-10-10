package apitypes

import (
	"time"
)

// What an app holds and says beyond its deploys: email (CONTROL-PLANE.md §6.12), storage and
// uploads (§6.13), package scanning (§6.28), logs and their forwarding (§6.17), errors, traces
// (DASHBOARD.md §4.9a), its customers (§4.6) and webhooks (§6.10).

// EmailProvider is which transport a business's mail goes through: Whisk's own, or a relay it
// brought before Whisk sent every business's mail itself.
type EmailProvider string

const (
	EmailPlatform EmailProvider = "platform"
	EmailSMTP     EmailProvider = "smtp"
)

// EmailDomainStatus is where a sending domain stands.
type EmailDomainStatus string

const (
	EmailDomainPending  EmailDomainStatus = "pending"
	EmailDomainVerified EmailDomainStatus = "verified"
	EmailDomainPaused   EmailDomainStatus = "paused"
)

// EmailAttachment is one file an app's email carries (POST …/email/send, `attachments`): the
// file base64 as content, or the full key of an object in the app's storage as storage_key, and
// a content_id for an image the HTML shows with <img src="cid:…">. The rules are in mailattach.
type EmailAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Content     string `json:"content,omitempty"`
	StorageKey  string `json:"storage_key,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

// EmailDomain is one sending domain: where it stands, and the records to publish if it is
// waiting.
type EmailDomain struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	// App is the slug of the app that registered the domain for its customers, which alone sends
	// from it; absent for a domain of the whole business.
	App           string            `json:"app,omitempty"`
	Status        EmailDomainStatus `json:"status"`
	DNSRecords    []DNSRecord       `json:"dns_records"`
	BounceRate    float64           `json:"bounce_rate"`
	ComplaintRate float64           `json:"complaint_rate"`
	VerifiedAt    time.Time         `json:"verified_at,omitzero"`
	// CheckedAt is when the provider was last asked whether the domain is verified.
	CheckedAt time.Time `json:"checked_at,omitzero"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailStatus is GET /orgs/:org/email: the provider, the domains and what has gone. MonthlyLimit
// is the plan's monthly allowance (0 for none) and SentMonth this month's sends; BilledPastDaily
// is whether sends past the daily allowance go out and are billed. SharedFrom is the business's
// address pattern on Whisk's domain, "*.<org>@<domain>", that apps send from when they name no
// domain of their own.
type EmailStatus struct {
	Provider        EmailProvider `json:"provider"`
	Domains         []EmailDomain `json:"domains"`
	SentToday       int64         `json:"sent_today"`
	DailyLimit      int64         `json:"daily_limit"`
	MonthlyLimit    int64         `json:"monthly_limit"`
	SentMonth       int64         `json:"sent_month"`
	BilledPastDaily bool          `json:"billed_past_daily"`
	SharedFrom      string        `json:"shared_from"`
}

// EmailProviderRequest is PUT /orgs/:org/email/provider: a business that brought its own relay
// moves onto Whisk's mail with kind platform.
type EmailProviderRequest struct {
	Kind EmailProvider `json:"kind"`
}

// EmailProviderSet answers PUT /orgs/:org/email/provider.
type EmailProviderSet struct {
	Provider EmailProvider `json:"provider"`
	Org      string        `json:"org"`
}

// SenderProvider is what the platform's own mail goes through.
type SenderProvider string

const (
	SenderResend SenderProvider = "resend"
	SenderSMTP   SenderProvider = "smtp"
	SenderSES    SenderProvider = "ses"
	SenderNone   SenderProvider = "none"
)

// EmailSender is how the platform's own mail leaves (CONTROL-PLANE.md §6.12): Resend once the
// operator set a key, otherwise what the server's environment configured. The key is never
// part of it. Set is whether the operator set a Resend key; Allowance is what Resend's plan
// allows and how much of it is used.
type EmailSender struct {
	Provider  SenderProvider  `json:"provider"`
	Set       bool            `json:"set"`
	From      string          `json:"from"`
	KeyHint   string          `json:"key_hint"`
	Bounces   bool            `json:"bounces"`
	SetBy     string          `json:"set_by"`
	SetAt     *time.Time      `json:"set_at,omitempty"`
	Fallback  string          `json:"fallback"`
	Allowance *EmailAllowance `json:"allowance,omitempty"`
	Receiving *EmailReceiving `json:"receiving,omitempty"`
}

// EmailReceiving is the platform's receiving domain, where apps get mail, in the operator's
// Resend account: the records to publish at the domain's DNS provider and whether Resend has
// verified them. Whisk registers the domain with the key already set.
type EmailReceiving struct {
	Domain   string      `json:"domain"`
	Verified bool        `json:"verified"`
	Records  []DNSRecord `json:"records"`
}

// EmailAllowance is how much of the Resend plan's sending allowance is used: today and this
// month, the recipients Resend refused today because the allowance was spent, and when Resend
// last answered its own figures. Set is whether the operator set the limits; otherwise they are
// Resend's free plan.
type EmailAllowance struct {
	Daily        EmailAllowancePeriod `json:"daily"`
	Monthly      EmailAllowancePeriod `json:"monthly"`
	RefusedToday int                  `json:"refused_today"`
	CheckedAt    *time.Time           `json:"checked_at,omitempty"`
	Set          bool                 `json:"set"`
	SetBy        string               `json:"set_by,omitempty"`
	SetAt        *time.Time           `json:"set_at,omitempty"`
}

// EmailAllowancePeriod is one period of the allowance. Used is the larger of what the platform
// handed Resend in the period and what Resend last said it counted; Limit 0 is no limit.
type EmailAllowancePeriod struct {
	Used     int       `json:"used"`
	Limit    int       `json:"limit"`
	ResetsAt time.Time `json:"resets_at"`
}

// BucketProvider is whose bucket an org's files are in: Whisk's own (hetzner) or one the
// business brought (byo).
type BucketProvider string

const (
	BucketPlatform BucketProvider = "hetzner"
	BucketBYO      BucketProvider = "byo"
)

// StorageInfo is GET /orgs/:org/storage: where the org's files are and how much of the plan is
// used. OwnBucketIncluded is whether the plan lets an owner bring a bucket of its own; a business
// already on its own bucket keeps it and may replace its keys on any plan.
type StorageInfo struct {
	Provider          BucketProvider `json:"provider"`
	Endpoint          string         `json:"endpoint"`
	Region            string         `json:"region"`
	Bucket            string         `json:"bucket"`
	UsedBytes         int64          `json:"used_bytes"`
	LimitBytes        int64          `json:"limit_bytes"`
	OwnBucketIncluded bool           `json:"own_bucket_included"`
}

// StorageProviderRequest is PUT /orgs/:org/storage/provider: a bucket of the business's own.
type StorageProviderRequest struct {
	Kind      BucketProvider `json:"kind"`
	Endpoint  string         `json:"endpoint"`
	Region    string         `json:"region"`
	Bucket    string         `json:"bucket"`
	AccessKey string         `json:"access_key"`
	SecretKey string         `json:"secret_key"`
}

// StorageProviderSet answers PUT /orgs/:org/storage/provider: where the files are now.
type StorageProviderSet struct {
	Provider BucketProvider `json:"provider"`
	Endpoint string         `json:"endpoint"`
	Region   string         `json:"region"`
	Bucket   string         `json:"bucket"`
}

// AppStorage is what one app holds and where its business stands against the plan: the app's
// database (previews' included), files and code (its git repository), and the business's total
// of all three, which is what the plan's storage allowance counts.
type AppStorage struct {
	DatabaseBytes int64 `json:"database_bytes"`
	FilesBytes    int64 `json:"files_bytes"`
	RepoBytes     int64 `json:"repo_bytes"`
	UsedBytes     int64 `json:"used_bytes"`
	OrgUsedBytes  int64 `json:"org_used_bytes"`
	LimitBytes    int64 `json:"limit_bytes"`
}

// UploadStatus is what the scan made of an upload.
type UploadStatus string

const (
	UploadAuthorised UploadStatus = "authorised"
	UploadClean      UploadStatus = "clean"
	UploadInfected   UploadStatus = "infected"
	UploadUnscanned  UploadStatus = "unscanned"
	UploadMissing    UploadStatus = "missing"
)

// Visibility is who may read an upload: the app alone, or anyone with its address.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

// Upload is one object the platform authorised into the org's bucket, and what the scan made of
// it (CONTROL-PLANE.md §6.13).
type Upload struct {
	ID           string       `json:"id"`
	Key          string       `json:"key"`
	Filename     string       `json:"filename"`
	Bytes        int64        `json:"bytes"`
	ContentType  string       `json:"content_type"`
	Status       UploadStatus `json:"status"`
	Detail       string       `json:"detail,omitempty"`
	AuthorisedBy string       `json:"authorised_by"`
	AuthorisedAt time.Time    `json:"authorised_at"`
	ScannedAt    time.Time    `json:"scanned_at,omitzero"`
	Visibility   Visibility   `json:"visibility"`
	Media        *UploadMedia `json:"media,omitempty"`
	Image        *UploadImage `json:"image,omitempty"`
	URL          string       `json:"url,omitempty"`
}

// MediaStatus is where a video or audio upload's conversion stands.
type MediaStatus string

const (
	MediaPending    MediaStatus = "pending"
	MediaConverting MediaStatus = "converting"
	MediaReady      MediaStatus = "ready"
	MediaFailed     MediaStatus = "failed"
	MediaHeld       MediaStatus = "held"
)

// MediaKind is whether a media upload is a video or audio.
type MediaKind string

const (
	MediaVideo MediaKind = "video"
	MediaAudio MediaKind = "audio"
)

// UploadMedia is where a video or audio upload's conversion stands and where it plays. Path is
// the playback address on any of the app's hostnames; URL and EmbedURL are it on the app's
// production hostname, and HTML the player element a page puts it in with.
type UploadMedia struct {
	Status     MediaStatus `json:"status"`
	Detail     string      `json:"detail,omitempty"`
	Kind       MediaKind   `json:"kind"`
	DurationMS int64       `json:"duration_ms,omitempty"`
	Width      int         `json:"width,omitempty"`
	Height     int         `json:"height,omitempty"`
	Outputs    []string    `json:"outputs"`
	Path       string      `json:"path"`
	URL        string      `json:"url,omitempty"`
	EmbedURL   string      `json:"embed_url,omitempty"`
	HTML       string      `json:"html"`
}

// UploadImage is where an image upload is shown in any size: Path on any of the app's hostnames
// (add ?w=, &h=, &fit=cover), URL on its production one, and Detail why the file could not be
// read as an image.
type UploadImage struct {
	Path   string `json:"path"`
	URL    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// MediaLink is one signed link, for the address asked for: Path on any of the app's hostnames
// but its previews, URL on its production one.
type MediaLink struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url,omitempty"`
}

// MediaLinks answers POST …/uploads/links: the links, in the order asked, and when they stop
// working.
type MediaLinks struct {
	ExpiresAt time.Time   `json:"expires_at"`
	Links     []MediaLink `json:"links"`
}

// MediaLinkKey is where an app's media-link signing key stands after a rotation: the
// generation links are now signed with, and until when links signed before still work.
type MediaLinkKey struct {
	Generation    int       `json:"generation"`
	RotatedAt     time.Time `json:"rotated_at"`
	PreviousUntil time.Time `json:"previous_until"`
}

// PackageSeverity is how serious a known vulnerability is.
type PackageSeverity string

const (
	SeverityCritical PackageSeverity = "critical"
	SeverityHigh     PackageSeverity = "high"
	SeverityMedium   PackageSeverity = "medium"
	SeverityLow      PackageSeverity = "low"
	SeverityUnknown  PackageSeverity = "unknown"
)

// PackageWhere is where a vulnerable package was found: in the built image, or in a lockfile in
// the commit.
type PackageWhere string

const (
	FoundInImage  PackageWhere = "image"
	FoundInSource PackageWhere = "source"
)

// PackageFinding is one known vulnerability in one installed package. Fixed is the lowest version
// that fixes it, above the installed one; empty when there is no fix yet. Path is the file inside
// the image or the lockfile in the commit.
type PackageFinding struct {
	ID        string          `json:"id"`
	Aliases   []string        `json:"aliases,omitempty"`
	Package   string          `json:"package"`
	Ecosystem string          `json:"ecosystem,omitempty"`
	Version   string          `json:"version"`
	Fixed     string          `json:"fixed,omitempty"`
	Severity  PackageSeverity `json:"severity"`
	Title     string          `json:"title,omitempty"`
	URL       string          `json:"url,omitempty"`
	Where     PackageWhere    `json:"where"`
	Path      string          `json:"path,omitempty"`
	Tool      string          `json:"tool"`
}

// PackageCounts is how many findings there are of each severity, how many have a fix, and how
// many want attention now.
type PackageCounts struct {
	Critical  int `json:"critical"`
	High      int `json:"high"`
	Medium    int `json:"medium"`
	Low       int `json:"low"`
	Unknown   int `json:"unknown"`
	Fixable   int `json:"fixable"`
	Attention int `json:"attention"`
}

// Total is every finding.
func (c PackageCounts) Total() int { return c.Critical + c.High + c.Medium + c.Low + c.Unknown }

// ScanStatus is where a check of an app's packages stands.
type ScanStatus string

const (
	ScanQueued  ScanStatus = "queued"
	ScanRunning ScanStatus = "running"
	ScanDone    ScanStatus = "done"
	ScanFailed  ScanStatus = "failed"
	ScanSkipped ScanStatus = "skipped"
)

// ScanTrigger is what started a check of an app's packages.
type ScanTrigger string

const (
	ScanOnDeploy  ScanTrigger = "deploy"
	ScanDaily     ScanTrigger = "daily"
	ScanOnRequest ScanTrigger = "request"
)

// PackageScan is one check of an app's packages. Findings are only on a finished scan.
type PackageScan struct {
	ID          string           `json:"id"`
	Status      ScanStatus       `json:"status"`
	Trigger     ScanTrigger      `json:"trigger"`
	BuildID     string           `json:"build_id,omitempty"`
	CommitSHA   string           `json:"commit_sha,omitempty"`
	ImageDigest string           `json:"image_digest,omitempty"`
	Error       string           `json:"error,omitempty"`
	Counts      PackageCounts    `json:"counts"`
	Findings    []PackageFinding `json:"findings,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	StartedAt   time.Time        `json:"started_at,omitzero"`
	FinishedAt  time.Time        `json:"finished_at,omitzero"`
}

// Packages is GET /orgs/:org/apps/:app/packages: whether the plan includes scanning, the newest
// finished scan with its findings, and the check waiting or running, if any.
type Packages struct {
	Included bool         `json:"included"`
	Scan     *PackageScan `json:"scan"`
	Pending  *PackageScan `json:"pending,omitempty"`
}

// LogDestinationKind is the kind of service a business forwards its logs to.
type LogDestinationKind string

const (
	LogToHTTPS   LogDestinationKind = "https"
	LogToDatadog LogDestinationKind = "datadog"
	LogToOTLP    LogDestinationKind = "otlp"
)

// LogDestinationStatus is how forwarding to a destination is going: lines have gone, nothing to
// send yet, the last batch was refused or not answered, or the plan does not include it.
type LogDestinationStatus string

const (
	LogSending LogDestinationStatus = "sending"
	LogWaiting LogDestinationStatus = "waiting"
	LogFailing LogDestinationStatus = "failing"
	LogStopped LogDestinationStatus = "stopped"
)

// LogDestination is one of an org's own log services: where it sends, the names of the headers
// it sends (never their values) and how sending is going.
type LogDestination struct {
	ID           string               `json:"id"`
	Kind         LogDestinationKind   `json:"kind"`
	URL          string               `json:"url,omitempty"`
	Site         string               `json:"site,omitempty"`
	Where        string               `json:"where"`
	Headers      []string             `json:"headers"`
	Status       LogDestinationStatus `json:"status"`
	LinesSent    int64                `json:"lines_sent"`
	LastSentAt   time.Time            `json:"last_sent_at,omitzero"`
	FailingSince time.Time            `json:"failing_since,omitzero"`
	LastError    string               `json:"last_error,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
}

// LogDestinationList is GET /orgs/:org/logs/destinations: the destinations, whether the plan
// includes forwarding, and what may be added.
type LogDestinationList struct {
	Items    []LogDestination     `json:"items"`
	Included bool                 `json:"included"`
	Limit    int                  `json:"limit"`
	Kinds    []LogDestinationKind `json:"kinds"`
	Sites    []string             `json:"datadog_sites"`
}

// LogDestinationRequest is POST /orgs/:org/logs/destinations: {kind, url, headers} for https or
// otlp, or {kind: datadog, site, api_key}.
type LogDestinationRequest struct {
	Kind    LogDestinationKind `json:"kind"`
	URL     string             `json:"url,omitempty"`
	Site    string             `json:"site,omitempty"`
	Headers map[string]string  `json:"headers,omitempty"`
	APIKey  string             `json:"api_key,omitempty"`
}

// LogLine is one log line. Unit and Host are set on the platform's own lines.
type LogLine struct {
	At     time.Time      `json:"at"`
	Line   string         `json:"line"`
	Stream string         `json:"stream"`
	Env    string         `json:"env"`
	Unit   string         `json:"unit,omitempty"`
	Host   string         `json:"host,omitempty"`
	JSON   map[string]any `json:"json,omitempty"`
}

// LogPage is GET /orgs/:org/apps/:app/logs without stream=true: the lines alone. Nothing of the
// log store's own query language or labels reaches a tenant.
type LogPage struct {
	Items []LogLine `json:"items"`
}

// PlatformLogPage is the operator's GET /operator/logs without stream=true: the platform's own
// lines and the LogQL query they were read with, for Whisk's own staff.
type PlatformLogPage struct {
	Items []LogLine `json:"items"`
	Query string    `json:"query"`
}

// ErrorGroup is one error group of an app.
type ErrorGroup struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Culprit   string    `json:"culprit,omitempty"`
	Level     string    `json:"level,omitempty"`
	Status    string    `json:"status"`
	Count     int       `json:"count"`
	Users     int       `json:"users"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// ErrorSample is one recorded occurrence of a group: the frames a person needs to find the line
// that threw, and the request that was in flight when it did.
type ErrorSample struct {
	ID      string         `json:"id"`
	At      time.Time      `json:"at"`
	Message string         `json:"message,omitempty"`
	Frames  []ErrorFrame   `json:"frames"`
	Tags    map[string]any `json:"tags,omitempty"`
}

// ErrorFrame is one line of a stack, innermost last, as a stack trace reads.
type ErrorFrame struct {
	Function string `json:"function,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	InApp    bool   `json:"in_app"`
}

// TraceSummary is one trace in the list, named by its entry span. A trace's id is 32 lowercase
// hex; RequestID is the same 16 bytes as the edge's ULID, so a log line's request_id opens its
// trace.
type TraceSummary struct {
	TraceID    string    `json:"trace_id"`
	RequestID  string    `json:"request_id"`
	Env        string    `json:"env"`
	Name       string    `json:"name"`
	Method     string    `json:"method,omitempty"`
	Path       string    `json:"path,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS float64   `json:"duration_ms"`
	Spans      int       `json:"spans"`
	Errors     int       `json:"errors"`
}

// TraceUsage is the app's spans kept today against its allowance.
type TraceUsage struct {
	SpansToday int64 `json:"spans_today"`
	Limit      int64 `json:"limit"`
}

// TraceList is GET /orgs/:org/apps/:app/traces: the traces, how long they are kept and today's
// usage.
type TraceList struct {
	Items         []TraceSummary `json:"items"`
	RetentionDays int            `json:"retention_days"`
	Usage         TraceUsage     `json:"usage"`
}

// TraceSpan is one span of a trace. Attribute values are strings, numbers, booleans or lists of
// them, as OpenTelemetry has them.
type TraceSpan struct {
	SpanID        string         `json:"span_id"`
	ParentSpanID  string         `json:"parent_span_id"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Service       string         `json:"service"`
	StartedAt     time.Time      `json:"started_at"`
	OffsetMS      float64        `json:"offset_ms"`
	DurationMS    float64        `json:"duration_ms"`
	Status        string         `json:"status"`
	StatusMessage string         `json:"status_message"`
	Attributes    map[string]any `json:"attributes"`
	Events        []TraceEvent   `json:"events"`
}

// TraceEvent is one event of a span, placed on the trace's clock.
type TraceEvent struct {
	Name       string         `json:"name"`
	OffsetMS   float64        `json:"offset_ms"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// TraceDetail is one trace's spans, ordered by start.
type TraceDetail struct {
	TraceID    string      `json:"trace_id"`
	RequestID  string      `json:"request_id"`
	Env        string      `json:"env"`
	StartedAt  time.Time   `json:"started_at"`
	DurationMS float64     `json:"duration_ms"`
	Spans      []TraceSpan `json:"spans"`
	Truncated  bool        `json:"truncated"`
}

// CustomerStatus is where one of a tenant's own users stands.
type CustomerStatus string

const (
	CustomerInvited CustomerStatus = "invited"
	CustomerActive  CustomerStatus = "active"
	CustomerBlocked CustomerStatus = "blocked"
)

// Customer is one of a tenant's own users (CONTROL-PLANE.md §4.6). The invite URL is returned
// once, when the invitation is made, because it is the way in.
type Customer struct {
	ID         string         `json:"id"`
	Email      string         `json:"email"`
	Name       string         `json:"name,omitempty"`
	Status     CustomerStatus `json:"status"`
	InvitedBy  string         `json:"invited_by,omitempty"`
	InviteURL  string         `json:"invite_url,omitempty"`
	LastSeenAt time.Time      `json:"last_seen_at,omitzero"`
	CreatedAt  time.Time      `json:"created_at"`
}

// CustomerList is a pool's people and whether a stranger may register themselves.
type CustomerList struct {
	Items  []Customer `json:"items"`
	Signup bool       `json:"signup"`
}

// WebhookSource is a webhook source: the URL to paste into the provider, how deliveries are
// verified, and how the last hundred went.
type WebhookSource struct {
	Name        string    `json:"name"`
	Preset      string    `json:"preset"`
	URL         string    `json:"url"`
	Handler     string    `json:"handler"`
	Secret      string    `json:"secret"`
	SecretSet   bool      `json:"secret_set"`
	IPAllowlist []string  `json:"ip_allowlist"`
	Events      int       `json:"events"`
	Unverified  float64   `json:"unverified_rate"`
	LastEventAt time.Time `json:"last_event_at,omitzero"`
}

// DeliveryStatus is where handing a webhook event to the app stands. Skipped is an unverified
// event, which is stored and never delivered.
type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryDead      DeliveryStatus = "dead"
	DeliverySkipped   DeliveryStatus = "skipped"
)

// WebhookEvent is one stored delivery. Body is the raw payload as text, because a person looking
// at a delivery wants to see exactly what arrived. LastError is why the latest delivery attempt
// failed.
type WebhookEvent struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	ReceivedAt time.Time         `json:"received_at"`
	Verified   bool              `json:"verified"`
	Reason     string            `json:"reason,omitempty"`
	Status     DeliveryStatus    `json:"delivery_status"`
	Attempts   int               `json:"attempts"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	LastError  *DeliveryError    `json:"last_error,omitempty"`
}

// DeliveryError is a failed attempt to hand a webhook event to the app's handler: the handler's
// HTTP status and the start of its answer, or the connection error (status 0).
type DeliveryError struct {
	Status int    `json:"status"`
	Error  string `json:"error"`
}

// InboxDomainStatus is where a business's own receiving domain stands: pending until the
// provider sees its records, then verified, when its mail reaches the app.
type InboxDomainStatus string

const (
	InboxDomainPending  InboxDomainStatus = "pending"
	InboxDomainVerified InboxDomainStatus = "verified"
)

// InboxDomain is a business's own domain whose mail reaches one app (CONTROL-PLANE.md §6.12
// "Receiving"), with the records to publish while it waits.
type InboxDomain struct {
	ID         string            `json:"id"`
	Domain     string            `json:"domain"`
	Status     InboxDomainStatus `json:"status"`
	DNSRecords []DNSRecord       `json:"dns_records"`
	CreatedAt  time.Time         `json:"created_at"`
	VerifiedAt time.Time         `json:"verified_at,omitzero"`
}

// InboxDomainRequest is POST /orgs/:org/apps/:app/inbox/domains.
type InboxDomainRequest struct {
	Domain string `json:"domain"`
}

// InboxMessage is one message an inbox received, as a list shows it: the event it is stored as
// (whisk webhooks events inbox lists the same), who sent it, and how its delivery went. Reason
// says why a message was dropped rather than delivered.
type InboxMessage struct {
	ID          string         `json:"id"`
	ReceivedAt  time.Time      `json:"received_at"`
	From        string         `json:"from"`
	Recipient   string         `json:"recipient"`
	Subject     string         `json:"subject"`
	Attachments int            `json:"attachments"`
	Status      DeliveryStatus `json:"status"`
	Reason      string         `json:"reason,omitempty"`
	Attempts    int            `json:"attempts"`
}

// Inbox is GET /orgs/:org/apps/:app/inbox: the app's address and its own domains when it
// declares an inbox, whether the platform can receive mail now, and the latest messages.
type Inbox struct {
	Declared  bool `json:"declared"`
	Available bool `json:"available"`
	// Included is whether the business's plan includes an inbox (Team and above); on one that
	// does not, every message is dropped as not_on_plan.
	Included  bool           `json:"included"`
	Address   string         `json:"address"`
	Handler   string         `json:"handler"`
	AllowFrom []string       `json:"allow_from"`
	Domains   []InboxDomain  `json:"domains"`
	Recent    []InboxMessage `json:"recent"`
}
