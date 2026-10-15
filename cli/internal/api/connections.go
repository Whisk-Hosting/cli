package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// Connections: outside systems an app reaches through Whisk's broker (CONTROL-PLANE.md §6.8).
// Granting and resuming are a person's, confirmed, in the dashboard; the CLI lists and pauses.
type (
	Connection         = apitypes.Connection
	ConnectionGrant    = apitypes.ConnectionGrant
	GrantNeeded        = apitypes.GrantNeeded
	ConnectionsChanged = apitypes.ConnectionsChanged
)

// ListConnections returns each connection the app's active manifests declare, with Whisk's
// summary, its grants by environment and whether a deploy waits on it.
func (c *Client) ListConnections(ctx context.Context, org, app string) ([]Connection, error) {
	return listAll[Connection](ctx, c, appPath(org, app)+"/connections")
}

// PauseConnection stops one connection's calls in environment, or in every environment when
// environment is "".
func (c *Client) PauseConnection(ctx context.Context, org, app, name string, environment apitypes.ConnectionEnvironment) (ConnectionsChanged, error) {
	var out ConnectionsChanged
	body := apitypes.ConnectionStateRequest{Environment: environment}
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/connections/"+pathSeg(name)+"/pause", body, &out)
}

// PauseAllConnections stops every connection of every app in the org.
func (c *Client) PauseAllConnections(ctx context.Context, org string) (ConnectionsChanged, error) {
	var out ConnectionsChanged
	return out, c.Do(ctx, http.MethodPost, orgPath(org)+"/connections/pause", map[string]any{}, &out)
}
