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

// AppFork is what an app carries when it is a fork of a product: the business's own code, which
// follows Whisk's releases of the product (FORKS.md §4).
type AppFork struct {
	Product string `json:"product"`
	Name    string `json:"name"`
	// Branch is the branch of the app's repository that carries the product's releases, which
	// only Whisk moves.
	Branch string `json:"branch"`
	// Release is the number of the newest release the app's own code holds, 0 when it holds
	// none; Newest is the newest release on Branch; Behind is how many releases the code has not
	// taken.
	Release int `json:"release"`
	Newest  int `json:"newest"`
	Behind  int `json:"behind"`
	// Changed is whether the app's code is anything but release Release as Whisk made it.
	Changed bool `json:"changed"`
}

// ForkUpdate is POST /orgs/:org/apps/:app/fork/update: the app as it stands now, the deploy that
// carries its code when the update started or found one, and whether its code moved to a newer
// release.
type ForkUpdate struct {
	App     App     `json:"app"`
	Deploy  *Deploy `json:"deploy,omitempty"`
	Updated bool    `json:"updated"`
}

// ForkChanges is GET /orgs/:org/apps/:app/fork/changes: the business's own changes in the code
// the app runs (FORKS.md §9). Commit is that code's commit, "" when nothing runs yet; Release is
// the number of the newest release it holds, 0 when it holds none.
type ForkChanges struct {
	Commit  string `json:"commit"`
	Release int    `json:"release"`
	// Changes are the records of custom/changes that can be read, by name; Unreadable the files
	// there that are not records that can be read; Uncovered the files that differ from the
	// release and that no record names.
	Changes    []ForkChange     `json:"changes"`
	Unreadable []ForkUnreadable `json:"unreadable"`
	Uncovered  []string         `json:"uncovered"`
}

// ForkChange is one change of the business's own as its record says it. Unchanged is each path
// it names that is as the release has it: all of them is a change the release holds now, or one
// undone.
type ForkChange struct {
	Name      string   `json:"name"`
	Asked     string   `json:"asked"`
	Touches   []string `json:"touches"`
	Checks    []string `json:"checks"`
	Unchanged []string `json:"unchanged"`
}

// ForkUnreadable is a file of custom/changes that is not a record that can be read, and why.
type ForkUnreadable struct {
	File     string        `json:"file"`
	Problems []ForkProblem `json:"problems"`
}

// ForkProblem is one reason a file of custom/changes cannot be read, at a line of it when there
// is one.
type ForkProblem struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// ForkSuggestion is one of a fork's changes its business suggested to Whisk (FORKS.md §10): the
// change as its record said it, the files the suggestion sent, the release the change was made
// on and the commit of the code it was sent from, the licence Whisk may use it under, and what
// became of it: open until Whisk decides, replaced by a later suggestion of the same change,
// declined with Reason, or taken into release TakenIn, with Moves saying what becomes of the
// business's data once that release's version of the change takes the place of its own.
type ForkSuggestion struct {
	ID          string            `json:"id"`
	Change      string            `json:"change"`
	Asked       string            `json:"asked"`
	Files       []string          `json:"files"`
	Release     int               `json:"release"`
	Commit      string            `json:"commit"`
	Licence     SuggestionLicence `json:"licence"`
	Status      SuggestionStatus  `json:"status"`
	Reason      string            `json:"reason,omitempty"`
	TakenIn     int               `json:"taken_in,omitempty"`
	Moves       string            `json:"moves,omitempty"`
	SuggestedAt time.Time         `json:"suggested_at"`
	DecidedAt   *time.Time        `json:"decided_at,omitempty"`
}

// ForkSuggestions is GET /orgs/:org/apps/:app/fork/suggestions: the fork's suggestions, newest
// first.
type ForkSuggestions struct {
	Suggestions []ForkSuggestion `json:"suggestions"`
}

// ForkSuggestRequest is POST /orgs/:org/apps/:app/fork/suggestions: the change of the code the
// fork runs to suggest, by name.
type ForkSuggestRequest struct {
	Change string `json:"change"`
}

// ForkDetail is one thing of the business's own that the lines a suggestion would send hold: the
// file and the line, none for the file's name, and what it is, never the value.
type ForkDetail struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	What string `json:"what"`
}

// SuggestionLicence is the licence a business gives Whisk a suggested change's lines under, which
// the person is shown before they suggest one.
type SuggestionLicence string

// SuggestionMIT is the MIT licence, which Whisk's own examples are published under.
const SuggestionMIT SuggestionLicence = "MIT"

// SuggestionStatus is what became of a suggestion.
type SuggestionStatus string

const (
	SuggestionOpen     SuggestionStatus = "open"
	SuggestionReplaced SuggestionStatus = "replaced"
	SuggestionDeclined SuggestionStatus = "declined"
	SuggestionTaken    SuggestionStatus = "taken"
)

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

// ProductKind is what a copy of a product is (FORKS.md §1): ProductManaged is Whisk's code, which
// Whisk runs for the business; ProductForks is the business's own code, which follows Whisk's
// releases.
type ProductKind string

const (
	ProductManaged ProductKind = "managed"
	ProductForks   ProductKind = "forks"
)

// ManagedProduct is one product a business may add: what a copy takes and its monthly price in
// the plan's currency's cents, absent when the plan does not include it (MANAGED-APPS.md §7).
type ManagedProduct struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Kind is ProductManaged or ProductForks. A platform that predates forks sends none, and
	// every product is managed.
	Kind     ProductKind      `json:"kind,omitempty"`
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

// Managed is GET /orgs/:org/managed: the products, the business's copies of those Whisk runs,
// and its forks of those whose copies are its own code (FORKS.md).
type Managed struct {
	Products []ManagedProduct `json:"products"`
	Copies   []App            `json:"copies"`
	Forks    []App            `json:"forks"`
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
