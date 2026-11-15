package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The me routes.
var (
	ExportMe = New[None, apitypes.AccountExport]("GET /v1/me/export")
	DeleteMe = New[None, None]("DELETE /v1/me")

	_ = public.Add(ExportMe, DeleteMe)
)
