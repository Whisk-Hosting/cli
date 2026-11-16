package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The routes of control-plane/internal/api/handlers_source.go.
var (
	GetSource  = New[GetSourceQuery, apitypes.SourceListing]("GET /v1/orgs/{org}/apps/{app}/source")
	PostSource = New[apitypes.SourceRequest, apitypes.SourceCommit]("POST /v1/orgs/{org}/apps/{app}/source")

	_ = public.Add(GetSource, PostSource)
)

// GetSourceQuery is what GetSource reads from the query.
type GetSourceQuery struct {
	Path string `query:"path"`
}
