package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The cli misses routes.
var (
	PostCLIMiss = New[apitypes.CLIMiss, apitypes.CLIMissReceipt]("POST /v1/cli/misses")

	_ = public.Add(PostCLIMiss)
)
