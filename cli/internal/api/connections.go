package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// Connections: outside systems an app reaches through Whisk's broker (CONTROL-PLANE.md §6.8).
// Granting and resuming are a person's, confirmed, in the dashboard; the CLI lists, pauses and
// sets limits.
type (
	Connection         = apitypes.Connection
	ConnectionGrant    = apitypes.ConnectionGrant
	GrantNeeded        = apitypes.GrantNeeded
	ConnectionsChanged = apitypes.ConnectionsChanged
)

// PauseConnection stops one connection's calls in environment, or in every environment when
// environment is "".
func (c *Client) PauseConnection(ctx context.Context, org, app, name string, environment apitypes.ConnectionEnvironment) (ConnectionsChanged, error) {
	return Send(ctx, c, endpoints.PauseConnection, apitypes.ConnectionStateRequest{Environment: environment}, org, app, name)
}
