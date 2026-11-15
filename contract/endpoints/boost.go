package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The boost routes.
var (
	BoostApp   = New[None, apitypes.App]("PUT /v1/orgs/{org}/apps/{app}/boost")
	UnboostApp = New[None, apitypes.App]("DELETE /v1/orgs/{org}/apps/{app}/boost")

	_ = public.Add(BoostApp, UnboostApp)
)
