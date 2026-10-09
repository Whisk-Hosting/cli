package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/whisk-run/contract/apitypes"
)

// Owned is one thing outside the platform's database a business owns or owned
// (CONTROL-PLANE.md §6.32): a repository, a database, a bucket, a sending domain, a Stripe
// customer, and the rest, by its id at its provider.
type Owned = apitypes.Resource

// OrgOwned is a business's record: every row, whether a release of it is done, and the rows
// stuck releasing.
type OrgOwned = apitypes.OrgResources

// OrgResources reads a business's record (operators only).
func (c *Client) OrgResources(ctx context.Context, org string) (OrgOwned, error) {
	var out OrgOwned
	return out, c.Do(ctx, http.MethodGet, "/operator/orgs/"+url.PathEscape(org)+"/resources", nil, &out)
}

// ConfirmedOwned is a confirmed row after its release ran.
type ConfirmedOwned = apitypes.ConfirmedResource

// ConfirmResource confirms that a row a lookup found (adopted) is the business's and may be
// released, which it is at once, by its recorded id (operators only).
func (c *Client) ConfirmResource(ctx context.Context, org, id string) (ConfirmedOwned, error) {
	var out ConfirmedOwned
	return out, c.Do(ctx, http.MethodPost, "/operator/orgs/"+url.PathEscape(org)+"/resources/"+url.PathEscape(id)+"/confirm", nil, &out)
}
