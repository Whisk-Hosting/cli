package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The logins routes.
var (
	GetLogin            = New[None, apitypes.Login]("GET /v1/me/logins/{id}")
	AddLoginBusiness    = New[apitypes.LoginBusinessRequest, apitypes.Login]("POST /v1/me/logins/{id}/businesses")
	RemoveLoginBusiness = New[None, apitypes.Login]("DELETE /v1/me/logins/{id}/businesses/{org}")

	_ = public.Add(GetLogin, AddLoginBusiness, RemoveLoginBusiness)
)
