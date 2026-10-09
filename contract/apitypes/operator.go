package apitypes

import (
	"time"

	"github.com/whisk-run/contract/status"
)

// The operator's screens (CONTROL-PLANE.md §6.20, §6.23, §6.27): abuse holds and signals,
// feedback, harness runs, revenue, sign-ups, break-glass reads and node drains.

// HoldStatus is where an abuse hold stands: held waits for the operator, released is one they
// found wrong, removed one they upheld.
type HoldStatus string

const (
	HoldHeld     HoldStatus = "held"
	HoldReleased HoldStatus = "released"
	HoldRemoved  HoldStatus = "removed"
)

// HoldKind is what found the app, or linked for a business held because its person, address or
// computer was already held.
type HoldKind string

const (
	HoldPhishingPage HoldKind = "phishing_page"
	HoldBlocklist    HoldKind = "blocklist"
	HoldWebRisk      HoldKind = "web_risk"
	HoldEmailLink    HoldKind = "email_link"
	HoldLinked       HoldKind = "linked"
)

// Hold is one abuse hold on the operator's list (CONTROL-PLANE.md §6.27).
type Hold struct {
	ID        string         `json:"id"`
	Org       string         `json:"org"`
	OrgName   string         `json:"org_name"`
	App       string         `json:"app,omitempty"`
	AppName   string         `json:"app_name,omitempty"`
	Hostname  string         `json:"hostname,omitempty"`
	Person    string         `json:"person,omitempty"`
	IPs       []string       `json:"ips"`
	Devices   int            `json:"devices"`
	Kind      HoldKind       `json:"kind"`
	Detail    map[string]any `json:"detail"`
	ParentID  string         `json:"parent_id,omitempty"`
	Status    HoldStatus     `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	DecidedAt *time.Time     `json:"decided_at,omitempty"`
}

// HoldDecision answers a release or a removal with the holds it decided.
type HoldDecision struct {
	Status HoldStatus `json:"status"`
	Holds  []string   `json:"holds"`
}

// WebRisk is whether the Google Web Risk key is set, and its last four. SetAt is the zero time
// while none is.
type WebRisk struct {
	Set   bool      `json:"set"`
	Hint  string    `json:"hint,omitempty"`
	SetAt time.Time `json:"set_at"`
}

// WebRiskRequest is PUT /operator/abuse/web-risk.
type WebRiskRequest struct {
	APIKey string `json:"api_key"`
}

// AbuseKind is a kind of abuse signal.
type AbuseKind string

const (
	AbuseEgress           AbuseKind = "egress"
	AbuseEmailThrottled   AbuseKind = "email_throttled"
	AbuseCPUPegged        AbuseKind = "cpu_pegged"
	AbuseRunGuard         AbuseKind = "run_guard"
	AbuseDisposableSignup AbuseKind = "disposable_signup"
)

// AbuseFlag is one org's signals of one kind over the last week.
type AbuseFlag struct {
	OrgID   string         `json:"org_id"`
	Org     string         `json:"org"`
	Name    string         `json:"name"`
	Kind    AbuseKind      `json:"kind"`
	Count   int            `json:"count"`
	FirstAt time.Time      `json:"first_at"`
	LastAt  time.Time      `json:"last_at"`
	Detail  map[string]any `json:"detail"`
}

// FeedbackKind is what a piece of feedback is (CONTROL-PLANE.md §6.23).
type FeedbackKind string

const (
	FeedbackBug        FeedbackKind = "bug"
	FeedbackDifficulty FeedbackKind = "difficulty"
	FeedbackIdea       FeedbackKind = "idea"
	FeedbackPraise     FeedbackKind = "praise"
)

// FeedbackStatus is whether the Whisk team has resolved a piece of feedback.
type FeedbackStatus string

const (
	FeedbackOpen     FeedbackStatus = "open"
	FeedbackResolved FeedbackStatus = "resolved"
)

// FeedbackSender is who sent a piece of feedback: a person, an agent token, or nobody signed in.
type FeedbackSender string

const (
	SenderUser      FeedbackSender = "user"
	SenderAgent     FeedbackSender = "agent"
	SenderAnonymous FeedbackSender = "anonymous"
)

// Feedback is one piece of feedback as the operator reads it. Note is what the Whisk team told
// the sender when it resolved or reopened it.
type Feedback struct {
	ID         string            `json:"id"`
	Kind       FeedbackKind      `json:"kind"`
	Message    string            `json:"message"`
	Status     FeedbackStatus    `json:"status"`
	Org        string            `json:"org,omitempty"`
	App        string            `json:"app,omitempty"`
	ActorKind  FeedbackSender    `json:"actor_kind"`
	Email      string            `json:"email,omitempty"`
	Client     string            `json:"client,omitempty"`
	Context    map[string]string `json:"context"`
	Redacted   int               `json:"redacted"`
	CreatedAt  time.Time         `json:"created_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	Note       string            `json:"note,omitempty"`
}

