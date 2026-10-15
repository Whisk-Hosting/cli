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
	Slug     string           `json:"slug"`
	Name     string           `json:"name"`
	Variants []ManagedVariant `json:"variants"`
	Settings []string         `json:"settings"`
	Links    []string         `json:"links"`
	// LinkKey is the setting that tells apart the copies linked to one app, when the product
	// has one: an app may then have a copy for each value of it (MANAGED-APPS.md §4).
	LinkKey     string `json:"link_key,omitempty"`
	MonthCents  *int64 `json:"month_cents,omitempty"`
	Available   bool   `json:"available"`
	Unavailable string `json:"unavailable,omitempty"` // plan | trial | no_release
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
