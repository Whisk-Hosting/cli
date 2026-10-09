package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/whisk-run/contract/apitypes"
)

// The operator's read access (CONTROL-PLANE.md §6.20): an operator's session, or an operator
// read token in WHISK_TOKEN, reads these; the token changes nothing.

// PlatformLogQuery is what the operator asks of the platform's own lines.
type PlatformLogQuery struct {
	Unit   string
	Host   string
	Stream string
	Grep   string
	Since  string
	Until  string
	Limit  int
}

func (q PlatformLogQuery) values() url.Values {
	v := url.Values{}
	for key, value := range map[string]string{"unit": q.Unit, "host": q.Host, "stream_name": q.Stream, "q": q.Grep, "since": q.Since, "until": q.Until} {
		if value != "" {
			v.Set(key, value)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", fmt.Sprint(q.Limit))
	}
	return v
}

// PlatformLogs reads a window of the platform's lines, oldest first.
func (c *Client) PlatformLogs(ctx context.Context, q PlatformLogQuery) ([]LogLine, error) {
	var out struct {
		Items []LogLine `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, "/operator/logs?"+q.values().Encode(), nil, &out)
}

// TailPlatformLogs follows the platform's lines until the context ends, reconnecting as
// TailLogs does.
func (c *Client) TailPlatformLogs(ctx context.Context, q PlatformLogQuery, fn func([]LogLine) error) error {
	return c.followLines(ctx, func(since string) string {
		v := q.values()
		v.Set("stream", "true")
		if since != "" {
			v.Set("since", since)
		}
		return "/operator/logs?" + v.Encode()
	}, q.Since, fn)
}

// PlatformLogLabels lists the services and hosts that shipped lines since the time given.
func (c *Client) PlatformLogLabels(ctx context.Context, since string) (units, hosts []string, err error) {
	v := url.Values{}
	if since != "" {
		v.Set("since", since)
	}
	var out struct {
		Units []string `json:"units"`
		Hosts []string `json:"hosts"`
	}
	err = c.Do(ctx, http.MethodGet, "/operator/logs/labels?"+v.Encode(), nil, &out)
	return out.Units, out.Hosts, err
}

// Node is a node as the operator sees it.
type Node = apitypes.Node

// OperatorNodes lists every node.
func (c *Client) OperatorNodes(ctx context.Context) ([]Node, error) {
	var out struct {
		Items []Node `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, "/operator/nodes", nil, &out)
}

// OperatorDeploys lists deploys across every org in a window: status is a deploy status or
// all, failed when empty; since is a duration or a time, a day when empty.
func (c *Client) OperatorDeploys(ctx context.Context, status, since string) ([]Deploy, error) {
	v := url.Values{}
	if status != "" {
		v.Set("status", status)
	}
	if since != "" {
		v.Set("since", since)
	}
	var out struct {
		Items []Deploy `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, "/operator/deploys?"+v.Encode(), nil, &out)
}

// Demo is a business an operator made for another company to hand over with a link
// (CONTROL-PLANE.md §6.1).
type Demo = apitypes.Demo

// CreateDemo makes a demo business for the company at domain, owned by the operator until
// someone at that domain claims it.
func (c *Client) CreateDemo(ctx context.Context, name, domain, slug string) (Demo, error) {
	var out Demo
	body := map[string]string{"name": name, "domain": domain}
	if slug != "" {
		body["slug"] = slug
	}
	return out, c.Do(ctx, http.MethodPost, "/operator/demos", body, &out)
}

// Demos lists the operator's demo businesses, newest first.
func (c *Client) Demos(ctx context.Context) ([]Demo, error) {
	var out struct {
		Items []Demo `json:"items"`
	}
	return out.Items, c.Do(ctx, http.MethodGet, "/operator/demos", nil, &out)
}

// Comp is a business on a paid plan free of charge (CONTROL-PLANE.md §6.15).
type Comp = apitypes.Comp

// CompedOrg is a business as the operator's comp routes answer it.
type CompedOrg struct {
	Org
	Comp *Comp `json:"comp,omitempty"`
}

// CompOrg puts a business on a paid plan free of charge; endsAt is a day (2027-03-31), or empty
// for no end.
func (c *Client) CompOrg(ctx context.Context, org, plan, endsAt, reason string) (CompedOrg, error) {
	var out CompedOrg
	body := map[string]string{"plan": plan, "reason": reason}
	if endsAt != "" {
		body["ends_at"] = endsAt
	}
	return out, c.Do(ctx, http.MethodPut, "/operator/orgs/"+url.PathEscape(org)+"/comp", body, &out)
}

// EndComp ends a business's comp now; it is back on Free.
func (c *Client) EndComp(ctx context.Context, org string) (CompedOrg, error) {
	var out CompedOrg
	return out, c.Do(ctx, http.MethodDelete, "/operator/orgs/"+url.PathEscape(org)+"/comp", nil, &out)
}

// Comps lists the comped businesses, from the operator's list of every org.
func (c *Client) Comps(ctx context.Context) ([]CompedOrg, error) {
	var out struct {
		Items []CompedOrg `json:"items"`
	}
	if err := c.Do(ctx, http.MethodGet, "/operator/orgs", nil, &out); err != nil {
		return nil, err
	}
	comped := []CompedOrg{}
	for _, o := range out.Items {
		if o.Comp != nil {
			comped = append(comped, o)
		}
	}
	return comped, nil
}

// Hold is an abuse hold (CONTROL-PLANE.md §6.27).
type Hold = apitypes.Hold

// Holds lists the abuse holds newest first, those in status when it is set.
func (c *Client) Holds(ctx context.Context, status string) ([]Hold, error) {
	var out struct {
		Items []Hold `json:"items"`
	}
	path := "/operator/holds"
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	return out.Items, c.Do(ctx, http.MethodGet, path, nil, &out)
}
