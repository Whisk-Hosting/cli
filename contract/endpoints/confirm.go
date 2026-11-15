package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The confirm routes.
var (
	ConfirmCode     = New[apitypes.ConfirmCodeRequest, apitypes.ConfirmCode]("POST /v1/me/confirm/code")
	ConfirmWithCode = New[apitypes.ConfirmRequest, apitypes.Confirmed]("POST /v1/me/confirm")

	_ = public.Add(ConfirmCode, ConfirmWithCode)
)
