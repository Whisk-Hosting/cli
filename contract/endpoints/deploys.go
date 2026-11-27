package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The deploys routes.
var (
	ListBuilds   = New[ListBuildsQuery, apitypes.Page[apitypes.Build]]("GET /v1/orgs/{org}/apps/{app}/builds")
	GetBuild     = New[None, apitypes.Build]("GET /v1/orgs/{org}/apps/{app}/builds/{id}")
	ListDeploys  = New[ListDeploysQuery, apitypes.Page[apitypes.Deploy]]("GET /v1/orgs/{org}/apps/{app}/deploys")
	GetDeploy    = New[None, apitypes.Deploy]("GET /v1/orgs/{org}/apps/{app}/deploys/{id}")
	CreateDeploy = New[apitypes.CreateDeployRequest, apitypes.Deploy]("POST /v1/orgs/{org}/apps/{app}/deploys")
	CancelDeploy = New[None, apitypes.Deploy]("POST /v1/orgs/{org}/apps/{app}/deploys/{id}/cancel")

	_ = public.Add(ListBuilds, GetBuild, ListDeploys, GetDeploy, CreateDeploy, CancelDeploy)
)

// ListBuildsQuery is what ListBuilds reads from the query.
type ListBuildsQuery struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

// ListDeploysQuery is what ListDeploys reads from the query. Env, when set, is the name of one
// of the app's environments (production, preview:<branch>), and only its deploys are listed.
type ListDeploysQuery struct {
	Cursor string `query:"cursor"`
	Env    string `query:"env"`
	Limit  int    `query:"limit"`
}
