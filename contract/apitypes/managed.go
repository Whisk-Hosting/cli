package apitypes

import "time"

// Managed apps (MANAGED-APPS.md): apps in a business whose code and releases belong to Whisk.

// AppManaged is what an app carries when it is a copy of a product (MANAGED-APPS.md §10).
type AppManaged struct {
	Product  string            `json:"product"`
	Name     string            `json:"name"`
	Variant  string            `json:"variant,omitempty"`
	Release  *ManagedRelease   `json:"release,omitempty"`
	Settings map[string]string `json:"settings"`
	Links    map[string]string `json:"links"` // role → linked app id
	Held     bool              `json:"held"`
}

// ManagedRelease is one release of a product: the source deploy's commit and when it went live.
type ManagedRelease struct {
	ID     string    `json:"id"`
	Commit string    `json:"commit"`
	LiveAt time.Time `json:"live_at"`
}

// ManagedVariant is one form a product comes in.
type ManagedVariant struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// Settings are the names a business may set on a copy of this variant, besides the
	// product's.
	Settings []string `json:"settings"`
}

// ManagedProduct is one product a business may add: what a copy takes and its monthly price in
// the plan's currency's cents, absent when the plan does not include it (MANAGED-APPS.md §7).
type ManagedProduct struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Variants    []ManagedVariant `json:"variants"`
	Settings    []string         `json:"settings"`
	Links       []string         `json:"links"`
	MonthCents  *int64           `json:"month_cents,omitempty"`
	Available   bool             `json:"available"`
	Unavailable string           `json:"unavailable,omitempty"` // plan | trial | no_release
}

// Managed is GET /orgs/:org/managed: the products and the business's copies.
type Managed struct {
	Products []ManagedProduct `json:"products"`
	Copies   []App            `json:"copies"`
}

// ManagedAddRequest is POST /orgs/:org/managed.
type ManagedAddRequest struct {
	Product  string            `json:"product"`
	Variant  string            `json:"variant,omitempty"`
	Links    map[string]string `json:"links"`
	Settings map[string]string `json:"settings,omitempty"`
}

// ManagedSettingsRequest is PATCH /orgs/:org/apps/:app/managed. A setting set to "" is removed.
type ManagedSettingsRequest struct {
	Settings map[string]string `json:"settings"`
}

// LinkRequest is PUT /orgs/:org/apps/:app/links/:role.
type LinkRequest struct {
	App string `json:"app"`
}

// ManagedReleaseState is a release's place in its rollout (MANAGED-APPS.md §3).
type ManagedReleaseState string

const (
	ReleaseSoaking    ManagedReleaseState = "soaking"
	ReleaseRolling    ManagedReleaseState = "rolling"
	ReleaseHalted     ManagedReleaseState = "halted"
	ReleaseDone       ManagedReleaseState = "done"
	ReleaseSuperseded ManagedReleaseState = "superseded"
)

// OperatorRelease is one release on the operator view.
type OperatorRelease struct {
	ID         string              `json:"id"`
	Commit     string              `json:"commit"`
	Status     ManagedReleaseState `json:"status"`
	LiveAt     *time.Time          `json:"live_at,omitempty"`
	HaltedApp  string              `json:"halted_app,omitempty"`
	HaltedCode string              `json:"halted_code,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
}

// OperatorCopy is one copy on the operator view: never its settings, logs or data (§8).
type OperatorCopy struct {
	AppID       string     `json:"app_id"`
	OrgID       string     `json:"org_id"`
	OrgSlug     string     `json:"org_slug"`
	ReleaseID   string     `json:"release_id,omitempty"`
	Held        bool       `json:"held"`
	Paused      bool       `json:"paused"`
	DeployState string     `json:"deploy_state,omitempty"`
	ErrorCode   string     `json:"error_code,omitempty"`
	PausedAt    *time.Time `json:"paused_at,omitempty"`
}

// OperatorProduct is one product on the operator view.
type OperatorProduct struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	SourceAppID string            `json:"source_app_id"`
	RetiredAt   *time.Time        `json:"retired_at,omitempty"`
	Releases    []OperatorRelease `json:"releases"`
	Copies      []OperatorCopy    `json:"copies"`
}

// ProductsOrg is what PUT /operator/orgs/:org/products answers: the business marked as Whisk's
// own, whose apps may be products' sources.
type ProductsOrg struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	WhiskProducts bool   `json:"whisk_products"`
}

// OperatorProductRequest is POST /operator/managed-products.
type OperatorProductRequest struct {
	AppID string `json:"app_id"`
}
