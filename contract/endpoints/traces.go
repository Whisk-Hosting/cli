package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The traces routes.
var (
	GetTrace   = New[None, apitypes.TraceDetail]("GET /v1/orgs/{org}/apps/{app}/traces/{id}")
	ListTraces = New[ListTracesQuery, apitypes.TraceList]("GET /v1/orgs/{org}/apps/{app}/traces")

	_ = public.Add(GetTrace, ListTraces)
)

// ListTracesQuery is what ListTraces reads from the query.
type ListTracesQuery struct {
	Env   string `query:"env"`
	Limit int    `query:"limit"`
	MinMS int    `query:"min_ms"`
	Q     string `query:"q"`
	Since string `query:"since"`
	Sort  string `query:"sort"`
	Until string `query:"until"`
}
