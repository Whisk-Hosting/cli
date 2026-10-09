package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// CDNHostname is one name of an app and whether the CDN answers it; Reason says why one
// reaches the origin directly: bare, business or preview (CONTROL-PLANE.md §6.27).
type CDNHostname = apitypes.CdnHostname

// CDN is GET /orgs/:org/apps/:app/cdn: whether the plan includes it, whether the platform
// offers it, what was asked, where the switch has got to, and this month's traffic.
type CDN = apitypes.AppCdn

func cdnPath(org, app string) string { return appPath(org, app) + "/cdn" }

// GetCDN reads the app's CDN.
func (c *Client) GetCDN(ctx context.Context, org, app string) (CDN, error) {
	var out CDN
	return out, c.Do(ctx, http.MethodGet, cdnPath(org, app), nil, &out)
}

// SetCDN records whether the app should be served through the CDN; the platform answers 202
// with the same shape while the switch proceeds.
func (c *Client) SetCDN(ctx context.Context, org, app string, on bool) (CDN, error) {
	var out CDN
	return out, c.Do(ctx, http.MethodPut, cdnPath(org, app), map[string]bool{"on": on}, &out)
}
