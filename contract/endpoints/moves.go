package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The moves routes.
var (
	MoveTargets = New[None, apitypes.AppMoveTargets]("GET /v1/orgs/{org}/apps/{app}/move/targets")
	WarmMove    = New[apitypes.AppMoveRequest, None]("POST /v1/orgs/{org}/apps/{app}/move/warm")
	MoveApp     = New[apitypes.AppMoveRequest, apitypes.AppMove]("POST /v1/orgs/{org}/apps/{app}/move")
	GetMove     = New[None, apitypes.AppMove]("GET /v1/orgs/{org}/apps/{app}/move")
	RetryMove   = New[None, apitypes.AppMove]("POST /v1/orgs/{org}/apps/{app}/move/retry")
	CancelMove  = New[None, apitypes.AppMove]("DELETE /v1/orgs/{org}/apps/{app}/move")

	_ = public.Add(MoveTargets, WarmMove, MoveApp, GetMove, RetryMove, CancelMove)
)
