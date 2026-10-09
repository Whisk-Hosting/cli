package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// An agency's client businesses (CONTROL-PLANE.md §6.15, CLI.md §5.11).

// ClientOverview is one client business on the agency's all-clients screen: its standing, whether
// it has been handed over, its live apps, its problems, and storage and runs against its plan.
type ClientOverview = apitypes.ClientOverview

// ClientInput is a new client business: its name, optional slug, the contact it is handed to,
// and how it is billed.
type ClientInput struct {
	Name     string `json:"name"`
	Slug     string `json:"slug,omitempty"`
	Contact  string `json:"contact,omitempty"`
	Interval string `json:"interval"`
	Trial    *bool  `json:"trial,omitempty"`
}

// ClientAdded is the new client business, and the payment page to open when a card is needed.
type ClientAdded = apitypes.ClientAdded

// Handover is the link a client business's contact accepts it with.
type Handover = apitypes.HandoverMade

// ClientsOverview reads every client business of the agency.
func (c *Client) ClientsOverview(ctx context.Context, org string) ([]ClientOverview, error) {
	var out Page[ClientOverview]
	err := c.Do(ctx, http.MethodGet, orgPath(org)+"/clients/overview", nil, &out)
	if out.Items == nil {
		out.Items = []ClientOverview{}
	}
	return out.Items, err
}

// AddClient adds a client business to the agency.
func (c *Client) AddClient(ctx context.Context, org string, in ClientInput) (ClientAdded, error) {
	var out ClientAdded
	err := c.Do(ctx, http.MethodPost, orgPath(org)+"/clients", in, &out)
	return out, err
}

// HandoverClient makes the link the client's contact accepts the business with; an empty contact
// keeps the one already set.
func (c *Client) HandoverClient(ctx context.Context, org, client, contact string) (Handover, error) {
	body := map[string]string{}
	if contact != "" {
		body["contact"] = contact
	}
	var out Handover
	err := c.Do(ctx, http.MethodPost, orgPath(org)+"/clients/"+pathSeg(client)+"/handover", body, &out)
	return out, err
}
