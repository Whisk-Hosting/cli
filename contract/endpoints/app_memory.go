package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The app memory routes.
var (
	GetAppMemory = New[GetAppMemoryQuery, apitypes.AppMemoryHistory]("GET /v1/orgs/{org}/apps/{app}/memory")

	_ = public.Add(GetAppMemory)
)

// GetAppMemoryQuery is what GetAppMemory reads from the query.
type GetAppMemoryQuery struct {
	Since string `query:"since"`
}
