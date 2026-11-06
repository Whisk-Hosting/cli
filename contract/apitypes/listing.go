package apitypes

// The business's home lists its apps by their listing (CONTRACT.md §3.4): a title, which is the
// app's name, one sentence on what people do in it, and the category it sits under.

// AppListing is how the home lists one app, on the home's apps only.
type AppListing struct {
	Description string `json:"description"`
	// Category is "" for an app given none, which the home lists last.
	Category string `json:"category"`
	// Hidden is true when an owner keeps the app off the home; only someone who may change the
	// home is sent a hidden app.
	Hidden bool `json:"hidden"`
	// FromYAML is what the app's live whisk.yaml says and Owned names the fields an owner set,
	// sent only to someone who may change the home, so the agent's text can be put back.
	FromYAML *ListingValues `json:"from_yaml,omitempty"`
	Owned    []string       `json:"owned,omitempty"`
}

// ListingValues are a listing's three fields.
type ListingValues struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// AppAccess is who on the business's team can open an app, on the home's apps only: everyone on
// the team, or the groups and the people named one by one, by name. Public is an app anyone may
// open without signing in as the team.
type AppAccess struct {
	Everyone bool     `json:"everyone"`
	Public   bool     `json:"public"`
	Groups   []string `json:"groups"`
	People   []string `json:"people"`
}

// HomeLayout is PUT /orgs/{org}/home: an owner's whole home at once, the categories in order and
// every app in order with its listing. An app the request leaves out keeps its listing and comes
// after those named.
type HomeLayout struct {
	Categories []string  `json:"categories"`
	Apps       []HomeApp `json:"apps"`
}

// HomeApp is one app in a HomeLayout. Title is the app's name.
type HomeApp struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Hidden      bool   `json:"hidden"`
}

// ValidationListing is the validate answer's view of the listing, for doctor's W160 and W161:
// the business's categories in the home's order, and what the home shows for this app where an
// owner set it.
type ValidationListing struct {
	Categories []string       `json:"categories"`
	Shown      *ListingValues `json:"shown,omitempty"`
	Owned      []string       `json:"owned"`
}
