package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The invites routes.
var (
	InviteAgain = New[None, apitypes.Member]("POST /v1/orgs/{org}/members/{id}/invite")

	_ = public.Add(InviteAgain)
)
