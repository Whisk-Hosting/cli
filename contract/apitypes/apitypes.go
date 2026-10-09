// Package apitypes is the JSON the Whisk API answers with and accepts (CONTROL-PLANE.md §5): the
// one definition the control plane serves, the CLI decodes and the dashboard's TypeScript is
// generated from (typescript.go). Times are RFC 3339; a field marked omitempty is absent when
// empty.
package apitypes

import (
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/status"
)

// Page wraps every list: items and the cursor for the next page ("" at the end).
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

// Org is an organisation. Role is the caller's role when they are a member.
type Org struct {
	ID          string         `json:"id"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Plan        string         `json:"plan"`
	Status      status.Org     `json:"status"` // pending | active | frozen | shredding | shredded
	Region      string         `json:"region"`
	Role        Role           `json:"role,omitempty"`
	Kind        MemberKind     `json:"kind,omitempty"` // for the caller
	ConfirmedAt *time.Time     `json:"confirmed_at,omitempty"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"` // pending orgs
	Settings    map[string]any `json:"settings"`
	Limits      map[string]any `json:"limits"` // the plan's limits
	CreatedAt   time.Time      `json:"created_at"`
	// Timeline is where the org stands on the payment timeline (CONTROL-PLANE.md §6.15).
	Timeline Timeline `json:"timeline"`
	// DataKey says whether the org still holds its data key; the operator's list carries it.
	DataKey *bool `json:"data_key,omitempty"`
	// Comp is the org's comp while it is in force (CONTROL-PLANE.md §6.15); the operator's list
	// and the comp routes carry it.
	Comp *Comp `json:"comp,omitempty"`
	// Support is the caller's open support access, when that is how they are reading the org.
	Support *Support `json:"support,omitempty"`
	// DemoFor is the company email domain of a demo business nobody has claimed yet
	// (CONTROL-PLANE.md §6.1).
	DemoFor string `json:"demo_for,omitempty"`
	// UnderReview is set while an abuse hold waits for the operator (CONTROL-PLANE.md §6.27).
	UnderReview bool `json:"under_review,omitempty"`
	// Agency is set on a business with client businesses (CONTROL-PLANE.md §6.15), which has the
	// all-clients screen.
	Agency bool `json:"agency,omitempty"`
}

// User is a person.
type User struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	IsOperator bool   `json:"is_operator,omitempty"`
}

// Member is a membership with the person.
type Member struct {
	ID        string       `json:"id"`
	UserID    string       `json:"user_id"`
	Email     string       `json:"email"`
	Name      string       `json:"name"`
	Role      Role         `json:"role"`
	Kind      MemberKind   `json:"kind"`
	Status    MemberStatus `json:"status"`
	Groups    []string     `json:"groups"` // group IDs
	InvitedBy string       `json:"invited_by,omitempty"`
	InviteURL string       `json:"invite_url,omitempty"` // while invited: the link the person redeems
	CreatedAt time.Time    `json:"created_at"`
}

// Group is a named set of members.
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Members   []string  `json:"members"` // user IDs
	CreatedAt time.Time `json:"created_at"`
}

