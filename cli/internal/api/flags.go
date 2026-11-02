package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// AppFlags lists the flags the app's whisk.yaml in force declares and who each is on for
// (CONTROL-PLANE.md §6.39).
func (c *Client) AppFlags(ctx context.Context, org, app string) (apitypes.AppFlagList, error) {
	var out apitypes.AppFlagList
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/flags", nil, &out)
}

// SetAppFlag sets who a flag is on for from now, replacing what was set.
func (c *Client) SetAppFlag(ctx context.Context, org, app, name string, in apitypes.AppFlagRequest) (apitypes.AppFlag, error) {
	var out apitypes.AppFlag
	return out, c.Do(ctx, http.MethodPut, appPath(org, app)+"/flags/"+pathSeg(name), in, &out)
}
