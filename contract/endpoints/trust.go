package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The trust routes.
var (
	GetTrust = New[None, apitypes.Trust]("GET /v1/orgs/{org}/trust")

	_ = public.Add(GetTrust)
)
