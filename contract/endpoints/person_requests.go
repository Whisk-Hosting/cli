package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The person requests routes.
var (
	GetPersonRequest     = New[None, apitypes.PersonRequest]("GET /v1/orgs/{org}/requests/{id}")
	ApprovePersonRequest = New[None, apitypes.PersonRequest]("POST /v1/orgs/{org}/requests/{id}/approve")
	DeclinePersonRequest = New[None, apitypes.PersonRequest]("POST /v1/orgs/{org}/requests/{id}/decline")

	_ = public.Add(GetPersonRequest, ApprovePersonRequest, DeclinePersonRequest)
)
