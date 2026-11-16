package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The routes of control-plane/internal/api/handlers_numbers.go.
var (
	PostVisit = New[apitypes.VisitBeacon, apitypes.VisitReceipt]("POST /v1/visits")

	_ = public.Add(PostVisit)
)
