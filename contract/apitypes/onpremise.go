package apitypes

import "time"

// LicenceState is what a Whisk On-Premise licence allows now (ON-PREMISE.md §3.3).
type LicenceState string

const (
	LicenceMissing LicenceState = "missing"
	LicenceInvalid LicenceState = "invalid"
	LicenceActive  LicenceState = "active"
	LicenceEnding  LicenceState = "ending"
	LicenceExpired LicenceState = "expired"
)

// Licence is GET /operator/licence on Whisk On-Premise: the installed licence and what it
// allows.
type Licence struct {
	State    LicenceState `json:"state"`
	Licence  string       `json:"licence,omitempty"`
	Customer string       `json:"customer,omitempty"`
	Starts   string       `json:"starts,omitempty"`
	Ends     string       `json:"ends,omitempty"`
	DaysLeft int          `json:"days_left"`
	Problem  string       `json:"problem,omitempty"`
}

// InstallLicenceRequest is PUT /operator/licence: the licence file's whole text.
type InstallLicenceRequest struct {
	File string `json:"file"`
}

// SupportDoor is GET /operator/support-door on Whisk On-Premise (ON-PREMISE.md §9): whether
// Whisk's staff may reach the install now, with which categories and until when.
type SupportDoor struct {
	Open       bool            `json:"open"`
	Categories []AgentCategory `json:"categories"`
	OpenedBy   string          `json:"opened_by,omitempty"`
	OpenedAt   *time.Time      `json:"opened_at,omitempty"`
	EndsAt     *time.Time      `json:"ends_at,omitempty"`
	Relay      string          `json:"relay"`
}

// OpenSupportDoorRequest is PUT /operator/support-door: from 1 to 72 hours, and the categories
// Whisk's staff may use beyond read.
type OpenSupportDoorRequest struct {
	Hours      int      `json:"hours"`
	Categories []string `json:"categories"`
}

// SupportDoorSeen is one install's open door as whisk.run's operators see it.
type SupportDoorSeen struct {
	Licence    string          `json:"licence"`
	Customer   string          `json:"customer"`
	Categories []AgentCategory `json:"categories"`
	EndsAt     time.Time       `json:"ends_at"`
	LastSeen   time.Time       `json:"last_seen"`
}

// SupportCallRequest is one call Whisk's staff send through an open door.
type SupportCallRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// SupportCallAnswer is the install's answer to a call: its API's status and body, cut at 1 MB.
type SupportCallAnswer struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        string `json:"body"`
	Cut         bool   `json:"cut,omitempty"`
}
