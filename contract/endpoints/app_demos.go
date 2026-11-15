package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The app demos routes.
var (
	GetDemo          = New[None, apitypes.Demo]("GET /v1/orgs/{org}/apps/{app}/demo")
	SetDemo          = New[apitypes.DemoRequest, apitypes.Demo]("PUT /v1/orgs/{org}/apps/{app}/demo")
	StopDemo         = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/demo")
	InviteToDemo     = New[apitypes.DemoPeopleRequest, apitypes.DemoPerson]("POST /v1/orgs/{org}/apps/{app}/demo/people")
	UninviteFromDemo = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/demo/people/{id}")
	DemoOffer        = New[None, apitypes.DemoOffer]("GET /v1/app-demos/{code}")
	CopyDemo         = New[apitypes.DemoCopyRequest, apitypes.DemoCopy]("POST /v1/app-demos/{code}/copies")
	DemoCopy         = New[None, apitypes.DemoCopy]("GET /v1/app-demos/{code}/copies/{id}")
	RetryDemoCopy    = New[None, apitypes.DemoCopy]("POST /v1/app-demos/{code}/copies/{id}/retry")

	_ = public.Add(GetDemo, SetDemo, StopDemo, InviteToDemo, UninviteFromDemo, DemoOffer, CopyDemo, DemoCopy, RetryDemoCopy)
)
