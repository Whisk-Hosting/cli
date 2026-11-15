package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The rebates routes.
var (
	Rebates      = New[None, apitypes.Rebates]("GET /v1/orgs/{org}/billing/rebates")
	SetUpPayouts = New[None, map[string]any]("POST /v1/orgs/{org}/billing/payouts")

	_ = public.Add(Rebates, SetUpPayouts)
)
