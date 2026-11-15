package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The apps routes.
var (
	ListApps          = New[ListAppsQuery, apitypes.Page[apitypes.App]]("GET /v1/orgs/{org}/apps")
	CreateApp         = New[apitypes.CreateAppRequest, apitypes.App]("POST /v1/orgs/{org}/apps")
	GetApp            = New[None, apitypes.App]("GET /v1/orgs/{org}/apps/{app}")
	PatchApp          = New[apitypes.PatchAppRequest, apitypes.App]("PATCH /v1/orgs/{org}/apps/{app}")
	RenameApp         = New[apitypes.RenameAppRequest, apitypes.App]("POST /v1/orgs/{org}/apps/{app}/rename")
	DeleteApp         = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}")
	ListDeletedApps   = New[None, apitypes.Page[apitypes.DeletedApp]]("GET /v1/orgs/{org}/deleted-apps")
	RestoreDeletedApp = New[None, apitypes.App]("POST /v1/orgs/{org}/deleted-apps/{id}/restore")
	GetManifest       = New[None, apitypes.Manifest]("GET /v1/orgs/{org}/apps/{app}/manifest")
	Validate          = New[apitypes.ValidateRequest, apitypes.Validation]("POST /v1/orgs/{org}/apps/{app}/validate")
	ListEnvironments  = New[None, apitypes.Page[apitypes.Environment]]("GET /v1/orgs/{org}/apps/{app}/environments")
	DeleteEnvironment = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/environments/{env}")
	GetAccess         = New[None, apitypes.Access]("GET /v1/orgs/{org}/apps/{app}/access")
	SetAccess         = New[apitypes.AccessRequest, apitypes.Access]("PUT /v1/orgs/{org}/apps/{app}/access")
	AddGrant          = New[apitypes.Grant, apitypes.Access]("POST /v1/orgs/{org}/apps/{app}/access/grants")
	AddDomain         = New[apitypes.DomainRequest, apitypes.Domain]("POST /v1/orgs/{org}/apps/{app}/domains")
	DeleteDomain      = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/domains/{id}")

	_ = public.Add(ListApps, CreateApp, GetApp, PatchApp, RenameApp, DeleteApp, ListDeletedApps, RestoreDeletedApp, GetManifest, Validate, ListEnvironments, DeleteEnvironment, GetAccess, SetAccess, AddGrant, AddDomain, DeleteDomain)
)

// ListAppsQuery is what ListApps reads from the query.
type ListAppsQuery struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}
