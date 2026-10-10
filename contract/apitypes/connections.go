package apitypes

import (
	"time"

	"github.com/whisk-run/contract/connect"
)

// Connections: the outside systems an app reaches through the broker without holding their
// credential, and what a person granted each (CONTROL-PLANE.md §6.8, BROKER.md).

// GrantKind is what a person grants for a deploy to go ahead. Connection is the one kind today;
// every kind waits in the same list (Deploy.GrantsNeeded) and answers GRANT_NEEDED.
type GrantKind string

const (
	GrantConnection GrantKind = "connection"
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
// name.
type GrantNeeded struct {
	Kind          GrantKind             `json:"kind"`
	Name          string                `json:"name"`
	Environment   ConnectionEnvironment `json:"environment"`
	Hash          string                `json:"hash,omitempty"`
	RecipeChanged bool                  `json:"recipe_changed"`
	NewOperations []connect.Operation   `json:"new_operations"`
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

// VendorApp is one of Whisk's own developer apps with an outside system, as the operator page
// shows it (GET /v1/operator/vendor-apps): never its values, only the client ID's hint.
type VendorApp struct {
	Name         string    `json:"name"`
	ClientIDHint string    `json:"client_id_hint"`
	WebhookKey   bool      `json:"webhook_key"`
	PerSecond    int       `json:"per_second"`
	PerMinute    int       `json:"per_minute"`
	PerDay       int       `json:"per_day"`
	SetBy        string    `json:"set_by"`
	SetAt        time.Time `json:"set_at"`
	// Grants counts the grants not revoked whose recipe names it.
	Grants int `json:"grants"`
}

// VendorAppRequest sets a vendor app (PUT /v1/operator/vendor-apps/:name). An empty secret or
// webhook key keeps the one set; a new vendor app needs its secret.
type VendorAppRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	WebhookKey   string `json:"webhook_key,omitempty"`
	PerSecond    int    `json:"per_second"`
	PerMinute    int    `json:"per_minute"`
	PerDay       int    `json:"per_day"`
}
