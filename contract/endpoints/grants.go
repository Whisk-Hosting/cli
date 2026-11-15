package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The grants routes.
var (
	ApproveDeploy    = New[apitypes.ApproveDeployRequest, apitypes.ApprovedDeploy]("POST /v1/orgs/{org}/apps/{app}/deploys/{id}/approve")
	GrantSecretApp   = New[None, apitypes.Secret]("PUT /v1/orgs/{org}/secrets/{name}/apps/{app}")
	UngrantSecretApp = New[None, apitypes.Secret]("DELETE /v1/orgs/{org}/secrets/{name}/apps/{app}")

	_ = public.Add(ApproveDeploy, GrantSecretApp, UngrantSecretApp)
)
