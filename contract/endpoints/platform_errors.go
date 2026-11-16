package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The routes of control-plane/internal/api/handlers_platform_errors.go.
var (
	PostClientError = New[apitypes.ClientErrorReport, apitypes.ClientErrorReceipt]("POST /v1/client-errors")

	_ = public.Add(PostClientError)
)
