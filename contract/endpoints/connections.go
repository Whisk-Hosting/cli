package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The connections routes.
var (
	ListConnections       = New[None, apitypes.Page[apitypes.Connection]]("GET /v1/orgs/{org}/apps/{app}/connections")
	GrantConnection       = New[apitypes.GrantConnectionRequest, apitypes.Connection]("POST /v1/orgs/{org}/apps/{app}/connections/{name}/grant")
	SetSpendCap           = New[apitypes.SpendCapRequest, apitypes.Connection]("PUT /v1/orgs/{org}/apps/{app}/connections/{name}/spend-cap")
	SetConnectionLimits   = New[apitypes.ConnectionLimitsRequest, apitypes.Connection]("PUT /v1/orgs/{org}/apps/{app}/connections/{name}/limits")
	ConnectionState       = New[apitypes.ConnectionStateRequest, apitypes.ConnectionsChanged]("POST /v1/orgs/{org}/apps/{app}/connections/{name}/pause")
	MakeConnectionKeypair = New[None, apitypes.ConnectionKeypair]("POST /v1/orgs/{org}/apps/{app}/connections/{name}/keypair")
	ConnectionStatePost   = New[apitypes.ConnectionStateRequest, apitypes.ConnectionsChanged]("POST /v1/orgs/{org}/apps/{app}/connections/{name}/resume")
	ConnectionState2      = New[apitypes.ConnectionStateRequest, apitypes.ConnectionsChanged]("POST /v1/orgs/{org}/apps/{app}/connections/{name}/revoke")
	PauseOrgConnections   = New[None, apitypes.ConnectionsChanged]("POST /v1/orgs/{org}/connections/pause")

	_ = public.Add(ListConnections, GrantConnection, SetSpendCap, SetConnectionLimits, ConnectionState, MakeConnectionKeypair, ConnectionStatePost, ConnectionState2, PauseOrgConnections)
)
