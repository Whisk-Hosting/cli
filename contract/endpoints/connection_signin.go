package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The connection signin routes.
var (
	StartConnectionSignIn = New[apitypes.SignInRequest, apitypes.SignInStart]("POST /v1/orgs/{org}/apps/{app}/connections/{name}/sign-in")

	_ = public.Add(StartConnectionSignIn)
)
