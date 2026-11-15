package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The previews routes.
var (
	ListBranches  = New[None, apitypes.Page[apitypes.Branch]]("GET /v1/orgs/{org}/apps/{app}/branches")
	StartPreview  = New[apitypes.StartPreviewRequest, apitypes.StartedPreview]("POST /v1/orgs/{org}/apps/{app}/previews")
	RestartOwnApp = New[None, apitypes.App]("POST /v1/orgs/{org}/apps/{app}/restart")

	_ = public.Add(ListBranches, StartPreview, RestartOwnApp)
)
