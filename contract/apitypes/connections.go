package apitypes

import (
	"time"

	"github.com/whisk-run/contract/connect"
)

// Connections: the outside systems an app reaches through the broker without holding their
// credential, and what a person granted each (CONTROL-PLANE.md §6.8, BROKER.md).

// GrantKind is what a person grants for a deploy to go ahead: a connection, a business secret
// the app is not on the list of, or access wider than the live deploy's (grants.go). Every kind
// waits in the same list (Deploy.GrantsNeeded) and answers GRANT_NEEDED.
type GrantKind string

const (
	GrantConnection GrantKind = "connection"
	// GrantSecret is a business secret the deploy's manifest names that the app is not on the
	// list of apps granted it (CONTROL-PLANE.md §6.7 Grants).
	GrantSecret GrantKind = "secret"
	// GrantAccess is access wider than the app's live production deploy has: a new app it
	// calls, a new webhook, a route made public, or a change to how routes or customers sign
	// in (CONTROL-PLANE.md §6.5). A secret name never is: the app's own needs no grant.
	GrantAccess GrantKind = "access"
)

// ConnectionEnvironment is what a connection's grant covers: production, or every preview of
// the app.
type ConnectionEnvironment string

const (
	ConnectionProduction ConnectionEnvironment = "production"
	ConnectionPreviews   ConnectionEnvironment = "previews"
)

// ConnectionState is where a grant stands.
type ConnectionState string

const (
	ConnectionActive  ConnectionState = "active"
	ConnectionPaused  ConnectionState = "paused"
	ConnectionRevoked ConnectionState = "revoked"
)

// GrantNeeded is one thing a blocked deploy waits on a person to grant. For a connection, Name
// is the connection's, Hash the version to grant, RecipeChanged whether its address or recipe
// differs from the grant (or there is none) and NewOperations the operations the grant does not
// name. For a secret, Name is the secret's and Hash its ID. For access, Name says what widened
// (such as calls:billing) and Hash is the access the deploy asks for. Change says it in plain
// words for every kind but a connection, whose summary the grant screen draws.
type GrantNeeded struct {
	Kind          GrantKind             `json:"kind"`
	Name          string                `json:"name"`
	Environment   ConnectionEnvironment `json:"environment"`
	Hash          string                `json:"hash,omitempty"`
	RecipeChanged bool                  `json:"recipe_changed"`
	NewOperations []connect.Operation   `json:"new_operations"`
	Change        string                `json:"change,omitempty"`
	// AI is set for a connection to an AI model: the person chooses its spending limit as they
	// allow it.
	AI *GrantAI `json:"ai,omitempty"`
}

// GrantAI is the spending limit an Allow page starts an AI connection's fields at: its grant's,
// else the one whisk.yaml suggests, else none.
type GrantAI struct {
	SpendCap connect.SpendCap `json:"spend_cap"`
}

// ConnectionGrant is what a person granted a connection in one environment.
type ConnectionGrant struct {
	ID             string                `json:"id"`
	Environment    ConnectionEnvironment `json:"environment"`
	Hash           string                `json:"hash"`
	State          ConnectionState       `json:"state"`
	LimitPerMinute int                   `json:"limit_per_minute"`
	LimitPerDay    int                   `json:"limit_per_day"`
	GrantedBy      string                `json:"granted_by"` // the person's name or email
	GrantedAt      time.Time             `json:"granted_at"`
	StateBy        string                `json:"state_by,omitempty"`
	StateAt        *time.Time            `json:"state_at,omitempty"`
	// Current is whether the grant covers the declared version: same address and recipe, and
	// every operation it names.
	Current bool `json:"current"`
	// Missing lists the declared operations the grant does not cover.
	Missing []connect.Operation `json:"missing"`
	// CallsToday and RefusedToday count calls since midnight UTC.
	CallsToday   int `json:"calls_today"`
	RefusedToday int `json:"refused_today"`
	// SignIn is the person's sign-in at the system for this environment, when the recipe signs
	// in (CONTROL-PLANE.md §6.8 "Signing in").
	SignIn *ConnectionSignIn `json:"sign_in,omitempty"`
}

// SignInState is a grant's sign-in: nobody has signed in under the granted recipe, signed in,
// or the system stopped accepting the sign-in.
type SignInState string

// Sign-in states.
const (
	SignInNeeded SignInState = "needed"
	SignInActive SignInState = "active"
	SignInLapsed SignInState = "lapsed"
)

// ConnectionSignIn is a grant's sign-in at the outside system. Its tokens never leave Whisk.
type ConnectionSignIn struct {
	State SignInState `json:"state"`
	By    string      `json:"by,omitempty"` // the person's name or email
	At    *time.Time  `json:"at,omitempty"`
	// Scopes are the scopes the system says it granted, or those the recipe asked for.
	Scopes       []string   `json:"scopes"`
	LapsedAt     *time.Time `json:"lapsed_at,omitempty"`
	LapsedReason string     `json:"lapsed_reason,omitempty"`
}

// SignInRequest starts a person's sign-in at a connection's system for one environment
// (POST /orgs/:org/apps/:app/connections/:name/sign-in).
type SignInRequest struct {
	Environment ConnectionEnvironment `json:"environment"`
}

// SignInStart is where to send the person's browser to sign in.
type SignInStart struct {
	URL string `json:"url"`
}

