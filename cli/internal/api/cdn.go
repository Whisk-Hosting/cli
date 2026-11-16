package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// CDNHostname is one name of an app and whether the CDN answers it; Reason says why one
// reaches the origin directly: bare, business or preview (CONTROL-PLANE.md §6.27).
type CDNHostname = apitypes.CdnHostname

// CDN is GET /orgs/:org/apps/:app/cdn: whether the plan includes it, whether the platform
// offers it, what was asked, where the switch has got to, and this month's traffic.
type CDN = apitypes.AppCdn

// CDNPurge is a purge's answer: whether the CDN serves the app, whether its whole copy was
// dropped, and each address dropped.
type CDNPurge = apitypes.CdnPurge

// PurgeCDN drops paths from the app's copy on the CDN, or the whole copy when paths is empty.
func (c *Client) PurgeCDN(ctx context.Context, org, app string, paths []string) (CDNPurge, error) {
	if paths == nil {
		paths = []string{}
	}
	return Send(ctx, c, endpoints.PurgeAppCDN, apitypes.CdnPurgeRequest{Paths: paths}, org, app)
}