// FeedbackPage is a page of feedback with how much is still open.
type FeedbackPage struct {
	Items      []Feedback `json:"items"`
	NextCursor string     `json:"next_cursor"`
	Open       int        `json:"open"`
}

// OwnFeedback is one piece of feedback as its sender reads it: what they said, where it stands
// and the Whisk team's note, without the address, client or other people's emails. SentByYou is
// whether the caller's person, or the calling token, sent it.
type OwnFeedback struct {
	ID         string            `json:"id"`
	Kind       FeedbackKind      `json:"kind"`
	Message    string            `json:"message"`
	Status     FeedbackStatus    `json:"status"`
	Org        string            `json:"org,omitempty"`
	App        string            `json:"app,omitempty"`
	SentByYou  bool              `json:"sent_by_you"`
	Context    map[string]string `json:"context"`
	Redacted   int               `json:"redacted"`
	CreatedAt  time.Time         `json:"created_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	Note       string            `json:"note,omitempty"`
}

// OwnFeedbackPage is a page of the caller's own feedback.
type OwnFeedbackPage struct {
	Items      []OwnFeedback `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

// FeedbackReceipt is what the sender of feedback gets back: the id, and the thanks.
type FeedbackReceipt struct {
	ID        string       `json:"id"`
	Kind      FeedbackKind `json:"kind"`
	Org       string       `json:"org,omitempty"`
	App       string       `json:"app,omitempty"`
	Redacted  int          `json:"redacted"`
	CreatedAt time.Time    `json:"created_at"`
	Message   string       `json:"message"`
}

// CanaryRun is one harness run as the operator lists it.
type CanaryRun struct {
	RunID      string           `json:"run_id"`
	Target     string           `json:"target"`
	Mode       string           `json:"mode"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	Passed     int              `json:"passed"`
	Failed     int              `json:"failed"`
	Blocked    int              `json:"blocked"`
	Flaky      int              `json:"flaky"`
	Failures   []HarnessFailure `json:"failures"`
}

// HarnessFailure is one scenario a harness run failed.
type HarnessFailure struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Assertion string `json:"assertion,omitempty"`
}

// Revenue is GET /operator/revenue.
type Revenue struct {
	Period            string        `json:"period"`
	ByPlan            []PlanRevenue `json:"by_plan"`
	MonthlyCents      int64         `json:"monthly_cents"`
	InvoicesPaid      int           `json:"invoices_paid"`
	InvoicesPaidCents int64         `json:"invoices_paid_cents"`
	SignupsWeek       int           `json:"signups_week"`
	ConfirmedWeek     int           `json:"confirmed_week"`
}

// PlanRevenue is what the orgs on one plan bring in a month.
type PlanRevenue struct {
	Plan         string `json:"plan"`
	Orgs         int    `json:"orgs"`
	MonthlyCents int64  `json:"monthly_cents"`
}

// Signup is a person who signed up: their first sign-in to Whisk (CONTROL-PLANE.md §6.20).
type Signup struct {
	ID         string      `json:"id"`
	Email      string      `json:"email"`
	Name       string      `json:"name"`
	SignedUpAt time.Time   `json:"signed_up_at"`
	Orgs       []SignupOrg `json:"orgs"`
}

// SignupOrg is an org a new person has made or joined, with their role in it.
type SignupOrg struct {
	Slug   string     `json:"slug"`
	Name   string     `json:"name"`
	Status status.Org `json:"status"`
	Role   Role       `json:"role"`
}

// BrokenGlass is POST /operator/orgs/:org/breakglass: one secret's current value.
type BrokenGlass struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Value   string `json:"value"`
}

// DrainResult is what POST /operator/nodes/:id/drain answers: the node, now draining, and the
// deploy queued for each environment that is leaving it (CONTROL-PLANE.md §6.6).
type DrainResult struct {
	Node  Node        `json:"node"`
	Moves []DrainMove `json:"moves"`
}

// DrainMove is one environment on its way to another node.
type DrainMove struct {
	DeployID      string `json:"deploy_id"`
	AppID         string `json:"app_id"`
	EnvironmentID string `json:"environment_id"`
	BuildID       string `json:"build_id"`
	FromNodeID    string `json:"from_node_id"`
	ToNodeID      string `json:"to_node_id"`
}
