package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The logs routes.
var (
	ListErrors     = New[ListErrorsQuery, apitypes.List[apitypes.ErrorGroup]]("GET /v1/orgs/{org}/apps/{app}/errors")
	GetError       = New[None, apitypes.ErrorSample]("GET /v1/orgs/{org}/apps/{app}/errors/{id}")
	SetErrorStatus = New[apitypes.SetErrorStatusRequest, apitypes.ErrorStatusSet]("POST /v1/orgs/{org}/apps/{app}/errors/{id}")
	GetLogs        = New[GetLogsQuery, apitypes.LogPage]("GET /v1/orgs/{org}/apps/{app}/logs")

	_ = public.Add(ListErrors, GetError, SetErrorStatus, GetLogs)
)

// ListErrorsQuery is what ListErrors reads from the query.
type ListErrorsQuery struct {
	Limit  int    `query:"limit"`
	Q      string `query:"q"`
	Since  string `query:"since"`
	Status string `query:"status"`
}

// GetLogsQuery is what GetLogs reads from the query. With stream=true the route follows the logs
// as server-sent events instead, which the declaration does not describe.
type GetLogsQuery struct {
	Env        string `query:"env"`
	Limit      int    `query:"limit"`
	Q          string `query:"q"`
	Since      string `query:"since"`
	Stream     string `query:"stream"`
	StreamName string `query:"stream_name"`
	Until      string `query:"until"`
}
