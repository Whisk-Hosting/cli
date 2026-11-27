package apitypes

import (
	"time"
)

// Request bodies, each named for its route.

// ChangePlanRequest is the body of PUT /v1/orgs/{org}/billing/plan.
type ChangePlanRequest struct {
	Plan     string    `json:"plan"`
	Interval string    `json:"interval"`
	QuotedAt time.Time `json:"quoted_at"`
	// Reason and Note answer the one question asked when a business moves off a paid plan to Free:
	// optional, and kept only for Whisk's own view of why businesses leave.
	Reason string `json:"reason,omitempty"`
	Note   string `json:"note,omitempty"`
}

// DeviceTokenRequest is the body of POST /v1/device/token.
type DeviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

// StartRestoreRequest is the body of POST /v1/orgs/{org}/apps/{app}/restore.
type StartRestoreRequest struct {
	At          time.Time `json:"at,omitzero"`
	Import      string    `json:"import,omitempty"`
	Before      string    `json:"before,omitempty"`
	Swap        bool      `json:"swap"`
	Environment string    `json:"environment"`
}

// AuthoriseImportRequest is the body of POST /v1/orgs/{org}/apps/{app}/db/imports.
type AuthoriseImportRequest struct {
	Bytes int64 `json:"bytes"`
}

// DbQueryRequest is the body of POST /v1/orgs/{org}/apps/{app}/db/query.
type DbQueryRequest struct {
	SQL         string `json:"sql"`
	Environment string `json:"environment"`
	Database    string `json:"database"`
	Confirm     string `json:"confirm"`
	Limit       int    `json:"limit"`
}

// DbSchemaRequest is the body of POST /v1/orgs/{org}/apps/{app}/db/schema.
type DbSchemaRequest struct {
	Environment string `json:"environment"`
	Database    string `json:"database"`
}

// CreateAgentTokenRequest is the body of POST /v1/orgs/{org}/tokens.
type CreateAgentTokenRequest struct {
	Label  string   `json:"label"`
	Scopes []string `json:"scopes"`
	AppID  string   `json:"app_id"`
}

// GithubInstallRequest is the body of POST /v1/orgs/{org}/github/installations.
type GithubInstallRequest struct {
	App string `json:"app"`
}

// GithubLinkAppRequest is the body of PUT /v1/orgs/{org}/apps/{app}/github.
type GithubLinkAppRequest struct {
	Repo string `json:"repo"`
}

// DecideApprovalRequest is the body of POST /v1/approvals/{id}.
type DecideApprovalRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

// SetErrorStatusRequest is the body of POST /v1/orgs/{org}/apps/{app}/errors/{id}.
type SetErrorStatusRequest struct {
	Status string `json:"status"`
}

// AuthoriseUploadRequest is the body of POST /v1/orgs/{org}/apps/{app}/uploads.
type AuthoriseUploadRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	MaxBytes    int64  `json:"max_bytes"`
	Visibility  string `json:"visibility"`
	Parts       bool   `json:"parts"`
	// SHA256 is the file's content hash, in lowercase hex. When the environment already holds a
	// scanned, clean upload of that content, type and visibility, the answer is that upload,
	// with stored true and no form, and nothing is sent.
	SHA256 string `json:"sha256,omitempty"`
}

// LinkUploadsRequest is the body of POST /v1/orgs/{org}/apps/{app}/uploads/links.
type LinkUploadsRequest struct {
	Paths     []string `json:"paths"`
	ExpiresIn int64    `json:"expires_in"`
}

// RotateLinkKeyRequest is the body of POST /v1/orgs/{org}/apps/{app}/uploads/links/rotate.
type RotateLinkKeyRequest struct {
	Immediately bool `json:"immediately"`
}

// PatchUploadRequest is the body of PATCH /v1/orgs/{org}/apps/{app}/uploads/{id}.
// Each field left out is left as it is.
type PatchUploadRequest struct {
	Visibility string `json:"visibility,omitempty"`
	// Canonical is the address /.whisk/file/<id> names in Link: <…>; rel="canonical"; "" names
	// none.
	Canonical *string `json:"canonical,omitempty"`
	// Path is the path on the app's site, such as /posters/install.jpg, that answers with the
	// file when the app has nothing there; "" names none.
	Path *string `json:"path,omitempty"`
}

// InviteCustomerRequest is the body of POST /v1/orgs/{org}/apps/{app}/customers.
type InviteCustomerRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// SetCustomerSignupRequest is the body of PUT /v1/orgs/{org}/apps/{app}/customers/signup.
type SetCustomerSignupRequest struct {
	Signup bool `json:"signup"`
}

// PatchCustomerRequest is the body of PATCH /v1/orgs/{org}/apps/{app}/customers/{id}.
type PatchCustomerRequest struct {
	Status string `json:"status"`
}

// AddEmailDomainRequest is the body of POST /v1/orgs/{org}/email/domains.
type AddEmailDomainRequest struct {
	Domain string `json:"domain"`
}

// CreateDeployKeyRequest is the body of POST /v1/orgs/{org}/apps/{app}/deploy-keys.
type CreateDeployKeyRequest struct {
	Label string `json:"label"`
}

// DbEnvironmentRequest is the body of POST /v1/orgs/{org}/apps/{app}/db/url and db/snapshot: the
// environment, which may be left out for production.
type DbEnvironmentRequest struct {
	Environment string `json:"environment"`
}

// UploadPartsRequest is the body of POST /v1/orgs/{org}/apps/{app}/uploads/{id}/parts and
// .../complete: the store's id for an upload in parts.
type UploadPartsRequest struct {
	UploadID string `json:"upload_id"`
}

var _ = more.addWire(DbEnvironmentRequest{}, UploadPartsRequest{}, ChangePlanRequest{}, DeviceTokenRequest{}, StartRestoreRequest{}, AuthoriseImportRequest{}, DbQueryRequest{}, DbSchemaRequest{}, CreateAgentTokenRequest{}, GithubInstallRequest{}, GithubLinkAppRequest{}, DecideApprovalRequest{}, SetErrorStatusRequest{}, AuthoriseUploadRequest{}, LinkUploadsRequest{}, RotateLinkKeyRequest{}, PatchUploadRequest{}, InviteCustomerRequest{}, SetCustomerSignupRequest{}, PatchCustomerRequest{}, AddEmailDomainRequest{}, CreateDeployKeyRequest{})
