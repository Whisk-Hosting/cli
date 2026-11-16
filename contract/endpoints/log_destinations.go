package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The log destinations routes.
var (
	ListLogDestinations  = New[None, apitypes.LogDestinationList]("GET /v1/orgs/{org}/logs/destinations")
	AddLogDestination    = New[apitypes.LogDestinationRequest, apitypes.LogDestination]("POST /v1/orgs/{org}/logs/destinations")
	TestLogDestination   = New[None, apitypes.LogDestinationTest]("POST /v1/orgs/{org}/logs/destinations/{id}/test")
	DeleteLogDestination = New[None, None]("DELETE /v1/orgs/{org}/logs/destinations/{id}")

	_ = public.Add(ListLogDestinations, AddLogDestination, TestLogDestination, DeleteLogDestination)
)
