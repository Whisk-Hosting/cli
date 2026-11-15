package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The logs routes.
var (
	ListErrors = New[ListErrorsQuery, apitypes.List[apitypes.ErrorGroup]]("GET /v1/orgs/{org}/apps/{app}/errors")
	GetError   = New[None, apitypes.ErrorSample]("GET /v1/orgs/{org}/apps/{app}/errors/{id}")

	_ = public.Add(ListErrors, GetError)
)

// ListErrorsQuery is what ListErrors reads from the query.
type ListErrorsQuery struct {
	Limit  int    `query:"limit"`
	Q      string `query:"q"`
	Since  string `query:"since"`
	Status string `query:"status"`
}
