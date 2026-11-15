package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The demos routes.
var (
	DemoInfo  = New[None, apitypes.DemoInfo]("GET /v1/demos/{code}")
	ClaimDemo = New[None, apitypes.Org]("POST /v1/demos/{code}/claim")

	_ = public.Add(DemoInfo, ClaimDemo)
)
