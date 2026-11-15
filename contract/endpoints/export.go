package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The export routes.
var (
	StartExport = New[None, apitypes.Export]("POST /v1/orgs/{org}/export")
	ListExports = New[ListExportsQuery, apitypes.Page[apitypes.Export]]("GET /v1/orgs/{org}/exports")
	GetExport   = New[None, apitypes.Export]("GET /v1/orgs/{org}/export/{id}")

	_ = public.Add(StartExport, ListExports, GetExport)
)

// ListExportsQuery is what ListExports reads from the query.
type ListExportsQuery struct {
	Limit int `query:"limit"`
}