// App is an application. Status is the stored status; State is the human word the
// dashboard shows (DASHBOARD.md §7): new | running | sleeping | starting | broken | blocked.
type App struct {
	ID                 string     `json:"id"`
	OrgID              string     `json:"org_id"`
	Slug               string     `json:"slug"`
	Name               string     `json:"name"`
	Hostname           string     `json:"hostname"`
	URL                string     `json:"url"`
	Region             string     `json:"region"`
	Status             status.App `json:"status"`
	State              AppState   `json:"state"`
	AlwaysOn           bool       `json:"always_on"`
	ConventionsVersion int        `json:"conventions_version"`
	CurrentDeployID    string     `json:"current_deploy_id,omitempty"`
	GitURL             string     `json:"git_url"`
	NodeID             string     `json:"node_id,omitempty"`
	UnsetSecrets       []string   `json:"unset_secrets"` // declared by the manifest, no value yet
	Memory             *AppMemory `json:"memory,omitempty"`
	Start              *AppStart  `json:"start,omitempty"`
	LastDeployAt       *time.Time `json:"last_deploy_at,omitempty"`
	// Problem is why the app is broken (its container exited after going live) or why its
	// last wake in the past day failed; absent when neither (CONTROL-PLANE.md §6.5).
	Problem *AppProblem `json:"problem,omitempty"`
	// Promoted is set while the app is a Promoted app, absent when it is not one
	// (CONTROL-PLANE.md §6.15).
	Promoted  *AppPromoted `json:"promoted,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// AppPromoted is when an app was made a Promoted app and by whom (a user id), and whether it is
// in force: false while the business's plan has no price for Promoted apps, when the app runs
// like any other and nothing is charged (CONTROL-PLANE.md §6.15).
type AppPromoted struct {
	Since   time.Time `json:"since"`
	By      string    `json:"by"`
	InForce bool      `json:"in_force"`
}

// AppProblem is the latest thing that stopped the app answering: CONTAINER_CRASHED with the
// exit code and last lines, or WAKE_TIMEOUT / WAKE_FAILED with the node's words.
type AppProblem struct {
	Code     string    `json:"code"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
	ExitCode *int      `json:"exit_code,omitempty"`
	Log      []string  `json:"log,omitempty"`
}

// AppStart is the app's typical start: the median time its production container took to answer
// its health check over its last 10 starts (wakes and deploys), and how many starts that was.
// Absent until one is recorded (CONTROL-PLANE.md §6.5).
type AppStart struct {
	TypicalMS int64 `json:"typical_ms"`
	Count     int   `json:"count"`
}

// AppMemory is the app's memory as its node last reported it: the plan's limit, what the
// running container uses now, the most it used since it last started (files in /tmp count),
// and when it was last killed for going over the limit.
type AppMemory struct {
	LimitBytes        int64      `json:"limit_bytes"`
	UsedBytes         int64      `json:"used_bytes"`
	PeakBytes         int64      `json:"peak_bytes"`
	LastOutOfMemoryAt *time.Time `json:"last_out_of_memory_at,omitempty"`
}

// Environment is production or a preview.
type Environment struct {
	ID              string     `json:"id"`
	AppID           string     `json:"app_id"`
	Name            string     `json:"name"` // production | preview:<branch>
	Hostname        string     `json:"hostname"`
	URL             string     `json:"url"`
	HasDatabase     bool       `json:"has_database"`
	CurrentDeployID string     `json:"current_deploy_id,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	// Status is active or sleeping; a preview sleeps on its own (§6.5), production's follows
	// the app's state.
	Status    EnvironmentStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
}

// Build is one image build.
type Build struct {
	ID            string          `json:"id"`
	AppID         string          `json:"app_id"`
	EnvironmentID string          `json:"environment_id"`
	CommitSHA     string          `json:"commit_sha"`
	Branch        string          `json:"branch"`
	Message       string          `json:"message"` // the commit's subject line; "" when it had none
	Status        BuildStatus     `json:"status"`
	Builder       string          `json:"builder"` // dockerfile | railpack
	ImageDigest   string          `json:"image_digest,omitempty"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	Error         *werrors.Detail `json:"error,omitempty"`
	TriggeredBy   string          `json:"triggered_by"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Phase is one recorded deploy transition.
type Phase struct {
	Phase  string    `json:"phase"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}

