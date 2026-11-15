package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The promoted routes.
var (
	PromoteApp   = New[None, apitypes.App]("PUT /v1/orgs/{org}/apps/{app}/promoted")
	UnpromoteApp = New[None, apitypes.App]("DELETE /v1/orgs/{org}/apps/{app}/promoted")

	_ = public.Add(PromoteApp, UnpromoteApp)
)
