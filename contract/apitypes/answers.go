package apitypes

// Small answers, each named for what it tells. Their fields are in the order encoding/json writes
// a map's keys, so naming them changed no answer's bytes.

// PageURL is where to send the person next: a payment provider's page or portal.
type PageURL struct {
	URL string `json:"url"`
}

// GithubInstallURL answers POST /v1/orgs/{org}/github/installations: where the person installs
// Whisk's GitHub App.
type GithubInstallURL struct {
	InstallURL string `json:"install_url"`
}

// SyncQueued answers POST /v1/orgs/{org}/apps/{app}/github/sync: the sync is queued.
type SyncQueued struct {
	Queued bool `json:"queued"`
}

// LogDestinationTest answers POST /v1/orgs/{org}/logs/destinations/{id}/test: the line sent.
type LogDestinationTest struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Sent    bool   `json:"sent"`
}

// ErrorStatusSet answers POST /v1/orgs/{org}/apps/{app}/errors/{id}: the group and its status now.
type ErrorStatusSet struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CustomerSignupSet answers PUT /v1/orgs/{org}/apps/{app}/customers/signup: whether strangers may
// register now.
type CustomerSignupSet struct {
	Signup bool `json:"signup"`
}

// ClientErrorReceipt answers POST /v1/client-errors.
type ClientErrorReceipt struct {
	Received bool `json:"received"`
}

// EventQueued answers POST /v1/orgs/{org}/apps/{app}/events: the platform's id for the event.
type EventQueued struct {
	ID string `json:"id"`
}

var _ = more.addWire(PageURL{}, GithubInstallURL{}, SyncQueued{}, LogDestinationTest{}, ErrorStatusSet{}, CustomerSignupSet{}, ClientErrorReceipt{}, EventQueued{})