// Deploy is one attempt to make a build live.
type Deploy struct {
	ID               string          `json:"id"`
	AppID            string          `json:"app_id"`
	EnvironmentID    string          `json:"environment_id"`
	Environment      string          `json:"environment"`
	BuildID          string          `json:"build_id"`
	CommitSHA        string          `json:"commit_sha"`
	Branch           string          `json:"branch"`
	Message          string          `json:"message"` // what changed: the build's commit subject line
	Status           status.Deploy   `json:"status"`
	Phases           []Phase         `json:"phases"`
	Error            *werrors.Detail `json:"error,omitempty"`
	NodeID           string          `json:"node_id,omitempty"`
	PreviousDeployID string          `json:"previous_deploy_id,omitempty"`
	TriggeredBy      string          `json:"triggered_by"`       // user:<id> | agent:<token id> | system
	TriggeredByLabel string          `json:"triggered_by_label"` // "Mathias", "Claude Code for Mathias", "Whisk"
	// AppName, AppSlug and OrgSlug name where the deploy happened on lists that span accounts
	// (the operator's failed deploys); an org's own deploy lists leave them out.
	AppName string `json:"app_name,omitempty"`
	AppSlug string `json:"app_slug,omitempty"`
	OrgSlug string `json:"org_slug,omitempty"`
	// StartMS is how long the deploy's container took to answer its health check, for a
	// production deploy that reached it.
	StartMS *int64 `json:"start_ms,omitempty"`
	// Warnings are what went wrong once the deploy was live without failing it, such as
	// FUNCTIONS_NOT_REGISTERED (CONTROL-PLANE.md §6.5); [] when nothing did.
	Warnings   []Warning  `json:"warnings"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// DeployEvent is one SSE event on GET /deploys/:id/events. The last one of a live deploy
// carries the URL, the secrets still unset and, for production, the deploy's start time.
type DeployEvent struct {
	DeployID string          `json:"deploy_id"`
	Status   status.Deploy   `json:"status"`
	Phase    Phase           `json:"phase"`
	Error    *werrors.Detail `json:"error,omitempty"`
	Done     bool            `json:"done"`
	URL      string          `json:"url,omitempty"`
	Unset    []string        `json:"unset_secrets,omitempty"`
	StartMS  *int64          `json:"start_ms,omitempty"`
	Warnings []Warning       `json:"warnings,omitempty"`
}

// Secret is a declared secret: never its value.
type Secret struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Scope   SecretScope `json:"scope"`
	AppID   string      `json:"app_id,omitempty"`
	Set     bool        `json:"set"`
	Version int         `json:"version"`
	Managed bool        `json:"managed"`
	// Previews says whether preview environments receive the secret; a secret reaches a
	// preview only when a person shares it with previews.
	Previews   bool       `json:"previews"`
	SetBy      string     `json:"set_by,omitempty"`
	SetAt      *time.Time `json:"set_at,omitempty"`
	LastReadAt *time.Time `json:"last_read_at,omitempty"`
	Consumers  []string   `json:"consumers"` // app IDs
	CreatedAt  time.Time  `json:"created_at"`
}

// SecretVersion is one stored value's metadata.
type SecretVersion struct {
	Version   int       `json:"version"`
	Length    int       `json:"length"`
	CreatedBy string    `json:"created_by"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SecretRead is one delivery.
type SecretRead struct {
	Version    int          `json:"version"`
	ReadByKind SecretReader `json:"read_by_kind"`
	ReadByID   string       `json:"read_by_id"`
	ReadAt     time.Time    `json:"read_at"`
}

// Grant lets a subject open an app.
type Grant struct {
	SubjectKind SubjectKind `json:"subject_kind"`
	SubjectID   string      `json:"subject_id,omitempty"`
	Audience    Audience    `json:"audience"`
	Label       string      `json:"label,omitempty"` // the person's email or the group's name
}

// Access is an app's grants.
type Access struct {
	AppID  string  `json:"app_id"`
	Grants []Grant `json:"grants"`
}

// Domain is a hostname of an app.
type Domain struct {
	ID         string     `json:"id"`
	Hostname   string     `json:"hostname"`
	Kind       DomainKind `json:"kind"`
	Verified   bool       `json:"verified"`
	CertStatus string     `json:"cert_status"`
	// CDN is set on a custom domain an operator marked as fronted by a CDN (CADDY.md §3).
	CDN bool `json:"cdn"`
	// RedirectTo is another hostname of the app that this custom domain sends every request to
	// with a permanent redirect (CONTROL-PLANE.md §6.11); absent while it serves the app.
	RedirectTo string `json:"redirect_to,omitempty"`
	// For custom domains awaiting verification.
	TXTRecord   string `json:"txt_record,omitempty"`
	TXTValue    string `json:"txt_value,omitempty"`
	CNAMETarget string `json:"cname_target,omitempty"`
	// Addresses are the app hostname's addresses, for a bare domain (example.com), which
	// cannot have a CNAME: A and AAAA records to them point it at the app instead.
	Addresses []string `json:"addresses,omitempty"`
	// Records are the DNS records to create, ready to show whoever runs the name's DNS: the
	// TXT that proves it, then a CNAME to the app, or A and AAAA records for a bare domain.
	// Empty once the domain is verified, and for every other kind.
	Records []DNSRecord `json:"records,omitempty"`
	// Status is where a custom domain is: waiting for its records, for its certificate, or
	// answering. Every other kind is active.
	Status DomainStatus `json:"status"`
	// AddedBy says who added a custom domain: the business's people and their agents (team)
	// or the app itself with its service token (app), which removes only its own.
	AddedBy   DomainAddedBy `json:"added_by,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

// Token is an agent or deploy token without its value.
type Token struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"`
	Scopes     []string   `json:"scopes"`
	AppID      string     `json:"app_id,omitempty"`
	OnBehalfOf string     `json:"on_behalf_of,omitempty"`
	Bound      bool       `json:"bound"`
	DeviceName string     `json:"device_name,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Session is a whisk.run sign-in.
type Session struct {
	ID         string       `json:"id"`
	Current    bool         `json:"current"`
	Method     SignInMethod `json:"method"`
	IP         string       `json:"ip"`
	UserAgent  string       `json:"user_agent"`
	LastSeenAt time.Time    `json:"last_seen_at"`
	CreatedAt  time.Time    `json:"created_at"`
	ExpiresAt  time.Time    `json:"expires_at"`
}

// Whoami describes the caller.
type Whoami struct {
	User    User         `json:"user"`
	Orgs    []Org        `json:"orgs"`
	Token   *WhoamiToken `json:"token,omitempty"`
	Session *Session     `json:"session,omitempty"`
	// Identity is the agent identity calling, when the token is one's credential: what it may
	// change, read fresh on this request (CONTROL-PLANE.md §4.4).
	Identity *WhoamiIdentity `json:"identity,omitempty"`
}

// WhoamiIdentity is the calling agent identity as whoami shows it.
type WhoamiIdentity struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Categories []string `json:"categories"`
}

// AuditEvent is one audit entry.
type AuditEvent struct {
	ID         string    `json:"id"`
	ActorKind  ActorKind `json:"actor_kind"`
	ActorID    string    `json:"actor_id"`
	ActorLabel string    `json:"actor_label"`
	OnBehalfOf string    `json:"on_behalf_of,omitempty"`
	Action     string    `json:"action"`
	TargetKind string    `json:"target_kind"`
	TargetID   string    `json:"target_id"`
	Before     any       `json:"before,omitempty"`
	After      any       `json:"after,omitempty"`
	IP         string    `json:"ip,omitempty"`
	At         time.Time `json:"at"`
}

// Node is a machine (operator views).
type Node struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Region       string         `json:"region"`
	MeshIP       string         `json:"mesh_ip"`
	Status       status.Node    `json:"status"`
	Runtime      string         `json:"runtime"`
	AgentVersion string         `json:"agent_version"`
	Capacity     map[string]any `json:"capacity"`
	Usage        map[string]any `json:"usage"`
	LastSeenAt   *time.Time     `json:"last_seen_at,omitempty"`
	Apps         int            `json:"apps"`
}

// Manifest is the recorded manifest at a commit.
type Manifest struct {
	CommitSHA string           `json:"commit_sha"`
	Valid     bool             `json:"valid"`
	Content   map[string]any   `json:"content"`
	Errors    []map[string]any `json:"errors"`
	CreatedAt time.Time        `json:"created_at"`
}

// Validation is the platform's view of a manifest (POST /validate).
type Validation struct {
	Valid    bool `json:"valid"`
	Problems []struct {
		Path    string `json:"path"`
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"problems"`
	Plan   string `json:"plan"`
	Limits struct {
		RepoBytes int64 `json:"repo_bytes"`
	} `json:"limits"`
	Unavailable []string `json:"unavailable"`
	// Start is the app's typical start, for doctor's W100; absent until one is recorded.
	Start *AppStart `json:"start,omitempty"`
	// Promoted is true when the app is a Promoted app in force (CONTROL-PLANE.md §6.15), which
	// may keep its own sign-in, so doctor skips W090.
	Promoted bool `json:"promoted"`
}

// DeviceCode is the answer to POST /device/code.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// DeviceToken is the answer to POST /device/token, and to POST /device/approve without the token
// itself.
type DeviceToken struct {
	Status    DeviceStatus `json:"status"`
	Token     string       `json:"token,omitempty"`
	ExpiresAt *time.Time   `json:"expires_at,omitempty"`
	Scopes    []string     `json:"scopes,omitempty"`
	Org       *Org         `json:"org,omitempty"`
	User      *User        `json:"user,omitempty"`
	// Orgs is, for a login for the whole account, the businesses recorded on it: the ones the
	// person belonged to when they approved it (CONTROL-PLANE.md §4.4).
	Orgs []Org `json:"orgs,omitempty"`
}

// DeviceInfo describes a pending code to the person approving it (GET /device/:code).
type DeviceInfo struct {
	UserCode      string        `json:"user_code"`
	Status        DeviceStatus  `json:"status"`
	ExpiresAt     time.Time     `json:"expires_at"`
	Orgs          []Org         `json:"orgs"` // the caller's orgs to choose from
	RequestedFrom RequestedFrom `json:"requested_from"`
	// Suggested is the choice the page starts on: the narrowest login that fits what whisk
	// login said it is working on (CONTROL-PLANE.md §4.5).
	Suggested DeviceSuggestion `json:"suggested"`
}

// RequestedFrom is the computer that asked for a device code and where the request came from
// (CONTROL-PLANE.md §4.5): the coding agent's name, the computer's name and system as whisk login
// reported them, the address, and that address's country (two letters, "" unknown) and network.
type RequestedFrom struct {
	Agent      string `json:"agent"`
	DeviceName string `json:"device_name"`
	OS         string `json:"os"`
	IP         string `json:"ip"`
	Country    string `json:"country"`
	Network    string `json:"network"`
}

// DeviceCodeRequest is POST /device/code: the public key whisk login made for this computer
// (Ed25519, standard base64), the coding agent's own name for itself (Claude Code, Codex), the
// computer's name and its system (os/arch), and the business and app it is working on when it
// knows them (slugs, from the directory's binding or --org and --app), so the approval page
// starts on the narrowest login that fits.
type DeviceCodeRequest struct {
	PublicKey  string `json:"public_key"`
	Agent      string `json:"agent,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	OS         string `json:"os,omitempty"`
	Org        string `json:"org,omitempty"`
	App        string `json:"app,omitempty"`
}

// WhoamiToken is the token in use as GET /whoami describes it. Bound says the token is bound to
// the computer that signed in (every request with it is signed); DeviceName names that computer.
type WhoamiToken struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Label      string    `json:"label"`
	Scopes     []string  `json:"scopes"`
	ExpiresAt  time.Time `json:"expires_at"`
	Bound      bool      `json:"bound"`
	DeviceName string    `json:"device_name,omitempty"`
}

// GitPassword is the answer to POST /tokens/git: a short git password for the signed request's
// bound token, its username and when it stops working (CONTROL-PLANE.md §6.3).
type GitPassword struct {
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Request bodies.

// CreateOrgRequest is POST /orgs.
type CreateOrgRequest struct {
	Name   string `json:"name"`
	Slug   string `json:"slug,omitempty"`
	Region string `json:"region,omitempty"`
}

// PatchOrgRequest is PATCH /orgs/:org. Settings change through PUT /orgs/:org/settings only.
type PatchOrgRequest struct {
	Name *string `json:"name,omitempty"`
}

// ConfirmOrgRequest is POST /orgs/:org/confirm.
type ConfirmOrgRequest struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Region string `json:"region,omitempty"`
}

// InviteRequest is POST /orgs/:org/members.
type InviteRequest struct {
	Email string     `json:"email"`
	Name  string     `json:"name,omitempty"`
	Role  Role       `json:"role"`
	Kind  MemberKind `json:"kind,omitempty"`
}

// PatchMemberRequest is PATCH /orgs/:org/members/:id.
type PatchMemberRequest struct {
	Role Role       `json:"role,omitempty"`
	Kind MemberKind `json:"kind,omitempty"`
}

// GroupRequest is POST/PATCH groups.
type GroupRequest struct {
	Name string `json:"name"`
}

// GroupMembersRequest is PUT /orgs/:org/groups/:id/members.
type GroupMembersRequest struct {
	Members []string `json:"members"`
}

// CreateAppRequest is POST /orgs/:org/apps.
type CreateAppRequest struct {
	Slug   string `json:"slug"`
	Name   string `json:"name,omitempty"`
	Region string `json:"region,omitempty"`
}

// PatchAppRequest is PATCH /orgs/:org/apps/:app.
type PatchAppRequest struct {
	Name *string `json:"name,omitempty"`
	// AlwaysOn is refused: whisk.yaml decides (CONTROL-PLANE.md §6.5).
	AlwaysOn *bool          `json:"always_on,omitempty"`
	Settings map[string]any `json:"settings,omitempty"`
}

// ValidateRequest is POST /orgs/:org/apps/:app/validate.
type ValidateRequest struct {
	Manifest string `json:"manifest"`
}

// AccessRequest is PUT /orgs/:org/apps/:app/access.
type AccessRequest struct {
	Grants []Grant `json:"grants"`
}

// CreateDeployRequest is POST /orgs/:org/apps/:app/deploys: a redeploy or rollback.
type CreateDeployRequest struct {
	BuildID     string `json:"build_id,omitempty"`
	DeployID    string `json:"deploy_id,omitempty"` // roll back to this deploy's build
	CommitSHA   string `json:"commit_sha,omitempty"`
	Environment string `json:"environment,omitempty"` // default production
}

// DeclareSecretRequest is POST /orgs/:org/secrets.
type DeclareSecretRequest struct {
	Name  string      `json:"name"`
	Scope SecretScope `json:"scope,omitempty"`
	AppID string      `json:"app_id,omitempty"`
	Value string      `json:"value,omitempty"` // human sessions only
}

// SecretValueRequest is PUT /orgs/:org/secrets/:name/value.
type SecretValueRequest struct {
	Value string `json:"value"`
	AppID string `json:"app_id,omitempty"` // app-scoped secret
	Note  string `json:"note,omitempty"`
}

// RollbackSecretRequest is POST /orgs/:org/secrets/:name/rollback.
type RollbackSecretRequest struct {
	Version int    `json:"version"`
	AppID   string `json:"app_id,omitempty"`
}

// SecretPreviewsRequest is PUT /orgs/:org/secrets/:name/previews: whether preview environments
// receive the secret. A person's, as setting a value is.
type SecretPreviewsRequest struct {
	Previews bool   `json:"previews"`
	AppID    string `json:"app_id,omitempty"` // app-scoped secret
}

// ShareSecretRequest is POST /orgs/:org/secrets/:name/share: the app whose value every app of
// the org that names the secret reads from now on.
type ShareSecretRequest struct {
	AppID string `json:"app_id"`
}

// SharedSecret answers a share: the secret, now org-scoped, and the apps whose empty slot of it
// was removed so they read it, by slug.
type SharedSecret struct {
	Secret Secret   `json:"secret"`
	Apps   []string `json:"apps"`
}

// DomainRequest is POST /orgs/:org/apps/:app/domains.
type DomainRequest struct {
	Hostname string `json:"hostname"`
}

// DeployKeyResponse is POST /orgs/:org/apps/:app/deploy-keys: the token, shown once.
type DeployKeyResponse struct {
	Token  Token  `json:"token"`
	Value  string `json:"value"`
	GitURL string `json:"git_url"`
}

// DeviceApproveRequest is POST /device/approve (session).
// AllOrgs approves the login for the person's whole account: every org they belong to now, as
// themselves, and never one they join later (CONTROL-PLANE.md §4.5). Otherwise org_id (with
// app_id to narrow it to one app) or new_org names what the login is for. Agent is the name the
// login goes by, as the person kept or changed it; empty keeps the name the agent gave.
type DeviceApproveRequest struct {
	UserCode string `json:"user_code"`
	Agent    string `json:"agent,omitempty"`
	AllOrgs  bool   `json:"all_orgs,omitempty"`
	OrgID    string `json:"org_id,omitempty"`
	AppID    string `json:"app_id,omitempty"`
	NewOrg   *struct {
		Name   string `json:"name"`
		Region string `json:"region,omitempty"`
	} `json:"new_org,omitempty"`
	Deny bool `json:"deny,omitempty"`
}

// RegisterNodeRequest is POST /operator/nodes.
type RegisterNodeRequest struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

// RegisterNodeResponse carries the enrolment token, shown once.
type RegisterNodeResponse struct {
	Node       Node   `json:"node"`
	EnrolToken string `json:"enrol_token"`
}

// OrgHome is GET /orgs/:org/home: the one screen answering "is everything okay?".
type OrgHome struct {
	Org       Org      `json:"org"`
	Apps      []App    `json:"apps"`
	Running   int      `json:"running"`
	Sleeping  int      `json:"sleeping"`
	Problems  int      `json:"problems"`
	NeedsYou  NeedsYou `json:"needs_you"`
	TrustLine string   `json:"trust_line"`
	// Owners names the org's owners for a member, who asks one of them for an app; empty for
	// everyone else (CONTROL-PLANE.md §5.2, "Inspect").
	Owners []string `json:"owners"`
}

// NeedsYou lists what waits on a human.
type NeedsYou struct {
	UnsetSecrets   []string `json:"unset_secrets"` // "app/NAME"
	PendingInvites int      `json:"pending_invites"`
	FailedDeploys  int      `json:"failed_deploys"` // today
	Approvals      int      `json:"approvals"`
	Confirm        bool     `json:"confirm"` // org still pending
	// FailingLogs names the log destinations whose last batch was refused or not answered
	// (CONTROL-PLANE.md §6.17), as each destination's where.
	FailingLogs []string `json:"failing_logs"`
}

// Comp is a business on a paid plan free of charge, as the operator reads it.
type Comp struct {
	Plan     string     `json:"plan"`
	EndsAt   *time.Time `json:"ends_at"`
	Reason   string     `json:"reason"`
	By       string     `json:"by"` // the operator's email, empty once they are gone
	CompedAt time.Time  `json:"comped_at"`
}

// Timeline is where an org stands on the payment timeline (CONTROL-PLANE.md §6.15), with the
// dates that matter next.
type Timeline struct {
	Status       status.Org   `json:"status"`
	PastDueAt    *time.Time   `json:"past_due_at,omitempty"`
	StopsAt      *time.Time   `json:"stops_at,omitempty"`
	FrozenAt     *time.Time   `json:"frozen_at,omitempty"`
	FrozenReason FrozenReason `json:"frozen_reason,omitempty"`
	ShreddingAt  *time.Time   `json:"shredding_at,omitempty"`
	ShreddedAt   *time.Time   `json:"shredded_at,omitempty"`
	// ShreddingFor is why a shredding org is being deleted, and Restorable whether its owner
	// may undo that now.
	ShreddingFor ShreddingFor `json:"shredding_for,omitempty"`
	Restorable   bool         `json:"restorable,omitempty"`
}

// Support is an operator's open support access to an org (CONTROL-PLANE.md §6.20).
type Support struct {
	ID        string     `json:"id"`
	Org       string     `json:"org"`
	Reason    string     `json:"reason"`
	StartedAt time.Time  `json:"started_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Warning is something that went wrong after a deploy went live without failing it
// (CONTROL-PLANE.md §6.5), such as FUNCTIONS_NOT_REGISTERED: a catalogued code, the sentence and
// the fix.
type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fix     string         `json:"fix"`
	Details map[string]any `json:"details,omitempty"`
}
