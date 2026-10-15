package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// Billing is the org's plan, this month's usage and where it stands with payments
// (CONTROL-PLANE.md §6.15).
type Billing = apitypes.Billing

// GetBilling reads the org's billing.
func (c *Client) GetBilling(ctx context.Context, org string) (Billing, error) {
	var out Billing
	err := c.Do(ctx, http.MethodGet, orgPath(org)+"/billing", nil, &out)
	return out, err
}
