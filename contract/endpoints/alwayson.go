package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The alwayson routes.
var (
	SetAlwaysOn = New[apitypes.AlwaysOnRequest, apitypes.AlwaysOnResult]("PUT /v1/orgs/{org}/apps/{app}/always-on")

	_ = public.Add(SetAlwaysOn)
)
