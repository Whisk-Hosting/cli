package apitypes

import (
	"encoding/json"
	"time"
)

// AppCDNRequest is PUT /v1/orgs/{org}/apps/{app}/cdn.
type AppCDNRequest struct {
	On bool `json:"on"`
}

type ClientRequest struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Contact  string `json:"contact"`
	Region   string `json:"region"`
	Interval string `json:"interval"`
}

// SleepRequest optionally names a preview to put to sleep instead of production.
type SleepRequest struct {
	Environment string `json:"environment"`
}

// AppEmailDomainRequest is POST /orgs/:org/apps/:app/email/domains.
type AppEmailDomainRequest struct {
	Domain string `json:"domain"`
}

// DomainRedirectRequest is the body of PUT …/domains/:id/redirect.
type DomainRedirectRequest struct {
	To string `json:"to"`
}

// MainAddressRequest is the body of PUT …/main-address.
type MainAddressRequest struct {
	Hostname string `json:"hostname"`
}

// RestoreJSON is one self-service restore as the API shows it.
type RestoreJSON struct {
	ID          string `json:"id"`
	Environment string `json:"environment"`
	// At is the moment a restore of the app's own history goes back to; Import names the
	// uploaded dump an import loads instead (CONTROL-PLANE.md §6.18). One of the two is set.
	// Before is the deploy whose restore point a restore goes back to, At being that point's
	// moment.
	At               *time.Time      `json:"at,omitempty"`
	Import           string          `json:"import,omitempty"`
	Before           string          `json:"before,omitempty"`
	Swap             bool            `json:"swap"`
	Status           string          `json:"status"`
	TargetDatabase   string          `json:"target_database,omitempty"`
	PreviousDatabase string          `json:"previous_database,omitempty"`
	Error            json.RawMessage `json:"error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
}

// VisitReceipt answers POST /v1/visits.
type VisitReceipt struct {
	Counted bool `json:"counted"`
	// Flags say what the visit is (bot, signed_in, app_page, private, other_site), so the
	// numbers can leave it out when they are read.
	Flags  []string `json:"flags"`
	Reason string   `json:"reason,omitempty"`
}

var _ = more.addWire(AppCDNRequest{}, ClientRequest{}, SleepRequest{}, AppEmailDomainRequest{}, DomainRedirectRequest{}, MainAddressRequest{}, RestoreJSON{}, VisitReceipt{})
