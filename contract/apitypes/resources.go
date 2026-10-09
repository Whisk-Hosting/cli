package apitypes

import (
	"time"

	"github.com/whisk-run/contract/status"
)

// The record of everything a business owns outside the platform's database (CONTROL-PLANE.md
// §6.32), as the operator reads it: GET /operator/orgs/:org/resources and
// POST /operator/orgs/:org/resources/:id/confirm.

// ResourceState is where a recorded thing stands: creating before the provider is asked, live
// once made, releasing while a release asks the provider, released once gone, retained when kept
// on purpose, adopted when a lookup found it rather than it being recorded when made (released
// only once an operator confirms it), failed when a release or create did not finish.
type ResourceState string

const (
	ResourceCreating  ResourceState = "creating"
	ResourceLive      ResourceState = "live"
	ResourceReleasing ResourceState = "releasing"
	ResourceReleased  ResourceState = "released"
	ResourceRetained  ResourceState = "retained"
	ResourceAdopted   ResourceState = "adopted"
	ResourceFailed    ResourceState = "failed"
)

// ResourceRelease is what releasing a kind does: delete it by its recorded id, retain it for
// the reason given, or let its parent's release take it.
type ResourceRelease string

const (
	ReleaseDelete     ResourceRelease = "delete"
	ReleaseRetain     ResourceRelease = "retain"
	ReleaseWithParent ResourceRelease = "with_parent"
)

// ResourceExport is what the business's export holds of a kind.
type ResourceExport string

const (
	ExportRepo  ResourceExport = "repo"
	ExportDump  ResourceExport = "dump"
	ExportFiles ResourceExport = "files"
	ExportNone  ResourceExport = "none"
)

// Resource is one thing a business owns or owned, by its id at its provider, with its kind's
// rules. Kind is one of the kinds CONTROL-PLANE.md §6.32 lists. NotExported and Retained say why,
// where the export holds none of it or the release keeps it.
type Resource struct {
	ID            string          `json:"id"`
	OrgID         string          `json:"org_id"`
	AppID         string          `json:"app_id,omitempty"`
	EnvironmentID string          `json:"environment_id,omitempty"`
	Kind          string          `json:"kind"`
	Provider      string          `json:"provider"`
	Account       string          `json:"account"`
	ExternalID    string          `json:"external_id"`
	ParentID      string          `json:"parent_id,omitempty"`
	Detail        map[string]any  `json:"detail"`
	State         ResourceState   `json:"state"`
	ReleaseAt     *time.Time      `json:"release_at,omitempty"`
	ReleasedAt    *time.Time      `json:"released_at,omitempty"`
	Attempts      int             `json:"attempts"`
	LastError     string          `json:"last_error,omitempty"`
	MadeBy        string          `json:"made_by,omitempty"`
	ConfirmedBy   string          `json:"confirmed_by,omitempty"`
	ConfirmedAt   *time.Time      `json:"confirmed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Release       ResourceRelease `json:"release"`
	Export        ResourceExport  `json:"export"`
	NotExported   string          `json:"not_exported,omitempty"`
	Retained      string          `json:"retained,omitempty"`
}

// OrgResources is a business's record: every row, whether a release of it is done, and the ids
// of the rows stuck releasing.
type OrgResources struct {
	Org       string     `json:"org"`
	Status    status.Org `json:"status"`
	Done      bool       `json:"done"`
	Stuck     []string   `json:"stuck"`
	Resources []Resource `json:"resources"`
}

// ConfirmedResource answers a confirmation: the row after its release ran, whether it is
// released, the release's error when it is not, and how many releases failed.
type ConfirmedResource struct {
	Resource Resource `json:"resource"`
	Released bool     `json:"released"`
	Error    string   `json:"error"`
	Failed   int      `json:"failed"`
}

// WaitingResource is an adopted row that is due, with the names an operator reads it by: the
// business's, and the app's when the row was an app's.
type WaitingResource struct {
	Resource Resource `json:"resource"`
	OrgName  string   `json:"org_name"`
	OrgSlug  string   `json:"org_slug"`
	AppName  string   `json:"app_name,omitempty"`
}

// WaitingResources is every adopted row across the platform that waits on an operator.
type WaitingResources struct {
	Items []WaitingResource `json:"items"`
}
