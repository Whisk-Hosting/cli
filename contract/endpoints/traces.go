package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The traces routes.
var (
	GetTrace = New[None, apitypes.TraceDetail]("GET /v1/orgs/{org}/apps/{app}/traces/{id}")

	_ = public.Add(GetTrace)
)
