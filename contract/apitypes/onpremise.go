package apitypes

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
