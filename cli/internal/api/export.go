package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// Export is one export of an org (CONTROL-PLANE.md §6.19).
type Export = apitypes.Export

// StartExport starts an export of the whole org.
func (c *Client) StartExport(ctx context.Context, org string) (Export, error) {
	var out Export
	err := c.Do(ctx, http.MethodPost, orgPath(org)+"/export", map[string]any{}, &out)
	return out, err
}

// GetExport reads one export, with its link once the archive is ready.
func (c *Client) GetExport(ctx context.Context, org, id string) (Export, error) {
	var out Export
	err := c.Do(ctx, http.MethodGet, orgPath(org)+"/export/"+pathSeg(id), nil, &out)
	return out, err
}
