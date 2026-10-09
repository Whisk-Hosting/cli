package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// AccountExport is GET /me/export: everything Whisk keeps about the person as themselves. The
// CLI saves the bytes as they came and reads the counts it reports from the decoded copy.
type AccountExport = apitypes.AccountExport

// ExportAccount fetches the export and the bytes it came as.
func (c *Client) ExportAccount(ctx context.Context) (AccountExport, []byte, error) {
	var raw json.RawMessage
	if err := c.Do(ctx, http.MethodGet, "/me/export", nil, &raw); err != nil {
		return AccountExport{}, nil, err
	}
	var out AccountExport
	if err := json.Unmarshal(raw, &out); err != nil {
		return AccountExport{}, nil, err
	}
	return out, raw, nil
}

// DeleteAccount is DELETE /me: the person deletes their own account.
func (c *Client) DeleteAccount(ctx context.Context) error {
	return c.Do(ctx, http.MethodDelete, "/me", nil, nil)
}
