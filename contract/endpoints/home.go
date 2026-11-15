package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The home routes.
var (
	PutHome = New[apitypes.HomeLayout, apitypes.HomeLayout]("PUT /v1/orgs/{org}/home")

	_ = public.Add(PutHome)
)
