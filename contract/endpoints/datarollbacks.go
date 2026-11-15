package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The datarollbacks routes.
var (
	RollbackWithData = New[apitypes.RollbackWithDataRequest, apitypes.Deploy]("POST /v1/orgs/{org}/apps/{app}/rollback-with-data")

	_ = public.Add(RollbackWithData)
)