// Connection is one connection an app's active manifests declare (GET
// /orgs/:org/apps/:app/connections): Whisk's summary of what granting it allows, drawn only from
// the rules the broker enforces, its version's hash, and its grants by environment.
type Connection struct {
	Name    string          `json:"name"`
	Hash    string          `json:"hash"`
	Summary connect.Summary `json:"summary"`
	// Unset names the secrets the recipe reads that have no value yet.
	Unset  []string          `json:"unset"`
	Grants []ConnectionGrant `json:"grants"`
	// Waiting is whether a blocked deploy waits on this connection's grant.
	Waiting bool `json:"waiting"`
	// Keypair is the key pair Whisk made for the connection, when its recipe asks for one: nil
	// until a person makes it.
	Keypair *ConnectionKeypair `json:"keypair,omitempty"`
	// AI is an AI connection's spend against its cap (CONTRACT.md §3.1 AI models); nil for any
	// other connection.
	AI *ConnectionAI `json:"ai,omitempty"`
	// RedirectURI is the address the business gives the app it registers with the system, when
	// the recipe signs in with the business's own client rather than Whisk's vendor app.
	RedirectURI string `json:"redirect_uri,omitempty"`
}

// AIPeriod is one window of an AI connection's spend cap.
type AIPeriod string

// The windows, in the order they are shown.
const (
	AIDay   AIPeriod = "day"
	AIWeek  AIPeriod = "week"
	AIMonth AIPeriod = "month"
	AITotal AIPeriod = "total"
)

// ConnectionAI is what an AI connection has spent of its cap (CONTROL-PLANE.md §6.8 AI
// connections): one entry for each window the cap names, and the deploys whose calls stopped
// because an answer named none of the cost parts.
type ConnectionAI struct {
	Windows    []AIWindow     `json:"windows"`
	Unreadable []AIUnreadable `json:"unreadable"`
}

// AIWindow is one window's cap and spend, in US dollars. Cap is production's granted cap, or
// the declared one before a grant. Stopped is whether calls are refused now; Until is when the
// window ends, absent for total.
type AIWindow struct {
	Period  AIPeriod   `json:"period"`
	Cap     float64    `json:"cap"`
	Spent   float64    `json:"spent"`
	Stopped bool       `json:"stopped"`
	Until   *time.Time `json:"until,omitempty"`
}

// AIUnreadable is a deploy whose calls to the connection stopped because an answer to the
// operation named none of the cost parts.
type AIUnreadable struct {
	DeployID  string    `json:"deploy_id"`
	Operation string    `json:"operation"`
	At        time.Time `json:"at"`
}

// ConnectionKeypair is a key pair Whisk made for a connection: the private key is the named
// secret, which nobody can read; the certificate is what a person uploads to the outside system.
type ConnectionKeypair struct {
	Secret string `json:"secret"`
	// Certificate is the self-signed certificate, PEM.
	Certificate string `json:"certificate"`
	// Fingerprint is the SHA-256 of the certificate, upper-case hex pairs joined by colons.
	Fingerprint string    `json:"fingerprint"`
	MadeBy      string    `json:"made_by"`
	MadeAt      time.Time `json:"made_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// GrantConnectionRequest grants exactly the version whose hash the person was shown.
type GrantConnectionRequest struct {
	Environment    ConnectionEnvironment `json:"environment"`
	Hash           string                `json:"spec_hash"`
	LimitPerMinute int                   `json:"limit_per_minute,omitempty"`
	LimitPerDay    int                   `json:"limit_per_day,omitempty"`
	// SpendCap is the spend cap the person chose for an AI connection, in US dollars; left out,
	// the one whisk.yaml suggests.
	SpendCap *connect.SpendCap `json:"spend_cap,omitempty"`
}

// SpendCapRequest changes the spend cap a person set on an AI connection's grant in one
// environment.
type SpendCapRequest struct {
	Environment ConnectionEnvironment `json:"environment"`
	SpendCap    connect.SpendCap      `json:"spend_cap"`
}

// ConnectionLimitsRequest changes how many calls a minute and a day a connection's grant in one
// environment allows. A limit left out keeps its value.
type ConnectionLimitsRequest struct {
	Environment    ConnectionEnvironment `json:"environment"`
	LimitPerMinute int                   `json:"limit_per_minute,omitempty"`
	LimitPerDay    int                   `json:"limit_per_day,omitempty"`
}

// ConnectionStateRequest pauses, resumes or revokes one environment's grant, or every
// environment's when Environment is empty.
type ConnectionStateRequest struct {
	Environment ConnectionEnvironment `json:"environment,omitempty"`
}

// BrokerDrop is whether the broker let go of what it held for the changed grants at once.
type BrokerDrop string

// What the broker did.
const (
	// BrokerDropped: the broker dropped its held answers and tokens for the grants at once.
	BrokerDropped BrokerDrop = "dropped"
	// BrokerUnreachable: the broker did not answer; it drops them at its next call for the grant,
	// which the control plane refuses whatever the broker holds.
	BrokerUnreachable BrokerDrop = "unreachable"
)

// ProviderRevocation is the outcome of asking the issuer of the tokens the broker held to
// cancel them (RFC 7009), on a revoke.
type ProviderRevocation string

// Outcomes of a provider revocation.
const (
	ProviderRevoked      ProviderRevocation = "revoked"
	ProviderFailed       ProviderRevocation = "failed"
	ProviderNotSupported ProviderRevocation = "not_supported"
	ProviderNoneHeld     ProviderRevocation = "none_held"
	ProviderUnknown      ProviderRevocation = "unknown"
)

// ConnectionsChanged answers a pause, resume, revoke or pause-all: how many grants changed, what
// the broker let go of, and on a revoke what the provider said and which of Whisk's copies of
// secrets were deleted. Deleting Whisk's copy does not cancel a credential at its issuer.
type ConnectionsChanged struct {
	Changed  int                `json:"changed"`
	Broker   BrokerDrop         `json:"broker,omitempty"`
	Provider ProviderRevocation `json:"provider,omitempty"`
	Deleted  []string           `json:"deleted"`
}
